package solarcontrol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func simple(c *Config) {
	c.Strategy = StrategySimple
}

func TestSimpleStart(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), simple)

	// pv 30 min average valid after 24 min, no hold
	s.run(24 * time.Minute)
	require.Empty(t, s.actions())
	assert.Equal(t, ReasonWarmup, s.reason())

	d := s.step()
	require.Equal(t, ActionStart, d.Action)
	ev := d.Events[0]
	assert.Equal(t, EventStart, ev.Type)
	assert.Equal(t, StrategySimple, ev.Strategy)
	assert.InDelta(t, 3000, *ev.PV, 1e-9)
	assert.Nil(t, ev.Surplus)
	assert.InDelta(t, 700, *ev.Threshold, 1e-9)
	assert.Equal(t, 1, s.c.State().StartsToday)
}

func TestSimpleStartEventsUsePV(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) {
		c.Mode = ModeShadow
		c.Strategy = StrategySimple
	})
	s.run(30 * time.Minute)

	r := s.c.Status().Reason
	assert.Equal(t, ReasonReady, r.Code)
	assert.Nil(t, r.Remaining, "starting now")

	s.in.AlwaysCharge = AlwaysChargeOn
	s.step()

	events := s.events()
	require.Len(t, events, 2)
	assert.Equal(t, EventShadow, events[0].Type)
	assert.Equal(t, EventExternal, events[1].Type)
	for _, ev := range events {
		assert.Equal(t, ActionStart, ev.Action)
		assert.Nil(t, ev.Surplus)
		require.NotNil(t, ev.PV)
		assert.InDelta(t, 3000, *ev.PV, 1e-9)
	}
}

func TestSimpleStartConditions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		start  time.Time
		in     func(*Inputs)
		st     State
		reason ReasonCode
	}{
		{name: "all met", start: at(10, 0), reason: ReasonReady},
		{name: "no daily limit or hours", start: at(19, 0), st: State{StartsDate: "2026-06-15", StartsToday: 9}, reason: ReasonReady},
		{name: "soc not needed", start: at(10, 0), in: func(in *Inputs) { in.Soc = nil }, reason: ReasonReady},
		{name: "not connected", start: at(10, 0), in: func(in *Inputs) { in.Connected = false }, reason: ReasonNotConnected},
		{name: "mode now", start: at(10, 0), in: func(in *Inputs) { in.LpMode = LpModeNow }, reason: ReasonManualNow},
		{name: "always once", start: at(10, 0), in: func(in *Inputs) { in.AlwaysCharge = AlwaysChargeOnce }, reason: ReasonManualOnce},
		{name: "cooldown", start: at(10, 0), st: State{StartsDate: "2026-06-15", LastStopAt: at(9, 30)}, reason: ReasonCooldown},
		{name: "sun not firm", start: at(10, 0), in: func(in *Inputs) { in.PV = 700 }, reason: ReasonSurplusLow},
		{name: "coverage low", start: at(10, 0), in: func(in *Inputs) { in.Forecast.EnergyRestOfDay = 4000 }, reason: ReasonForecastLow},
		{name: "forecast missing", start: at(10, 0), in: func(in *Inputs) { in.Forecast = nil }, reason: ReasonForecastMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := sunnyInputs()
			if tc.in != nil {
				tc.in(&in)
			}

			s := newSimState(t, tc.start, in, simple, tc.st)
			res, ok := s.runUntilAction(30 * time.Minute)

			if tc.reason == ReasonReady {
				require.True(t, ok)
				assert.Equal(t, ActionStart, res.Action)
				return
			}

			assert.False(t, ok, "unexpected %s at %s", res.Action, res.at)
			assert.Equal(t, tc.reason, s.reason())
		})
	}
}

func TestSimpleReasonValues(t *testing.T) {
	in := sunnyInputs()
	in.Forecast.EnergyRestOfDay = 4000
	s := newSim(t, at(10, 0), in, simple)
	s.run(25 * time.Minute) // last cycle at 10:24:30

	r := s.c.Status().Reason
	require.Equal(t, ReasonForecastLow, r.Code)
	// 4000 Wh over 7.5925 h of sun left at 1400 W
	assert.InDelta(t, 4000/(7.0+35.5/60)/1400*100, *r.Value, 1e-9)
	assert.Equal(t, 55.0, *r.Threshold)

	v := s.c.Status().Values
	assert.InDelta(t, 700, *v.StartThreshold, 1e-9)
	assert.Nil(t, v.StopThreshold)
	assert.Nil(t, v.PvStopThreshold)

	s.in.PV = 500
	s.run(30 * time.Minute)
	r = s.c.Status().Reason
	require.Equal(t, ReasonSurplusLow, r.Code)
	assert.InDelta(t, 500, *r.Value, 1e-9)
	assert.Equal(t, 700.0, *r.Threshold)
	assert.Nil(t, s.c.Status().StartHoldRemaining)
}

