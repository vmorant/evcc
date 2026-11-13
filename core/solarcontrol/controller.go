package solarcontrol

import (
	"math"
	"sync"
	"time"
)

const (
	// minSharePower is the charge power below which no solar share sample is taken
	minSharePower = 200

	// applyBackoff is how long a failed action is not proposed again
	applyBackoff = 5 * time.Minute
)

// Controller evaluates the solar charging rules once per control cycle.
// All methods are safe for concurrent use.
type Controller struct {
	mu  sync.Mutex
	loc *time.Location
	cfg Config
	st  State
	rev uint64

	surplus15, pv15, pv30, share15 *window

	sunOk     bool
	startHold holdTimer // smart: surplus above start threshold
	stopHold  holdTimer // smart: always charge on while sun is low

	stopPower    holdTimer // simple A
	stopShare    holdTimer // simple B
	stopEndOfDay holdTimer // simple C

	// earliest sun start seen today, rate feeds may begin at the current slot
	sunDate  string
	sunStart time.Time

	// last known sun declining, kept while the forecast is unavailable
	sunDeclining bool

	observed      bool
	prevConnected bool
	prevAlways    string

	// shadow: action that would have been taken in the previous cycle
	lastWould Action

	// active: last action whose write failed, see Applied
	failedAction Action
	failedAt     time.Time

	evaluated bool
	lastNow   time.Time
	lastIn    Inputs
	status    Status
}

// New creates a controller from persisted config and state. loc is the local
// time zone for start hours and the daily counter, nil means time.Local.
// An invalid config is replaced by DefaultConfig, which is disabled.
func New(cfg Config, st State, loc *time.Location) *Controller {
	if loc == nil {
		loc = time.Local
	}
	if cfg.Validate() != nil {
		cfg = DefaultConfig()
	}

	st = cloneState(st)
	if len(st.Events) > MaxEvents {
		st.Events = st.Events[:MaxEvents]
	}

	return &Controller{
		loc:       loc,
		cfg:       cfg,
		st:        st,
		surplus15: newWindow(15 * time.Minute),
		pv15:      newWindow(15 * time.Minute),
		pv30:      newWindow(30 * time.Minute),
		share15:   newWindow(15 * time.Minute),
		sunOk:     true,
	}
}

// cycle holds the inputs of one evaluation and the values derived from them
type cycle struct {
	now time.Time
	in  Inputs

	pvKnown      bool
	surplusKnown bool
	surplus      float64 // W, 0 when unknown
	soc          *float64

	connectEdge    bool
	disconnectEdge bool

	startThreshold  *float64 // smart, nil without soc
	stopThreshold   *float64
	pvStopThreshold *float64

	simpleStartThreshold float64

	forecastSurplus2h *float64
	forecastCoverage  *float64
	sunDeclining      *bool
}

// proposal is the action the rules ask for, before the mode decides what happens
type proposal struct {
	action      Action
	resetMode   bool
	resetAlways bool
	event       Event
}

// Evaluate runs one cycle and returns what the caller must apply.
// Returns an action only in ModeActive. An action is not counted until the
// caller confirms it with Applied.
func (c *Controller) Evaluate(now time.Time, in Inputs) Decision {
	c.mu.Lock()
	defer c.mu.Unlock()

	in = cloneInputs(in)

	c.rollover(now)
	c.updateSunStart(now, in.Forecast)

	cy := c.derive(now, in)
	if c.observed {
		cy.connectEdge = !c.prevConnected && in.Connected
		cy.disconnectEdge = c.prevConnected && !in.Connected
	}

	c.feed(&cy)
	c.updateConditions(&cy)

	var d Decision

	switch c.cfg.Mode {
	case ModeActive:
		c.lastWould = ActionNone
		if p := c.propose(&cy); p.action != ActionNone && !c.backingOff(now, p.action) {
			d = Decision{
				Action:            p.action,
				ResetMode:         p.resetMode,
				ResetAlwaysCharge: p.resetAlways,
				Events:            []Event{cloneEvent(p.event)},
			}
		}

	case ModeShadow:
		c.external(&cy, &d)
		p := c.propose(&cy)
		if p.action != ActionNone && p.action != c.lastWould {
			ev := p.event
			ev.Type = EventShadow
			ev.Action = p.action
			c.addEvent(ev, &d)
		}
		c.lastWould = p.action

	default:
		c.lastWould = ActionNone
	}

	c.observed = true
	c.prevConnected = in.Connected
	c.prevAlways = in.AlwaysCharge

	c.evaluated = true
	c.lastNow = now
	c.lastIn = in
	c.status = c.buildStatus(&cy)

	return d
}

