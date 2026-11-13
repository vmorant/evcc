package solarcontrol

import (
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigValid(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, cfg.Validate())
	assert.Equal(t, ModeOff, cfg.Mode)
	assert.Equal(t, StrategySmart, cfg.Strategy)
}

func TestValidate(t *testing.T) {
	for _, tc := range []struct {
		field string
		mod   func(*Config)
	}{
		{"mode", func(c *Config) { c.Mode = "on" }},
		{"strategy", func(c *Config) { c.Strategy = "" }},
		{"minChargePower", func(c *Config) { c.MinChargePower = 499 }},
		{"minChargePower", func(c *Config) { c.MinChargePower = math.NaN() }},
		{"restartCooldownMin", func(c *Config) { c.RestartCooldownMin = 121 }},
		{"socLow", func(c *Config) { c.SocLow = -1 }},
		{"socHigh", func(c *Config) { c.SocHigh = c.SocLow }},
		{"startShareHighSoc", func(c *Config) { c.StartShareHighSoc = 151 }},
		{"stopHysteresis", func(c *Config) { c.StopHysteresis = 5 }},
		{"baseHomeLoad", func(c *Config) { c.BaseHomeLoad = math.Inf(1) }},
		{"maxStartsPerDay", func(c *Config) { c.MaxStartsPerDay = 0 }},
		{"startHourFrom", func(c *Config) { c.StartHourFrom = -1 }},
		{"startHourTo", func(c *Config) { c.StartHourTo = 25 }},
		{"startHourTo", func(c *Config) { c.StartHourTo = c.StartHourFrom }},
		{"startHoldMin", func(c *Config) { c.StartHoldMin = 61 }},
		{"stopHoldMin", func(c *Config) { c.StopHoldMin = -1 }},
		{"simpleForecastShare", func(c *Config) { c.SimpleForecastShare = 101 }},
		{"simpleEndOfDayPower", func(c *Config) { c.SimpleEndOfDayPower = 2001 }},
	} {
		cfg := DefaultConfig()
		tc.mod(&cfg)

		var verr *ValidationError
		require.ErrorAs(t, cfg.Validate(), &verr, tc.field)
		assert.Equal(t, tc.field, verr.Field)
	}
}

func TestNewInvalidConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeActive
	cfg.SocHigh = 0

	c := New(cfg, State{}, madrid)
	assert.Equal(t, DefaultConfig(), c.Config())
}

func TestNewTrimsEvents(t *testing.T) {
	st := State{Events: make([]Event, MaxEvents+5)}
	c := New(DefaultConfig(), st, nil)
	assert.Len(t, c.State().Events, MaxEvents)
}

func TestStatusJSON(t *testing.T) {
	c := New(DefaultConfig(), State{}, madrid)

	for range 2 {
		b, err := json.Marshal(c.Status())
		require.NoError(t, err)
		assert.Contains(t, string(b), `"events":[]`)
		assert.Contains(t, string(b), `"lastStartAt":null`)
		assert.Contains(t, string(b), `"reason":{"code":"disabled"}`)

		c.Evaluate(at(10, 0), Inputs{PV: math.NaN(), Home: math.NaN(), ChargePower: math.Inf(1)})
	}
}

func TestStatusBeforeEvaluate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeShadow
	cfg.Strategy = StrategySimple
	st := State{StartsToday: 1, LastStartAt: at(9, 0)}

	s := New(cfg, st, madrid).Status()
	assert.Equal(t, ReasonWarmup, s.Reason.Code)
	assert.Equal(t, ModeShadow, s.Mode)
	assert.Equal(t, StrategySimple, s.Strategy)
	assert.Equal(t, 2, s.MaxStartsPerDay)
	assert.Equal(t, 1, s.StartsToday)
	assert.Equal(t, at(9, 0), *s.LastStartAt)
	assert.True(t, s.SunOk)
	assert.NotNil(t, s.Events)
}