func TestSimpleStops(t *testing.T) {
	for _, tc := range []struct {
		name      string
		start     time.Time
		in        func(*Inputs)
		forecast  time.Duration // forecast available for this long, -1 for always
		rule      StopRule
		after     time.Duration
		threshold *float64
	}{
		{name: "A power", start: at(10, 0), in: func(in *Inputs) { in.PV = 700 }, forecast: -1, rule: StopPower, after: 34 * time.Minute, threshold: new(800.0)},
		{name: "B share declining", start: at(14, 0), in: func(in *Inputs) { in.PV, in.Home = 1000, 600 }, forecast: -1, rule: StopShare, after: 22 * time.Minute},
		{name: "B share rising", start: at(10, 0), in: func(in *Inputs) { in.PV, in.Home = 1000, 600 }, forecast: -1},
		{name: "C end of day declining", start: at(14, 0), in: notCharging(600), forecast: -1, rule: StopEndOfDay, after: 39 * time.Minute, threshold: new(690.0)},
		{name: "C end of day rising", start: at(10, 0), in: notCharging(600), forecast: -1},
		{name: "C sun unknown", start: at(14, 0), in: notCharging(600), forecast: 0},
		{name: "C last known declining", start: at(14, 0), in: notCharging(600), forecast: 5 * time.Minute, rule: StopEndOfDay, after: 39 * time.Minute, threshold: new(690.0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := chargingInputs()
			tc.in(&in)
			forecast := in.Forecast
			if tc.forecast == 0 {
				in.Forecast = nil
			}

			s := newSim(t, tc.start, in, simple)

			var res timedDecision
			var ok bool
			for end := s.now.Add(time.Hour); s.now.Before(end) && !ok; {
				if tc.forecast >= 0 && s.now.Sub(tc.start) >= tc.forecast {
					s.in.Forecast = nil
				} else {
					s.in.Forecast = forecast
				}
				now := s.now
				if d := s.step(); d.Action != ActionNone {
					res, ok = timedDecision{now, d}, true
				}
			}

			if tc.rule == "" {
				assert.False(t, ok, "unexpected %s at %s", res.Action, res.at)
				assert.Equal(t, ReasonCharging, s.reason())
				return
			}

			require.True(t, ok)
			assert.Equal(t, ActionStop, res.Action)
			assert.Equal(t, tc.start.Add(tc.after), res.at)

			ev := res.Events[0]
			assert.Equal(t, tc.rule, ev.Rule)
			assert.Equal(t, StrategySimple, ev.Strategy)
			assert.Equal(t, tc.threshold, ev.Threshold)
			require.NotNil(t, ev.PV)
			assert.InDelta(t, in.PV, *ev.PV, 1e-9)
		})
	}
}

func notCharging(pv float64) func(*Inputs) {
	return func(in *Inputs) {
		in.PV = pv
		in.Charging = false
		in.ChargePower = 0
	}
}

func TestSimpleChargingRemaining(t *testing.T) {
	in := chargingInputs()
	in.PV, in.Home = 700, 400 // share 21 %, pv below stop power
	s := newSim(t, at(10, 0), in, simple)

	// the share timer runs from 10:12 but cannot fire before the sun declines
	s.run(24 * time.Minute)
	r := s.c.Status().Reason
	require.Equal(t, ReasonCharging, r.Code)
	assert.Nil(t, r.Remaining)
	assert.Nil(t, s.c.Status().StopHoldRemaining)
	assert.Nil(t, r.Value)
	assert.Equal(t, 800.0, *r.Threshold)

	s.step() // 10:24, pv average valid, power timer starts
	st := s.c.Status()
	assert.Equal(t, 600.0, *st.Reason.Remaining)
	assert.Equal(t, 600.0, *st.StopHoldRemaining)
	assert.InDelta(t, 700, *st.Reason.Value, 1e-9)

	s.run(9*time.Minute + 30*time.Second)
	assert.Equal(t, 30.0, *s.c.Status().Reason.Remaining)

	res, ok := s.runUntilAction(time.Minute)
	require.True(t, ok)
	assert.Equal(t, StopPower, res.Events[0].Rule)
	assert.Equal(t, at(10, 34), res.at)
}