// derive computes the per cycle values. It must not change controller state,
// since it also refreshes the status after config changes.
func (c *Controller) derive(now time.Time, in Inputs) cycle {
	cy := cycle{
		now:     now,
		in:      in,
		pvKnown: finite(in.PV),
	}

	homeKnown := finite(in.Home)
	if cy.pvKnown && homeKnown {
		cy.surplusKnown = true
		cy.surplus = max(in.PV-in.Home, 0)
	}

	if in.Soc != nil && finite(*in.Soc) {
		cy.soc = new(*in.Soc)

		start := c.cfg.MinChargePower * c.startShare(*cy.soc) / 100
		stop := start * c.cfg.StopHysteresis / 100
		cy.startThreshold = new(start)
		cy.stopThreshold = new(stop)
		cy.pvStopThreshold = new(stop + c.cfg.BaseHomeLoad)
	}

	cy.simpleStartThreshold = c.cfg.MinChargePower * c.cfg.SimpleStartShare / 100

	if f := in.Forecast; f != nil {
		if homeKnown {
			cy.forecastSurplus2h = finitePtr(f.EnergyNext2h/2 - in.Home)
		}

		hours := 0.5
		if !f.SunEnd.IsZero() {
			hours = max(f.SunEnd.Sub(now).Hours(), hours)
		}
		cy.forecastCoverage = finitePtr(f.EnergyRestOfDay / hours / c.cfg.MinChargePower * 100)

		cy.sunDeclining = c.sunDecliningAt(now, f)
	}

	return cy
}

// startShare interpolates the start share between the low and high soc points
func (c *Controller) startShare(soc float64) float64 {
	switch {
	case soc <= c.cfg.SocLow:
		return c.cfg.StartShareLowSoc
	case soc >= c.cfg.SocHigh:
		return c.cfg.StartShareHighSoc
	default:
		frac := (soc - c.cfg.SocLow) / (c.cfg.SocHigh - c.cfg.SocLow)
		return c.cfg.StartShareLowSoc + frac*(c.cfg.StartShareHighSoc-c.cfg.StartShareLowSoc)
	}
}

func (c *Controller) updateSunStart(now time.Time, f *Forecast) {
	if f == nil || f.SunStart.IsZero() {
		return
	}
	today := now.In(c.loc).Format(dateLayout)
	if f.SunStart.In(c.loc).Format(dateLayout) != today {
		return
	}
	if c.sunDate != today || f.SunStart.Before(c.sunStart) {
		c.sunDate = today
		c.sunStart = f.SunStart
	}
}

// sunDecliningAt reports whether now is past the midpoint of today's forecast sun, nil if unknown
func (c *Controller) sunDecliningAt(now time.Time, f *Forecast) *bool {
	if f.SunEnd.IsZero() {
		return nil
	}

	start := f.SunStart
	if c.sunDate == now.In(c.loc).Format(dateLayout) && !c.sunStart.IsZero() && (start.IsZero() || c.sunStart.Before(start)) {
		start = c.sunStart
	}
	if start.IsZero() || !f.SunEnd.After(start) {
		return nil
	}

	mid := start.Add(f.SunEnd.Sub(start) / 2)
	return new(now.After(mid))
}

