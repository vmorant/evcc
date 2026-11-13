package solarcontrol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmartThresholds(t *testing.T) {
	for _, tc := range []struct {
		soc                 float64
		start, stop, pvStop float64
	}{
		{0, 700, 350, 700},
		{20, 700, 350, 700},
		{47.5, 770, 385, 735},
		{75, 840, 420, 770},
		{100, 840, 420, 770},
	} {
		in := sunnyInputs()
		in.Soc = new(tc.soc)
		s := newSim(t, at(10, 0), in, nil)
		s.step()

		v := s.c.Status().Values
		require.NotNil(t, v.StartThreshold, "soc %v", tc.soc)
		assert.InDelta(t, tc.start, *v.StartThreshold, 1e-9, "start soc %v", tc.soc)
		assert.InDelta(t, tc.stop, *v.StopThreshold, 1e-9, "stop soc %v", tc.soc)
		assert.InDelta(t, tc.pvStop, *v.PvStopThreshold, 1e-9, "pv stop soc %v", tc.soc)
	}
}

func TestSmartStartAfterHold(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), nil)

	// surplus average valid after 12 min, then 10 min hold
	s.run(22 * time.Minute)
	require.Empty(t, s.actions())
	assert.Equal(t, ReasonReady, s.reason())
	require.NotNil(t, s.c.Status().Reason.Remaining)
	assert.Equal(t, 30.0, *s.c.Status().Reason.Remaining)

	d := s.step()
	assert.Equal(t, ActionStart, d.Action)
	require.Len(t, d.Events, 1)

	ev := d.Events[0]
	assert.Equal(t, EventStart, ev.Type)
	assert.Equal(t, StrategySmart, ev.Strategy)
	assert.Empty(t, ev.Action)
	require.NotNil(t, ev.Surplus)
	assert.InDelta(t, 2700, *ev.Surplus, 1e-9)
	require.NotNil(t, ev.Threshold)
	assert.InDelta(t, 776.36, *ev.Threshold, 0.01)

	st := s.c.State()
	assert.Equal(t, 1, st.StartsToday)
	assert.Equal(t, at(10, 22), st.LastStartAt)
	assert.Equal(t, "2026-06-15", st.StartsDate)
}

func TestSmartReadyStartingNow(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), shadow)
	s.run(22 * time.Minute)
	require.Equal(t, 30.0, *s.c.Status().Reason.Remaining)

	s.step() // 10:22, hold satisfied
	r := s.c.Status().Reason
	assert.Equal(t, ReasonReady, r.Code)
	assert.Nil(t, r.Remaining)

	// connecting would resume right away
	s = newSim(t, at(10, 0), sunnyInputs(), shadow)
	s.in.Connected = false
	s.run(13 * time.Minute)
	s.in.Connected = true
	s.step()
	r = s.c.Status().Reason
	assert.Equal(t, ReasonReady, r.Code)
	assert.Nil(t, r.Remaining)
}

func TestSmartHoldRestartsAfterDip(t *testing.T) {
	in := sunnyInputs()
	in.Home = 2200 // surplus 800 W, just above the 776 W threshold
	s := newSim(t, at(10, 0), in, nil)

	s.run(18 * time.Minute) // hold started at 10:12
	require.Empty(t, s.actions())

	// one cycle without surplus pulls the average below the threshold
	s.in.Home = 3000
	s.step()
	assert.Equal(t, ReasonSurplusLow, s.reason())
	assert.Nil(t, s.c.Status().StartHoldRemaining)

	// compensate so the average is back above the threshold at 10:18:30
	s.in.Home = 1400
	s.step()
	s.in.Home = 2200
	require.Equal(t, ReasonReady, s.reason())

	res, ok := s.runUntilAction(30 * time.Minute)
	require.True(t, ok)
	assert.Equal(t, ActionStart, res.Action)
	assert.Equal(t, at(10, 18).Add(30*time.Second+10*time.Minute), res.at)
}

