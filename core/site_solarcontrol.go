package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/evcc-io/evcc/api"
	"github.com/evcc-io/evcc/core/loadpoint"
	"github.com/evcc-io/evcc/core/solarcontrol"
	"github.com/evcc-io/evcc/db/settings"
	"github.com/evcc-io/evcc/messenger"
	"github.com/evcc-io/evcc/tariff"
	"github.com/evcc-io/evcc/util"
)

const (
	solarControlKey         = "solarControl" // published state key
	solarControlConfigKey   = "solarcontrol.config"
	solarControlStateKey    = "solarcontrol.state"
	solarControlEventPrefix = "solar" // messenger events solarStart, solarStop, solarResume
)

// solarControlRuntime lives outside the Site struct to keep the upstream diff small
type solarControlRuntime struct {
	once     sync.Once
	cfgMu    sync.Mutex // serializes config read-modify-write
	mu       sync.Mutex // serializes persistence
	ctrl     *solarcontrol.Controller
	log      *util.Logger
	savedRev uint64
}

var solarControls sync.Map // *Site -> *solarControlRuntime

func (site *Site) solarControl() *solarControlRuntime {
	v, _ := solarControls.LoadOrStore(site, new(solarControlRuntime))
	rt := v.(*solarControlRuntime)

	rt.once.Do(func() {
		rt.log = util.NewLogger("solarcontrol")

		// a broken stored config must never stop the control loop or the api
		defer func() {
			if r := recover(); r != nil {
				rt.log.ERROR.Printf("init: %v, using defaults", r)
				rt.ctrl = solarcontrol.New(solarcontrol.DefaultConfig(), solarcontrol.State{}, nil)
			}
			rt.savedRev = rt.ctrl.Revision()
		}()

		rt.ctrl = loadSolarControl(rt.log)
	})

	return rt
}

func loadSolarControl(log *util.Logger) *solarcontrol.Controller {
	cfg := solarcontrol.DefaultConfig()
	if settings.Exists(solarControlConfigKey) {
		stored := cfg // unknown or missing fields keep their defaults
		if err := settings.Json(solarControlConfigKey, &stored); err != nil {
			log.ERROR.Printf("config: %v, using defaults", err)
		} else if err := stored.Validate(); err != nil {
			log.ERROR.Printf("config: %v, using defaults", err)
		} else {
			cfg = stored
		}
	}

	var st solarcontrol.State
	if settings.Exists(solarControlStateKey) {
		if err := settings.Json(solarControlStateKey, &st); err != nil {
			log.ERROR.Printf("state: %v", err)
			st = solarcontrol.State{}
		}
	}

	log.INFO.Printf("mode: %s, strategy: %s", cfg.Mode, cfg.Strategy)

	return solarcontrol.New(cfg, st, nil)
}

// persist stores config and state when the controller changed. flush writes the
// database right away instead of waiting for the periodic settings flush.
func (rt *solarControlRuntime) persist(flush bool) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	rev := rt.ctrl.Revision()
	if rev == rt.savedRev {
		return
	}

	if err := settings.SetJson(solarControlConfigKey, rt.ctrl.Config()); err != nil {
		rt.log.ERROR.Printf("persist config: %v", err)
		return
	}
	if err := settings.SetJson(solarControlStateKey, rt.ctrl.State()); err != nil {
		rt.log.ERROR.Printf("persist state: %v", err)
		return
	}

	rt.savedRev = rev

	if flush {
		if err := settings.Persist(); err != nil {
			rt.log.ERROR.Printf("persist: %v", err)
		}
	}
}

// solarControlStatus publishes as a single json value, not as a topic tree
type solarControlStatus solarcontrol.Status

var _ api.BytesMarshaler = (*solarControlStatus)(nil)

func (s solarControlStatus) MarshalBytes() ([]byte, error) {
	return json.Marshal(solarcontrol.Status(s))
}

func (site *Site) publishSolarControl(rt *solarControlRuntime) {
	site.publish(solarControlKey, solarControlStatus(rt.ctrl.Status()))
}

// updateSolarControl runs the solar charging rules once per control cycle.
// totalChargePower is the charge power of all loadpoints as used for the home power.
func (site *Site) updateSolarControl(totalChargePower float64) {
	if len(site.loadpoints) == 0 {
		return
	}

	site.RLock()
	pv := site.pvPower
	home := max(site.gridPower+max(0, site.pvPower)+site.battery.Power-totalChargePower, 0)
	site.RUnlock()

	fc := site.solarControlForecast(time.Now())

	site.runSolarControl(site.solarControl(), site.loadpoints[0], time.Now(), pv, home, fc)
}