func (c *Controller) feed(cy *cycle) {
	now, in := cy.now, cy.in

	if cy.pvKnown {
		c.pv15.add(now, in.PV)
		c.pv30.add(now, in.PV)
	} else {
		c.pv15.prune(now)
		c.pv30.prune(now)
	}

	if cy.surplusKnown {
		c.surplus15.add(now, cy.surplus)
	} else {
		c.surplus15.prune(now)
	}

	// without charging the share window keeps its last value
	if cy.surplusKnown && finite(in.ChargePower) && in.ChargePower >= minSharePower {
		c.share15.add(now, min(cy.surplus, in.ChargePower)/in.ChargePower*100)
	}
}

// updateConditions advances sunOk and all hold timers. Unknown inputs keep the
// last known condition.
func (c *Controller) updateConditions(cy *cycle) {
	now, in := cy.now, cy.in
	on := in.AlwaysCharge == AlwaysChargeOn

	if c.pv15.valid(now) && cy.pvStopThreshold != nil {
		c.sunOk = c.pv15.mean() >= *cy.pvStopThreshold
	}
	c.stopHold.update(now, on && !c.sunOk)

	// the start hold needs continuous data, a gap between cycles starts it over
	if c.evaluated && (now.Before(c.lastNow) || now.Sub(c.lastNow) > maxSampleGap) {
		c.startHold.update(now, false)
	}
	c.startHold.update(now, c.surplusAboveStart(cy))

	if cy.sunDeclining != nil {
		c.sunDeclining = *cy.sunDeclining
	}

	c.updateSimpleTimers(cy)
}

// surplusAboveStart is smart start condition 4 without the hold. It requires
// this cycle's surplus, so the hold never runs on old samples alone.
func (c *Controller) surplusAboveStart(cy *cycle) bool {
	return cy.surplusKnown && c.surplus15.valid(cy.now) && cy.startThreshold != nil && c.surplus15.mean() > *cy.startThreshold
}

func (c *Controller) propose(cy *cycle) proposal {
	if cy.disconnectEdge {
		return c.proposeDisconnect(cy)
	}
	if c.cfg.Strategy == StrategySimple {
		return c.proposeSimple(cy)
	}
	return c.proposeSmart(cy)
}

// proposeDisconnect undoes manual overrides when the vehicle leaves. A manual
// loadpoint mode off is respected.
func (c *Controller) proposeDisconnect(cy *cycle) proposal {
	p := proposal{
		resetMode:   cy.in.LpMode == LpModeNow,
		resetAlways: cy.in.AlwaysCharge == AlwaysChargeOn,
	}
	if !p.resetMode && !p.resetAlways {
		return proposal{}
	}

	p.action = ActionDisconnect
	p.event = c.sessionEvent(cy, EventDisconnect)
	p.event.ResetMode = p.resetMode
	p.event.ResetAlways = p.resetAlways
	return p
}