func TestSmartResumeOnConnect(t *testing.T) {
	for _, tc := range []struct {
		name      string
		connectAt time.Duration
		home      float64
		action    Action
		at        time.Duration
	}{
		{"hold elapsed", 30 * time.Minute, 300, ActionResume, 30 * time.Minute},
		{"hold pending", 13 * time.Minute, 300, ActionResume, 13 * time.Minute},
		{"surplus low at connect", 13 * time.Minute, 2900, ActionNone, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := sunnyInputs()
			in.Connected = false
			in.Home = tc.home
			s := newSim(t, at(10, 0), in, nil)

			s.run(tc.connectAt)
			require.Empty(t, s.actions())
			assert.Equal(t, ReasonNotConnected, s.reason())

			s.in.Connected = true
			res, ok := s.runUntilAction(time.Minute)
			if tc.action == ActionNone {
				assert.False(t, ok)
				return
			}

			require.True(t, ok)
			assert.Equal(t, tc.action, res.Action)
			assert.Equal(t, at(10, 0).Add(tc.at), res.at)
			require.Len(t, res.Events, 1)
			assert.Equal(t, EventResume, res.Events[0].Type)
			assert.Equal(t, 1, s.c.State().StartsToday)
		})
	}
}

func TestSmartStartHoldZero(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) { c.StartHoldMin = 0 })
	s.run(11*time.Minute + 30*time.Second)
	require.Empty(t, s.actions())

	res, ok := s.runUntilAction(time.Minute)
	require.True(t, ok)
	assert.Equal(t, ActionStart, res.Action)
	assert.Equal(t, at(10, 12), res.at)
}