var errSolarControlInactive = errors.New("mode changed, not applied")

// runSolarControl evaluates the controller against a loadpoint and applies its decision
func (site *Site) runSolarControl(rt *solarControlRuntime, lp loadpoint.API, now time.Time, pv, home float64, fc *solarcontrol.Forecast) {
	defer func() {
		if r := recover(); r != nil {
			rt.log.ERROR.Printf("panic: %v", r)
		}
	}()

	in := solarControlInputs(lp, pv, home, fc)
	d := rt.ctrl.Evaluate(now, in)

	if d.Action != solarcontrol.ActionNone {
		// the mode may have changed since evaluating
		err := errSolarControlInactive
		if rt.ctrl.Config().Mode == solarcontrol.ModeActive {
			err = applySolarDecision(lp, d)
		}
		rt.ctrl.Applied(now, d, err)

		if err != nil {
			rt.log.ERROR.Printf("%s: %v", d.Action, err)
			d.Events = nil
		} else {
			site.notifySolarControl(d)
		}
	}

	for _, ev := range d.Events {
		rt.log.INFO.Println(describeSolarEvent(ev))
	}

	rt.persist(false)
	site.publishSolarControl(rt)
}

func solarControlInputs(lp loadpoint.API, pv, home float64, fc *solarcontrol.Forecast) solarcontrol.Inputs {
	status := lp.GetStatus()

	in := solarcontrol.Inputs{
		PV:            pv,
		Home:          home,
		Connected:     status == api.StatusB || status == api.StatusC,
		Charging:      status == api.StatusC,
		ChargePower:   lp.GetChargePower(),
		LpMode:        string(lp.GetMode()),
		AlwaysCharge:  string(lp.GetAlwaysCharge()),
		Forecast:      fc,
		SessionEnergy: lp.GetChargedEnergy(),
	}

	// soc 0 is how evcc reports an unknown soc
	if soc := lp.GetSoc(); soc > 0 && !math.IsNaN(soc) && !math.IsInf(soc, 0) {
		in.Soc = &soc
	}

	if l, ok := lp.(*Loadpoint); ok {
		in.SessionSolarPercentage = l.sessionSolarPercentage()
	}

	return in
}

func (lp *Loadpoint) sessionSolarPercentage() *float64 {
	lp.RLock()
	defer lp.RUnlock()

	if lp.energyMetrics.TotalWh() <= 0 {
		return nil
	}

	return new(lp.energyMetrics.SolarPercentage())
}

// applySolarDecision writes the decision to the loadpoint. Only always charge
// and the mode now -> smart reset on disconnect are ever written.
func applySolarDecision(lp loadpoint.API, d solarcontrol.Decision) error {
	switch d.Action {
	case solarcontrol.ActionStart, solarcontrol.ActionResume:
		if lp.GetMode() != api.ModeSmart {
			return errors.New("mode is not smart")
		}
		return lp.SetAlwaysCharge(api.AlwaysChargeOn)

	case solarcontrol.ActionStop:
		return lp.SetAlwaysCharge(api.AlwaysChargeOff)

	case solarcontrol.ActionDisconnect:
		var err error
		if d.ResetMode && lp.GetMode() == api.ModeNow {
			lp.SetMode(api.ModeSmart)
		}
		if d.ResetAlwaysCharge && lp.GetAlwaysCharge() == api.AlwaysChargeOn {
			err = lp.SetAlwaysCharge(api.AlwaysChargeOff)
		}
		return err
	}

	return nil
}

// solarControlForecast condenses the solar forecast, scaled like the forecast page shows it
func (site *Site) solarControlForecast(now time.Time) *solarcontrol.Forecast {
	rr := tariff.Rates(site.GetTariff(api.TariffUsageSolar))
	if len(rr) == 0 {
		return nil
	}

	rates := make([]solarcontrol.Rate, 0, len(rr))
	for _, r := range rr {
		rates = append(rates, solarcontrol.Rate{Start: r.Start, End: r.End, Value: r.Value})
	}

	energy := func(from, to time.Time) float64 {
		if !from.Before(to) {
			return 0
		}
		return solarEnergy(rr, from, to)
	}

	return solarcontrol.NewForecast(now, time.Local, rates, energy, site.effectiveSolarScale())
}