func TestSetConfig(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) { c.Mode = ModeOff })
	s.run(15 * time.Minute)
	rev := s.c.Revision()

	cfg := s.c.Config()
	cfg.SocHigh = 10
	err := s.c.SetConfig(cfg)
	var verr *ValidationError
	require.ErrorAs(t, err, &verr)
	assert.Equal(t, "socHigh", verr.Field)
	assert.Equal(t, 75.0, s.c.Config().SocHigh)
	assert.Equal(t, rev, s.c.Revision())

	cfg = s.c.Config()
	cfg.Mode = ModeShadow
	cfg.MaxStartsPerDay = 5
	require.NoError(t, s.c.SetConfig(cfg))
	assert.Greater(t, s.c.Revision(), rev)

	st := s.c.Status()
	assert.Equal(t, ModeShadow, st.Mode)
	assert.Equal(t, 5, st.MaxStartsPerDay)
	assert.Equal(t, ReasonReady, st.Reason.Code)
}

func TestRevision(t *testing.T) {
	in := sunnyInputs()
	in.Home = 3000
	s := newSim(t, at(10, 0), in, shadow)
	s.step()
	rev := s.c.Revision()

	s.run(10 * time.Minute)
	assert.Equal(t, rev, s.c.Revision(), "quiet cycles")

	s.in.AlwaysCharge = AlwaysChargeOn
	s.step()
	assert.Greater(t, s.c.Revision(), rev, "external event")
	rev = s.c.Revision()

	s.c.Reset(s.now)
	assert.Greater(t, s.c.Revision(), rev, "reset")
}

func TestReset(t *testing.T) {
	st := State{StartsDate: "2026-06-15", StartsToday: 2, LastStartAt: at(9, 0), LastStopAt: at(9, 50)}
	s := newSimState(t, at(10, 0), sunnyInputs(), nil, st)
	s.run(15 * time.Minute)
	require.Equal(t, ReasonDailyLimit, s.reason())

	s.c.Reset(s.now)

	state := s.c.State()
	assert.Equal(t, 0, state.StartsToday)
	assert.True(t, state.LastStopAt.IsZero())
	assert.Equal(t, at(9, 0), state.LastStartAt)

	status := s.c.Status()
	assert.Equal(t, 0.0, status.CooldownRemaining)
	assert.Equal(t, ReasonReady, status.Reason.Code)
	assert.Nil(t, status.LastStopAt)
}

func TestStateIsCopy(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), shadow)
	s.run(30 * time.Minute)

	st := s.c.State()
	require.Len(t, st.Events, 1)
	*st.Events[0].Surplus = -1
	st.Events[0].Type = EventStop

	ev := s.c.State().Events[0]
	assert.Equal(t, EventShadow, ev.Type)
	assert.InDelta(t, 2700, *ev.Surplus, 1e-9)

	status := s.c.Status()
	status.Events[0].Type = EventStop
	assert.Equal(t, EventShadow, s.c.Status().Events[0].Type)
}

func TestEventsCapped(t *testing.T) {
	in := sunnyInputs()
	in.Home = 3000
	s := newSim(t, at(10, 0), in, shadow)
	s.step()

	for i := range MaxEvents + 10 {
		if i%2 == 0 {
			s.in.AlwaysCharge = AlwaysChargeOn
		} else {
			s.in.AlwaysCharge = AlwaysChargeOff
		}
		s.step()
	}

	events := s.c.State().Events
	require.Len(t, events, MaxEvents)
	assert.True(t, events[0].Time.After(events[1].Time), "newest first")
	assert.Equal(t, s.now.Add(-cycleLen), events[0].Time)
}

func TestConcurrentUse(t *testing.T) {
	c := New(DefaultConfig(), State{}, madrid)
	done := make(chan struct{})

	go func() {
		defer close(done)
		for i := range 500 {
			c.Evaluate(at(10, 0).Add(time.Duration(i)*cycleLen), sunnyInputs())
		}
	}()

	for range 500 {
		_ = c.Status()
		_ = c.State()
		_ = c.Revision()
		cfg := c.Config()
		cfg.Mode = ModeShadow
		_ = c.SetConfig(cfg)
	}
	c.Reset(at(10, 0))
	<-done
}

func TestValidationErrorMessage(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxStartsPerDay = 11
	err := cfg.Validate()

	var verr *ValidationError
	require.True(t, errors.As(err, &verr))
	assert.Equal(t, "maxStartsPerDay: must be between 1 and 10", err.Error())
}