func TestSmartStartConditions(t *testing.T) {
	today := State{StartsDate: "2026-06-15"}

	for _, tc := range []struct {
		name   string
		start  time.Time
		in     func(*Inputs)
		st     State
		reason ReasonCode
	}{
		{name: "all met", start: at(10, 0), reason: ReasonReady},
		{name: "1 not connected", start: at(10, 0), in: func(in *Inputs) { in.Connected = false }, reason: ReasonNotConnected},
		{name: "2 mode now", start: at(10, 0), in: func(in *Inputs) { in.LpMode = LpModeNow }, reason: ReasonManualNow},
		{name: "2 mode off", start: at(10, 0), in: func(in *Inputs) { in.LpMode = LpModeOff }, reason: ReasonManualOff},
		{name: "2 always once", start: at(10, 0), in: func(in *Inputs) { in.AlwaysCharge = AlwaysChargeOnce }, reason: ReasonManualOnce},
		{name: "3 before hours", start: at(7, 0), reason: ReasonOutsideHours},
		{name: "3 after hours", start: at(16, 45), reason: ReasonOutsideHours},
		{name: "4 surplus low", start: at(10, 0), in: func(in *Inputs) { in.Home = 2900 }, reason: ReasonSurplusLow},
		{name: "5 forecast low", start: at(10, 0), in: func(in *Inputs) { in.Forecast.EnergyNext2h = 2000 }, reason: ReasonForecastLow},
		{name: "5 forecast missing", start: at(10, 0), in: func(in *Inputs) { in.Forecast = nil }, reason: ReasonForecastMissing},
		{name: "6 cooldown", start: at(10, 0), st: State{StartsDate: "2026-06-15", LastStopAt: at(9, 45)}, reason: ReasonCooldown},
		{name: "7 daily limit", start: at(10, 0), st: State{StartsDate: "2026-06-15", StartsToday: 2}, reason: ReasonDailyLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := sunnyInputs()
			if tc.in != nil {
				tc.in(&in)
			}
			st := tc.st
			if st.StartsDate == "" {
				st = today
			}

			s := newSimState(t, tc.start, in, nil, st)
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

func TestSmartReasonValues(t *testing.T) {
	in := sunnyInputs()
	in.Forecast.EnergyNext2h = 2000
	s := newSimState(t, at(10, 0), in, nil, State{StartsDate: "2026-06-15", StartsToday: 2})
	s.run(13 * time.Minute)

	r := s.c.Status().Reason
	assert.Equal(t, ReasonDailyLimit, r.Code)
	assert.Equal(t, 2, *r.Count)
	assert.Equal(t, 2, *r.Max)

	s.c.Reset(s.now)
	r = s.c.Status().Reason
	assert.Equal(t, ReasonForecastLow, r.Code)
	assert.InDelta(t, 700, *r.Value, 1e-9)
	assert.InDelta(t, 776.36, *r.Threshold, 0.01)

	s = newSim(t, at(17, 0), sunnyInputs(), nil)
	s.step()
	r = s.c.Status().Reason
	assert.Equal(t, ReasonOutsideHours, r.Code)
	assert.Equal(t, 8, *r.From)
	assert.Equal(t, 17, *r.To)
}

func TestSmartStopSunLow(t *testing.T) {
	in := chargingInputs()
	in.PV = 500 // below the 738 W pv stop threshold
	s := newSim(t, at(15, 0), in, nil)

	// pv average valid at 15:12, sun low for 15 min
	s.run(12*time.Minute + 30*time.Second)
	st := s.c.Status()
	assert.False(t, st.SunOk)
	assert.Equal(t, ReasonCharging, st.Reason.Code)
	assert.InDelta(t, 500, *st.Reason.Value, 1e-9)
	assert.InDelta(t, 738.18, *st.Reason.Threshold, 0.01)
	assert.Equal(t, 900.0, *st.Reason.Remaining)

	res, ok := s.runUntilAction(time.Hour)
	require.True(t, ok)
	assert.Equal(t, ActionStop, res.Action)
	assert.Equal(t, at(15, 27), res.at)

	require.Len(t, res.Events, 1)
	ev := res.Events[0]
	assert.Equal(t, EventStop, ev.Type)
	assert.Equal(t, StopSunLow, ev.Rule)
	assert.InDelta(t, 500, *ev.PV, 1e-9)
	assert.InDelta(t, 738.18, *ev.Threshold, 0.01)
	assert.InDelta(t, 4.2, *ev.Energy, 1e-9)
	assert.Equal(t, 80.0, *ev.SolarPercentage)

	assert.Equal(t, res.at, s.c.State().LastStopAt)
}

func TestSmartStopManualAlwaysOn(t *testing.T) {
	// always charge switched on while the sun is already low still waits the full hold
	in := sunnyInputs()
	in.PV = 500
	s := newSim(t, at(15, 0), in, nil)
	s.run(20 * time.Minute)
	require.False(t, s.c.Status().SunOk)

	s.in = chargingInputs()
	s.in.PV = 500
	onAt := s.now

	res, ok := s.runUntilAction(time.Hour)
	require.True(t, ok)
	assert.Equal(t, ActionStop, res.Action)
	assert.Equal(t, onAt.Add(15*time.Minute), res.at)
}

func TestSmartSunOkKeepsValue(t *testing.T) {
	t.Run("soc unknown", func(t *testing.T) {
		s := newSim(t, at(12, 0), chargingInputs(), nil)
		s.run(15 * time.Minute)
		require.True(t, s.c.Status().SunOk)

		s.in.Soc = nil
		s.in.PV = 100
		s.run(30 * time.Minute)
		assert.True(t, s.c.Status().SunOk)
		assert.Empty(t, s.actions())
		assert.Nil(t, s.c.Status().Values.PvStopThreshold)
	})

	t.Run("average invalid", func(t *testing.T) {
		s := newSim(t, at(12, 0), chargingInputs(), nil)
		s.run(15 * time.Minute)
		require.True(t, s.c.Status().SunOk)

		s.in.PV = nan()
		s.run(30 * time.Minute)
		st := s.c.Status()
		assert.False(t, st.Valid.PvAvg15)
		assert.True(t, st.SunOk)
		assert.Empty(t, s.actions())
	})

	t.Run("low sun kept without soc stops", func(t *testing.T) {
		in := chargingInputs()
		in.PV = 500
		s := newSim(t, at(15, 0), in, nil)
		s.run(13 * time.Minute)
		require.False(t, s.c.Status().SunOk)

		s.in.Soc = nil
		res, ok := s.runUntilAction(time.Hour)
		require.True(t, ok)
		assert.Equal(t, ActionStop, res.Action)
		assert.Nil(t, res.Events[0].Threshold)
	})

	t.Run("initial true", func(t *testing.T) {
		in := chargingInputs()
		in.PV = 100
		s := newSim(t, at(15, 0), in, nil)
		s.run(11 * time.Minute)
		assert.True(t, s.c.Status().SunOk)
		assert.Nil(t, s.c.Status().StopHoldRemaining)
	})
}

func TestSmartHomeSpikeDoesNotStop(t *testing.T) {
	s := newSim(t, at(12, 0), chargingInputs(), nil)
	s.run(20 * time.Minute)

	s.in.Home = 6000
	s.run(20 * time.Minute)

	assert.Empty(t, s.actions())
	st := s.c.Status()
	assert.True(t, st.SunOk)
	assert.Equal(t, 0.0, st.Values.Surplus)
	assert.Nil(t, st.StopHoldRemaining)
}

func TestSmartCooldown(t *testing.T) {
	in := chargingInputs()
	in.PV = 500
	s := newSim(t, at(11, 0), in, func(c *Config) { c.StopHoldMin = 0 })
	s.autoApply = true

	stop, ok := s.runUntilAction(time.Hour)
	require.True(t, ok)
	require.Equal(t, ActionStop, stop.Action)

	// sun is back right after the stop
	s.in.PV = 3000
	s.run(25 * time.Minute)
	st := s.c.Status()
	assert.Equal(t, ReasonCooldown, st.Reason.Code)
	assert.Greater(t, st.CooldownRemaining, 0.0)
	assert.Equal(t, st.CooldownRemaining, *st.Reason.Remaining)

	start, ok := s.runUntilAction(2 * time.Hour)
	require.True(t, ok)
	assert.Equal(t, ActionStart, start.Action)
	assert.Equal(t, stop.at.Add(time.Hour), start.at)
}

func TestSmartDailyLimit(t *testing.T) {
	in := sunnyInputs()
	s := newSim(t, at(10, 0), in, func(c *Config) {
		c.RestartCooldownMin = 0
		c.StopHoldMin = 0
	})
	s.autoApply = true

	for i := range 2 {
		res, ok := s.runUntilAction(time.Hour)
		require.True(t, ok, "start %d", i)
		require.Equal(t, ActionStart, res.Action)

		s.in.PV = 300
		res, ok = s.runUntilAction(time.Hour)
		require.True(t, ok, "stop %d", i)
		require.Equal(t, ActionStop, res.Action)
		s.in.PV = 3000
	}

	_, ok := s.runUntilAction(2 * time.Hour)
	assert.False(t, ok)
	assert.Equal(t, ReasonDailyLimit, s.reason())
	assert.Equal(t, 2, s.c.Status().StartsToday)
}

func TestSmartDailyResetAtMidnight(t *testing.T) {
	start := time.Date(2026, 6, 15, 23, 40, 0, 0, madrid)
	s := newSimState(t, start, sunnyInputs(), func(c *Config) {
		c.StartHourFrom = 0
		c.StartHourTo = 24
	}, State{StartsDate: "2026-06-15", StartsToday: 2})

	s.run(20 * time.Minute)
	require.Empty(t, s.actions())
	assert.Equal(t, ReasonDailyLimit, s.reason())

	rev := s.c.Revision()
	s.step() // 00:00
	assert.Greater(t, s.c.Revision(), rev)
	st := s.c.State()
	assert.Equal(t, 0, st.StartsToday)
	assert.Equal(t, "2026-06-16", st.StartsDate)

	res, ok := s.runUntilAction(time.Hour)
	require.True(t, ok)
	assert.Equal(t, ActionStart, res.Action)
	assert.Equal(t, time.Date(2026, 6, 16, 0, 2, 0, 0, madrid), res.at)
	assert.Equal(t, 1, s.c.State().StartsToday)
}

func TestWarmupAfterRestart(t *testing.T) {
	s := newSimState(t, at(12, 0), sunnyInputs(), nil, State{StartsDate: "2026-06-15", StartsToday: 1})

	assert.Equal(t, ReasonWarmup, s.reason())

	s.run(12 * time.Minute)
	st := s.c.Status()
	assert.Equal(t, ReasonWarmup, st.Reason.Code)
	assert.False(t, st.Valid.SurplusAvg15)
	assert.False(t, st.Valid.PvAvg15)
	assert.Equal(t, 1, st.StartsToday)

	s.step() // 12:12
	st = s.c.Status()
	assert.True(t, st.Valid.SurplusAvg15)
	assert.True(t, st.Valid.PvAvg15)
	assert.False(t, st.Valid.PvAvg30)
	assert.Equal(t, ReasonReady, st.Reason.Code)

	s.run(11*time.Minute + 30*time.Second)
	assert.False(t, s.c.Status().Valid.PvAvg30)
	s.step() // 12:24
	assert.True(t, s.c.Status().Valid.PvAvg30)
}