// notifySolarControl raises messenger events for executed actions. Texts come from
// the messaging event templates, e.g. msg: "{{.solarText}}".
func (site *Site) notifySolarControl(d solarcontrol.Decision) {
	for _, ev := range d.Events {
		switch ev.Type {
		case solarcontrol.EventStart, solarcontrol.EventStop, solarcontrol.EventResume:
		default:
			continue
		}

		name := solarControlEventPrefix + strings.ToUpper(string(ev.Type[:1])) + string(ev.Type[1:])
		site.pushEvent(messenger.Event{
			Loadpoint: new(0),
			Event:     name,
			Attributes: map[string]any{
				"solarTitle":           "Solar control: " + string(ev.Type),
				"solarText":            describeSolarEvent(ev),
				"solarStrategy":        string(ev.Strategy),
				"solarRule":            string(ev.Rule),
				"solarSurplus":         deref(ev.Surplus),
				"solarPv":              deref(ev.PV),
				"solarThreshold":       deref(ev.Threshold),
				"solarEnergy":          deref(ev.Energy),
				"solarSolarPercentage": deref(ev.SolarPercentage),
			},
		})
	}
}

func deref(f *float64) float64 {
	if f == nil {
		return 0
	}
	return *f
}

// describeSolarEvent renders an event as plain English for log and notifications
func describeSolarEvent(ev solarcontrol.Event) string {
	var b strings.Builder

	switch ev.Type {
	case solarcontrol.EventShadow:
		fmt.Fprintf(&b, "shadow (%s): would %s", ev.Strategy, ev.Action)
	case solarcontrol.EventExternal:
		fmt.Fprintf(&b, "external (%s): always charge %s", ev.Strategy, ev.Action)
	default:
		fmt.Fprintf(&b, "%s (%s)", ev.Type, ev.Strategy)
	}

	if ev.Rule != "" {
		fmt.Fprintf(&b, ", rule %s", ev.Rule)
	}
	if ev.Surplus != nil {
		fmt.Fprintf(&b, ", surplus %.0fW", *ev.Surplus)
	}
	if ev.PV != nil {
		fmt.Fprintf(&b, ", pv %.0fW", *ev.PV)
	}
	if ev.Threshold != nil {
		fmt.Fprintf(&b, ", threshold %.0fW", *ev.Threshold)
	}
	if ev.Energy != nil {
		fmt.Fprintf(&b, ", charged %.1fkWh", *ev.Energy)
	}
	if ev.SolarPercentage != nil {
		fmt.Fprintf(&b, ", solar %.0f%%", *ev.SolarPercentage)
	}
	if ev.ResetMode {
		b.WriteString(", mode now -> smart")
	}
	if ev.ResetAlways {
		b.WriteString(", always charge off")
	}

	return b.String()
}

// SolarControl returns the current solar control config and status
func (site *Site) SolarControl() (solarcontrol.Config, solarcontrol.Status) {
	rt := site.solarControl()
	return rt.ctrl.Config(), rt.ctrl.Status()
}

// SetSolarControlConfig applies a full or partial json config
func (site *Site) SetSolarControlConfig(b []byte) error {
	rt := site.solarControl()

	rt.cfgMu.Lock()
	defer rt.cfgMu.Unlock()

	cfg := rt.ctrl.Config()

	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cfg); err != nil {
		return &solarcontrol.ValidationError{Field: unknownJsonField(err), Code: "invalid", Msg: err.Error()}
	}

	prev := rt.ctrl.Config().Mode
	if err := rt.ctrl.SetConfig(cfg); err != nil {
		return err
	}

	if cfg.Mode != prev {
		rt.log.INFO.Printf("mode: %s -> %s", prev, cfg.Mode)
	}

	rt.persist(true)
	site.publishSolarControl(rt)

	return nil
}

// unknownJsonField extracts the field name from json decoding errors where possible
func unknownJsonField(err error) string {
	if te, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return te.Field
	}
	if f, ok := strings.CutPrefix(err.Error(), "json: unknown field "); ok {
		return strings.Trim(f, `"`)
	}
	return ""
}

// ResetSolarControl clears the daily start counter and the restart cooldown
func (site *Site) ResetSolarControl() {
	rt := site.solarControl()
	rt.ctrl.Reset(time.Now())
	rt.log.INFO.Println("reset counter and cooldown")
	rt.persist(true)
	site.publishSolarControl(rt)
}
