package solarcontrol

import (
	"math"
	"testing"
	"time"
	_ "time/tzdata"

	"github.com/stretchr/testify/require"
)

const cycleLen = 30 * time.Second

var madrid = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		panic(err)
	}
	return loc
}()

// at returns a local time on the reference test day
func at(h, m int) time.Time {
	return time.Date(2026, 6, 15, h, m, 0, 0, madrid)
}

func sunnyForecast() *Forecast {
	return &Forecast{
		EnergyNext2h:    6000,
		EnergyRestOfDay: 15000,
		SunStart:        at(8, 0),
		SunEnd:          at(18, 0),
	}
}

// sunnyInputs can start a smart charge: surplus 2700 W, soc 50 % (start threshold ~776 W)
func sunnyInputs() Inputs {
	return Inputs{
		PV:           3000,
		Home:         300,
		Soc:          new(50.0),
		Connected:    true,
		LpMode:       LpModeSmart,
		AlwaysCharge: AlwaysChargeOff,
		Forecast:     sunnyForecast(),
	}
}

// chargingInputs is a running always charge session
func chargingInputs() Inputs {
	in := sunnyInputs()
	in.AlwaysCharge = AlwaysChargeOn
	in.Charging = true
	in.ChargePower = 1400
	in.SessionEnergy = 4200
	in.SessionSolarPercentage = new(80.0)
	return in
}

type timedDecision struct {
	at time.Time
	Decision
}

// sim drives a controller in 30 s cycles
type sim struct {
	t      *testing.T
	c      *Controller
	now    time.Time
	in     Inputs
	evalAt time.Time // time of the last evaluation, the controller's clock

	// autoApply applies returned actions to the inputs like the loadpoint would
	autoApply bool

	// applyErr is reported to Applied for every returned action
	applyErr error

	log []timedDecision // cycles that returned an action or events
}

func newSim(t *testing.T, start time.Time, in Inputs, mutate func(*Config)) *sim {
	return newSimState(t, start, in, mutate, State{})
}

func newSimState(t *testing.T, start time.Time, in Inputs, mutate func(*Config), st State) *sim {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Mode = ModeActive
	if mutate != nil {
		mutate(&cfg)
	}
	require.NoError(t, cfg.Validate())

	s := &sim{t: t, c: New(cfg, st, madrid), now: start, in: in, evalAt: start}
	s.c.clock = func() time.Time { return s.evalAt }
	return s
}

// step evaluates one cycle at the current time and advances the clock
func (s *sim) step() Decision {
	s.evalAt = s.now
	d := s.c.Evaluate(s.now, s.in)

	if d.Action != ActionNone || len(d.Events) > 0 {
		s.log = append(s.log, timedDecision{s.now, d})
	}

	if d.Action != ActionNone {
		s.c.Applied(s.now, d, s.applyErr)
	}

	if s.autoApply && s.applyErr == nil {
		switch d.Action {
		case ActionStart, ActionResume:
			s.in.AlwaysCharge = AlwaysChargeOn
			s.in.Charging = true
			s.in.ChargePower = 1400
		case ActionStop:
			s.in.AlwaysCharge = AlwaysChargeOff
			s.in.Charging = false
			s.in.ChargePower = 0
		case ActionDisconnect:
			if d.ResetMode {
				s.in.LpMode = LpModeSmart
			}
			if d.ResetAlwaysCharge {
				s.in.AlwaysCharge = AlwaysChargeOff
			}
		}
	}

	s.now = s.now.Add(cycleLen)
	return d
}

// run steps for the given duration
func (s *sim) run(d time.Duration) {
	for end := s.now.Add(d); s.now.Before(end); {
		s.step()
	}
}

// runUntilAction steps until an action is returned or the duration is exhausted
func (s *sim) runUntilAction(d time.Duration) (timedDecision, bool) {
	for end := s.now.Add(d); s.now.Before(end); {
		now := s.now
		if res := s.step(); res.Action != ActionNone {
			return timedDecision{now, res}, true
		}
	}
	return timedDecision{}, false
}

func (s *sim) actions() []timedDecision {
	var res []timedDecision
	for _, d := range s.log {
		if d.Action != ActionNone {
			res = append(res, d)
		}
	}
	return res
}

func (s *sim) events() []Event {
	var res []Event
	for _, d := range s.log {
		res = append(res, d.Events...)
	}
	return res
}

func (s *sim) reason() ReasonCode {
	return s.c.Status().Reason.Code
}

func nan() float64 {
	return math.NaN()
}