// Applied reports the outcome of applying a Decision returned by Evaluate.
// Without error the action counts for the daily limit and cooldown and its
// events are recorded. With error nothing is counted or recorded and the same
// action is not proposed again for 5 minutes. Decisions without an action are ignored.
func (c *Controller) Applied(now time.Time, d Decision, err error) {
	if d.Action == ActionNone {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if err != nil {
		c.failedAction = d.Action
		c.failedAt = now
		return
	}
	c.failedAction = ActionNone

	c.rollover(now)
	switch d.Action {
	case ActionStart, ActionResume:
		c.countStart(now)
	case ActionStop:
		c.countStop(now)
	}
	for _, ev := range d.Events {
		c.record(ev)
	}

	c.refresh()
}

func (c *Controller) backingOff(now time.Time, a Action) bool {
	return a == c.failedAction && now.Sub(c.failedAt) < applyBackoff
}

// external records always charge changes made by someone else. Only used in
// shadow mode, where those changes count for the daily limit and cooldown.
func (c *Controller) external(cy *cycle, d *Decision) {
	if !c.observed {
		return
	}

	switch prev, cur := c.prevAlways, cy.in.AlwaysCharge; {
	case prev == AlwaysChargeOff && cur == AlwaysChargeOn:
		ev := c.startEvent(cy, EventExternal)
		ev.Action = ActionStart
		c.countStart(cy.now)
		c.addEvent(ev, d)

	case prev == AlwaysChargeOn && cur == AlwaysChargeOff && cy.in.Connected:
		ev := c.sessionEvent(cy, EventExternal)
		ev.Action = ActionStop
		ev.Rule = StopManualOff
		ev.PV = c.pvAverage()
		c.countStop(cy.now)
		c.addEvent(ev, d)

	case prev == AlwaysChargeOn && cur == AlwaysChargeOff:
		ev := c.sessionEvent(cy, EventExternal)
		ev.Action = ActionDisconnect
		ev.ResetAlways = true
		c.addEvent(ev, d)
	}
}

// pvAverage is the pv average the active strategy stops on, for event details.
// Nil without samples.
func (c *Controller) pvAverage() *float64 {
	w := c.pv15
	if c.cfg.Strategy == StrategySimple {
		w = c.pv30
	}
	if len(w.samples) == 0 {
		return nil
	}
	return new(w.mean())
}

func (c *Controller) newEvent(cy *cycle, typ EventType) Event {
	return Event{
		Time:     cy.now,
		Type:     typ,
		Strategy: c.cfg.Strategy,
	}
}

// sessionEvent creates an event with the details of the ending session
func (c *Controller) sessionEvent(cy *cycle, typ EventType) Event {
	ev := c.newEvent(cy, typ)
	ev.Energy = finitePtr(cy.in.SessionEnergy / 1e3)
	if p := cy.in.SessionSolarPercentage; p != nil {
		ev.SolarPercentage = finitePtr(*p)
	}
	return ev
}

func (c *Controller) startEvent(cy *cycle, typ EventType) Event {
	ev := c.newEvent(cy, typ)

	if c.cfg.Strategy == StrategySimple {
		ev.Surplus = c.pvAverage()
		ev.Threshold = new(cy.simpleStartThreshold)
		return ev
	}

	if c.surplus15.valid(cy.now) {
		ev.Surplus = new(c.surplus15.mean())
	}
	ev.Threshold = clonePtr(cy.startThreshold)
	return ev
}

func (c *Controller) stopEvent(cy *cycle, rule StopRule, threshold *float64) Event {
	ev := c.sessionEvent(cy, EventStop)
	ev.Rule = rule
	ev.PV = c.pvAverage()
	ev.Threshold = threshold
	return ev
}

// blockedByCommon checks the start conditions shared by both strategies, in reason order
func (c *Controller) blockedByCommon(cy *cycle, charging Reason) (Reason, bool) {
	in := cy.in

	switch {
	case !in.Connected:
		return Reason{Code: ReasonNotConnected}, true
	case in.AlwaysCharge == AlwaysChargeOn:
		return charging, true
	case in.LpMode == LpModeNow:
		return Reason{Code: ReasonManualNow}, true
	case in.LpMode != LpModeSmart:
		return Reason{Code: ReasonManualOff}, true
	case in.AlwaysCharge != AlwaysChargeOff:
		return Reason{Code: ReasonManualOnce}, true
	}

	return Reason{}, false
}

func (c *Controller) cooldownReason(now time.Time) (Reason, bool) {
	if cd := c.cooldownRemaining(now); cd > 0 {
		return Reason{Code: ReasonCooldown, Remaining: new(cd.Seconds())}, true
	}
	return Reason{}, false
}

func (c *Controller) buildStatus(cy *cycle) Status {
	now := cy.now

	s := Status{
		Values: Values{
			Surplus:           cy.surplus,
			SurplusAvg15:      c.surplus15.mean(),
			PvAvg15:           c.pv15.mean(),
			PvAvg30:           c.pv30.mean(),
			SolarShareAvg15:   c.share15.mean(),
			ForecastSurplus2h: cy.forecastSurplus2h,
			ForecastCoverage:  cy.forecastCoverage,
			Soc:               cy.soc,
			SunDeclining:      cy.sunDeclining,
		},
		Valid: Valid{
			SurplusAvg15:    c.surplus15.valid(now),
			PvAvg15:         c.pv15.valid(now),
			PvAvg30:         c.pv30.valid(now),
			SolarShareAvg15: c.share15.frozenValid(),
		},
		CooldownRemaining: c.cooldownRemaining(now).Seconds(),
		SunOk:             c.sunOk,
	}

	if c.cfg.Strategy == StrategySimple {
		s.Values.StartThreshold = new(cy.simpleStartThreshold)
		s.Reason = c.simpleReason(cy)
		if c.sunFirm(cy) {
			s.StartHoldRemaining = new(0.0)
		}
		s.StopHoldRemaining = c.simpleStopRemaining(now)
	} else {
		s.Values.StartThreshold = cy.startThreshold
		s.Values.StopThreshold = cy.stopThreshold
		s.Values.PvStopThreshold = cy.pvStopThreshold
		s.Reason = c.smartReason(cy)
		s.StartHoldRemaining = c.startHold.remaining(now, minutes(c.cfg.StartHoldMin))
		s.StopHoldRemaining = c.stopHold.remaining(now, minutes(c.cfg.StopHoldMin))
	}

	if c.cfg.Mode == ModeOff {
		s.Reason = Reason{Code: ReasonDisabled}
	}

	return s
}

// refresh rebuilds the status from the last inputs after config or state changes
func (c *Controller) refresh() {
	if !c.evaluated {
		return
	}
	cy := c.derive(c.lastNow, c.lastIn)
	c.status = c.buildStatus(&cy)
}

// Status returns the state published after the last evaluation
func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()

	s := c.status
	if !c.evaluated {
		s = Status{
			Reason: Reason{Code: ReasonWarmup},
			SunOk:  c.sunOk,
		}
		if c.cfg.Mode == ModeOff {
			s.Reason = Reason{Code: ReasonDisabled}
		}
	}

	s.Mode = c.cfg.Mode
	s.Strategy = c.cfg.Strategy
	s.MaxStartsPerDay = c.cfg.MaxStartsPerDay
	s.StartsToday = c.st.StartsToday
	s.LastStartAt = timePtr(c.st.LastStartAt)
	s.LastStopAt = timePtr(c.st.LastStopAt)
	s.Events = cloneEvents(c.st.Events)

	return s
}

