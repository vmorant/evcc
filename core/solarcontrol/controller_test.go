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
		field    string
		code     string
		min, max *float64
		mod      func(*Config)
	}{
		{"mode", "invalid", nil, nil, func(c *Config) { c.Mode = "on" }},
		{"strategy", "invalid", nil, nil, func(c *Config) { c.Strategy = "" }},
		{"minChargePower", "range", new(500.0), new(11000.0), func(c *Config) { c.MinChargePower = 499 }},
		{"minChargePower", "range", new(500.0), new(11000.0), func(c *Config) { c.MinChargePower = math.NaN() }},
		{"restartCooldownMin", "range", new(0.0), new(120.0), func(c *Config) { c.RestartCooldownMin = 121 }},
		{"socLow", "range", new(0.0), new(100.0), func(c *Config) { c.SocLow = -1 }},
		{"socHigh", "order", nil, nil, func(c *Config) { c.SocHigh = c.SocLow }},
		{"startShareHighSoc", "range", new(10.0), new(150.0), func(c *Config) { c.StartShareHighSoc = 151 }},
		{"stopHysteresis", "range", new(10.0), new(100.0), func(c *Config) { c.StopHysteresis = 5 }},
		{"baseHomeLoad", "range", new(100.0), new(1000.0), func(c *Config) { c.BaseHomeLoad = math.Inf(1) }},
		{"maxStartsPerDay", "range", new(1.0), new(10.0), func(c *Config) { c.MaxStartsPerDay = 0 }},
		{"startHourFrom", "range", new(0.0), new(24.0), func(c *Config) { c.StartHourFrom = -1 }},
		{"startHourTo", "range", new(0.0), new(24.0), func(c *Config) { c.StartHourTo = 25 }},
		{"startHourTo", "order", nil, nil, func(c *Config) { c.StartHourTo = c.StartHourFrom }},
		{"startHoldMin", "range", new(0.0), new(60.0), func(c *Config) { c.StartHoldMin = 61 }},
		{"stopHoldMin", "range", new(0.0), new(60.0), func(c *Config) { c.StopHoldMin = -1 }},
		{"simpleForecastShare", "range", new(10.0), new(100.0), func(c *Config) { c.SimpleForecastShare = 101 }},
		{"simpleEndOfDayPower", "range", new(0.0), new(2000.0), func(c *Config) { c.SimpleEndOfDayPower = 2001 }},
	} {
		cfg := DefaultConfig()
		tc.mod(&cfg)

		var verr *ValidationError
		require.ErrorAs(t, cfg.Validate(), &verr, tc.field)
		assert.Equal(t, tc.field, verr.Field)
		assert.Equal(t, tc.code, verr.Code, tc.field)
		assert.Equal(t, tc.min, verr.Min, tc.field)
		assert.Equal(t, tc.max, verr.Max, tc.field)
		assert.NotEmpty(t, verr.Msg, tc.field)
	}
}

func TestValidationErrorJSON(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxStartsPerDay = 11
	b, err := json.Marshal(cfg.Validate())
	require.NoError(t, err)
	assert.JSONEq(t, `{"field":"maxStartsPerDay","code":"range","error":"must be between 1 and 10","min":1,"max":10}`, string(b))

	cfg = DefaultConfig()
	cfg.SocHigh = cfg.SocLow
	b, err = json.Marshal(cfg.Validate())
	require.NoError(t, err)
	assert.JSONEq(t, `{"field":"socHigh","code":"order","error":"must be greater than socLow"}`, string(b))
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
	st := State{StartsDate: "2026-06-15", StartsToday: 1, LastStartAt: at(9, 0)}

	c := New(cfg, st, madrid)
	c.clock = func() time.Time { return at(9, 30) }
	s := c.Status()
	assert.Equal(t, ReasonWarmup, s.Reason.Code)
	assert.Equal(t, ModeShadow, s.Mode)
	assert.Equal(t, StrategySimple, s.Strategy)
	assert.Equal(t, 2, s.MaxStartsPerDay)
	assert.Equal(t, 1, s.StartsToday)
	assert.Equal(t, at(9, 0), *s.LastStartAt)
	assert.True(t, s.SunOk)
	assert.NotNil(t, s.Events)
}

func TestStatusStartsTodayOfOtherDay(t *testing.T) {
	st := State{StartsDate: "2026-06-14", StartsToday: 2, LastStartAt: time.Date(2026, 6, 14, 12, 0, 0, 0, madrid)}
	c := New(DefaultConfig(), st, madrid)
	c.clock = func() time.Time { return at(7, 0) }

	assert.Equal(t, 0, c.Status().StartsToday)
	assert.Equal(t, 2, c.State().StartsToday, "state untouched until evaluated")

	c.clock = func() time.Time { return time.Date(2026, 6, 14, 23, 0, 0, 0, madrid) }
	assert.Equal(t, 2, c.Status().StartsToday)

	// the day is taken in the controller's time zone
	c.clock = func() time.Time { return time.Date(2026, 6, 14, 22, 30, 0, 0, time.UTC) }
	assert.Equal(t, 0, c.Status().StartsToday)
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