// Config returns the current config
func (c *Controller) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// SetConfig validates and applies a new config
func (c *Controller) SetConfig(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if cfg.Mode != c.cfg.Mode || cfg.Strategy != c.cfg.Strategy {
		c.lastWould = ActionNone
	}
	c.failedAction = ActionNone
	c.cfg = cfg
	c.rev++
	c.refresh()

	return nil
}

// State returns a copy of the persistent state
func (c *Controller) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return cloneState(c.st)
}

// Revision increases whenever config or persistent state change, so callers
// know when to persist.
func (c *Controller) Revision() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rev
}

// Reset clears the daily start counter and the restart cooldown
func (c *Controller) Reset(now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.st.StartsToday = 0
	c.st.StartsDate = now.In(c.loc).Format(dateLayout)
	c.st.LastStopAt = time.Time{}
	c.failedAction = ActionNone
	c.rev++
	c.refresh()
}

// cloneInputs detaches the inputs from caller owned pointers
func cloneInputs(in Inputs) Inputs {
	in.Soc = clonePtr(in.Soc)
	in.Forecast = clonePtr(in.Forecast)
	in.SessionSolarPercentage = clonePtr(in.SessionSolarPercentage)
	return in
}

func finite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func finitePtr(f float64) *float64 {
	if !finite(f) {
		return nil
	}
	return new(f)
}
