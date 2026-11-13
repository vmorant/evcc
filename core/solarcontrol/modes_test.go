package solarcontrol

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func shadow(c *Config) {
	c.Mode = ModeShadow
}

func eventsOf(events []Event, typ EventType) []Event {
	var res []Event
	for _, ev := range events {
		if ev.Type == typ {
			res = append(res, ev)
		}
	}
	return res
}

func TestShadowOneEventPerTransition(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), shadow)

	// would start from 10:22 on, every cycle
	s.run(time.Hour)
	assert.Empty(t, s.actions())
	ev := eventsOf(s.events(), EventShadow)
	require.Len(t, ev, 1)
	assert.Equal(t, ActionStart, ev[0].Action)
	assert.Equal(t, at(10, 22), ev[0].Time)
	assert.InDelta(t, 2700, *ev[0].Surplus, 1e-9)
	assert.InDelta(t, 776.36, *ev[0].Threshold, 0.01)

	st := s.c.State()
	assert.Equal(t, 0, st.StartsToday)
	assert.True(t, st.LastStartAt.IsZero())
	require.Len(t, st.Events, 1)

	// surplus gone and back is a new transition
	s.in.Home = 3000
	s.run(20 * time.Minute)
	s.in.Home = 300
	s.run(time.Hour)

	assert.Empty(t, s.actions())
	ev = eventsOf(s.events(), EventShadow)
	require.Len(t, ev, 2)
	assert.Equal(t, ActionStart, ev[1].Action)
	assert.Equal(t, 0, s.c.State().StartsToday)
	assert.Equal(t, ReasonReady, s.reason())
}

func TestShadowWouldActions(t *testing.T) {
	t.Run("stop", func(t *testing.T) {
		in := chargingInputs()
		in.PV = 500
		s := newSim(t, at(15, 0), in, shadow)
		s.run(time.Hour)

		assert.Empty(t, s.actions())
		ev := s.events()
		require.Len(t, ev, 1)
		assert.Equal(t, EventShadow, ev[0].Type)
		assert.Equal(t, ActionStop, ev[0].Action)
		assert.Equal(t, StopSunLow, ev[0].Rule)
		assert.Equal(t, at(15, 27), ev[0].Time)
		assert.InDelta(t, 4.2, *ev[0].Energy, 1e-9)
		assert.True(t, s.c.State().LastStopAt.IsZero())
	})

	t.Run("resume", func(t *testing.T) {
		in := sunnyInputs()
		in.Connected = false
		s := newSim(t, at(10, 0), in, shadow)
		s.run(15 * time.Minute)
		s.in.Connected = true
		d := s.step()

		assert.Equal(t, ActionNone, d.Action)
		require.Len(t, d.Events, 1)
		assert.Equal(t, ActionResume, d.Events[0].Action)

		// the would start continues as a start, which is a new transition
		d = s.step()
		require.Len(t, d.Events, 0)
		s.run(10 * time.Minute)
		ev := eventsOf(s.events(), EventShadow)
		require.Len(t, ev, 2)
		assert.Equal(t, ActionStart, ev[1].Action)
	})

	t.Run("disconnect", func(t *testing.T) {
		in := chargingInputs()
		in.LpMode = LpModeNow
		s := newSim(t, at(12, 0), in, shadow)
		s.run(5 * time.Minute)

		s.in.Connected = false
		d := s.step()
		assert.Equal(t, ActionNone, d.Action)
		require.Len(t, d.Events, 1)
		ev := d.Events[0]
		assert.Equal(t, EventShadow, ev.Type)
		assert.Equal(t, ActionDisconnect, ev.Action)
		assert.True(t, ev.ResetMode)
		assert.True(t, ev.ResetAlways)

		s.run(10 * time.Minute)
		assert.Len(t, s.events(), 1)
	})
}

func TestShadowExternalTransitions(t *testing.T) {
	in := sunnyInputs()
	in.Home = 3000 // no own would actions
	s := newSim(t, at(10, 0), in, shadow)
	s.run(time.Minute)

	// someone switches always charge on
	s.in.AlwaysCharge = AlwaysChargeOn
	onAt := s.now
	d := s.step()
	assert.Equal(t, ActionNone, d.Action)
	require.Len(t, d.Events, 1)
	assert.Equal(t, EventExternal, d.Events[0].Type)
	assert.Equal(t, ActionStart, d.Events[0].Action)
	st := s.c.State()
	assert.Equal(t, 1, st.StartsToday)
	assert.Equal(t, onAt, st.LastStartAt)

	// and off again while connected
	s.run(time.Minute)
	s.in.AlwaysCharge = AlwaysChargeOff
	offAt := s.now
	d = s.step()
	require.Len(t, d.Events, 1)
	assert.Equal(t, ActionStop, d.Events[0].Action)
	assert.Equal(t, StopManualOff, d.Events[0].Rule)
	assert.Equal(t, offAt, s.c.State().LastStopAt)
	assert.Equal(t, ReasonCooldown, s.reason())

	// transitions through once are ignored
	for _, ac := range []string{AlwaysChargeOnce, AlwaysChargeOn, AlwaysChargeOnce, AlwaysChargeOff} {
		s.in.AlwaysCharge = ac
		d = s.step()
		assert.Empty(t, d.Events, ac)
	}

	// on, then off in the disconnect cycle, is no solar stop
	s.in.AlwaysCharge = AlwaysChargeOn
	s.step()
	s.in.AlwaysCharge = AlwaysChargeOff
	s.in.Connected = false
	d = s.step()
	require.Len(t, d.Events, 1)
	assert.Equal(t, EventExternal, d.Events[0].Type)
	assert.Equal(t, ActionDisconnect, d.Events[0].Action)

	st = s.c.State()
	assert.Equal(t, 2, st.StartsToday)
	assert.Equal(t, offAt, st.LastStopAt)
	assert.Len(t, eventsOf(st.Events, EventExternal), 4)
	assert.Empty(t, eventsOf(st.Events, EventShadow))
}

func TestActiveIgnoresExternal(t *testing.T) {
	in := sunnyInputs()
	in.Home = 3000
	s := newSim(t, at(10, 0), in, nil)
	s.run(time.Minute)

	for _, ac := range []string{AlwaysChargeOn, AlwaysChargeOff, AlwaysChargeOn, AlwaysChargeOff} {
		s.in.AlwaysCharge = ac
		s.run(time.Minute)
	}

	assert.Empty(t, s.log)
	st := s.c.State()
	assert.Equal(t, 0, st.StartsToday)
	assert.True(t, st.LastStartAt.IsZero())
	assert.True(t, st.LastStopAt.IsZero())
}

func TestOffNoWrites(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) { c.Mode = ModeOff })

	// would start
	s.run(time.Hour)
	st := s.c.Status()
	assert.Equal(t, ReasonDisabled, st.Reason.Code)
	assert.True(t, st.Valid.SurplusAvg15)
	assert.InDelta(t, 2700, st.Values.SurplusAvg15, 1e-9)
	assert.NotNil(t, st.Values.StartThreshold)

	// would stop
	s.in = chargingInputs()
	s.in.PV = 300
	s.run(time.Hour)

	// would disconnect, external transitions
	s.in.LpMode = LpModeNow
	s.step()
	s.in.Connected = false
	s.step()
	s.in.AlwaysCharge = AlwaysChargeOff
	s.step()

	// simple strategy as well
	cfg := s.c.Config()
	cfg.Strategy = StrategySimple
	require.NoError(t, s.c.SetConfig(cfg))
	s.in = sunnyInputs()
	s.run(time.Hour)

	assert.Empty(t, s.log)
	state := s.c.State()
	assert.Equal(t, 0, state.StartsToday)
	assert.True(t, state.LastStartAt.IsZero())
	assert.True(t, state.LastStopAt.IsZero())
	assert.Empty(t, state.Events)
	assert.Equal(t, ReasonDisabled, s.reason())
}

func TestOffKeepsAveragesWarm(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) { c.Mode = ModeOff })
	s.run(30 * time.Minute)
	require.Equal(t, ReasonDisabled, s.reason())

	cfg := s.c.Config()
	cfg.Mode = ModeShadow
	require.NoError(t, s.c.SetConfig(cfg))
	assert.Equal(t, ReasonReady, s.reason())

	d := s.step()
	require.Len(t, d.Events, 1)
	assert.Equal(t, ActionStart, d.Events[0].Action)
}

func TestNeverStartsUnlessSmart(t *testing.T) {
	for _, strategy := range []Strategy{StrategySmart, StrategySimple} {
		for _, tc := range []struct{ mode, always string }{
			{LpModeNow, AlwaysChargeOff},
			{LpModeOff, AlwaysChargeOff},
			{"", AlwaysChargeOff},
			{"pv", AlwaysChargeOff},
			{LpModeSmart, AlwaysChargeOnce},
			{LpModeSmart, ""},
		} {
			in := sunnyInputs()
			in.LpMode = tc.mode
			in.AlwaysCharge = tc.always
			s := newSim(t, at(10, 0), in, func(c *Config) { c.Strategy = strategy })

			s.run(40 * time.Minute)
			s.in.Connected = false
			s.step()
			s.in.Connected = true
			s.run(40 * time.Minute)

			for _, d := range s.actions() {
				assert.NotContains(t, []Action{ActionStart, ActionResume}, d.Action, "%s %+v", strategy, tc)
			}
		}
	}
}

func TestNeverStartsWithMissingData(t *testing.T) {
	cases := []struct {
		name       string
		strategies []Strategy
		in         func(*Inputs)
		reason     ReasonCode
	}{
		{"soc", []Strategy{StrategySmart}, func(in *Inputs) { in.Soc = nil }, ReasonSocUnknown},
		{"soc nan", []Strategy{StrategySmart}, func(in *Inputs) { in.Soc = new(math.NaN()) }, ReasonSocUnknown},
		{"pv nan", []Strategy{StrategySmart, StrategySimple}, func(in *Inputs) { in.PV = math.NaN() }, ReasonWarmup},
		{"pv inf", []Strategy{StrategySmart, StrategySimple}, func(in *Inputs) { in.PV = math.Inf(1) }, ReasonWarmup},
		{"home nan", []Strategy{StrategySmart}, func(in *Inputs) { in.Home = math.NaN() }, ReasonWarmup},
		{"forecast", []Strategy{StrategySmart, StrategySimple}, func(in *Inputs) { in.Forecast = nil }, ReasonForecastMissing},
	}

	// connected and duration before the data goes missing
	phases := []struct {
		name      string
		connected bool
		before    time.Duration
	}{
		{"from start", false, 0},
		{"then connect", false, 40 * time.Minute},
		{"while waiting", true, 15 * time.Minute},
	}

	for _, ph := range phases {
		for _, tc := range cases {
			for _, strategy := range tc.strategies {
				t.Run(ph.name+" "+tc.name+" "+string(strategy), func(t *testing.T) {
					in := sunnyInputs()
					in.Connected = ph.connected
					s := newSim(t, at(10, 0), in, func(c *Config) { c.Strategy = strategy })
					s.run(ph.before)
					require.Empty(t, s.actions())

					tc.in(&s.in)
					if ph.before == 0 {
						s.run(40 * time.Minute)
					}
					s.in.Connected = true
					s.run(40 * time.Minute)

					assert.Empty(t, s.actions())
					assert.Equal(t, tc.reason, s.reason())
				})
			}
		}
	}
}

func TestMissingDataStopsHoldImmediately(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   func(*Inputs)
	}{
		{"pv", func(in *Inputs) { in.PV = math.NaN() }},
		{"home", func(in *Inputs) { in.Home = math.NaN() }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSim(t, at(10, 0), sunnyInputs(), nil)
			s.run(15 * time.Minute) // hold runs since 10:12
			require.NotNil(t, s.c.Status().StartHoldRemaining)

			tc.in(&s.in)
			s.step()
			st := s.c.Status()
			assert.Nil(t, st.StartHoldRemaining)
			assert.Equal(t, ReasonWarmup, st.Reason.Code)

			// data back: the hold starts over
			s.in = sunnyInputs()
			res, ok := s.runUntilAction(time.Hour)
			require.True(t, ok)
			assert.Equal(t, ActionStart, res.Action)
			assert.Equal(t, at(10, 25).Add(30*time.Second), res.at)
		})
	}
}

func TestStartHoldResetsAfterGap(t *testing.T) {
	for _, tc := range []struct {
		name  string
		gap   time.Duration
		start time.Duration // after the first cycle back
	}{
		{"within limit", 2 * time.Minute, 30 * time.Second},
		// averages stay valid across a short gap, only the hold starts over
		{"short", 3 * time.Minute, 10 * time.Minute},
		// averages need 12 min of new samples first
		{"long", 11 * time.Minute, 22 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newSim(t, at(10, 0), sunnyInputs(), nil)
			s.run(20 * time.Minute) // hold runs since 10:12

			s.now = s.now.Add(tc.gap - cycleLen)
			back := s.now
			res, ok := s.runUntilAction(time.Hour)
			require.True(t, ok)
			assert.Equal(t, ActionStart, res.Action)
			assert.Equal(t, back.Add(tc.start), res.at)
		})
	}
}

func TestUnknownPVAgesOut(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), func(c *Config) { c.StartHoldMin = 30 })
	s.run(20 * time.Minute)
	require.True(t, s.c.Status().Valid.PvAvg15)

	s.in.PV = math.NaN()
	s.run(2*time.Minute + 30*time.Second)

	st := s.c.Status()
	assert.False(t, st.Valid.SurplusAvg15)
	assert.False(t, st.Valid.PvAvg15)
	assert.False(t, st.Valid.PvAvg30)
	assert.Equal(t, ReasonWarmup, st.Reason.Code)
	assert.Nil(t, st.StartHoldRemaining)

	s.run(15 * time.Minute)
	assert.Empty(t, s.actions())
}

func TestDisconnect(t *testing.T) {
	t.Run("only disconnect when a stop is due", func(t *testing.T) {
		in := chargingInputs()
		in.PV = 500
		in.LpMode = LpModeNow
		s := newSim(t, at(15, 0), in, nil)
		s.autoApply = true

		// the stop would fire at 15:27
		s.run(27 * time.Minute)
		require.Empty(t, s.actions())

		s.in.Connected = false
		d := s.step()
		assert.Equal(t, ActionDisconnect, d.Action)
		assert.True(t, d.ResetMode)
		assert.True(t, d.ResetAlwaysCharge)
		require.Len(t, d.Events, 1)
		assert.Equal(t, EventDisconnect, d.Events[0].Type)
		assert.True(t, d.Events[0].ResetMode)
		assert.True(t, d.Events[0].ResetAlways)

		s.run(time.Hour)
		assert.Len(t, s.actions(), 1)
		assert.True(t, s.c.State().LastStopAt.IsZero(), "disconnect is no solar stop")
	})

	t.Run("mode now only", func(t *testing.T) {
		in := sunnyInputs()
		in.LpMode = LpModeNow
		s := newSim(t, at(10, 0), in, nil)
		s.run(30 * time.Minute)

		s.in.Connected = false
		d := s.step()
		assert.Equal(t, ActionDisconnect, d.Action)
		assert.True(t, d.ResetMode)
		assert.False(t, d.ResetAlwaysCharge)
	})

	t.Run("manual off is respected", func(t *testing.T) {
		in := sunnyInputs()
		in.LpMode = LpModeOff
		s := newSim(t, at(10, 0), in, nil)
		s.run(5 * time.Minute)

		s.in.Connected = false
		s.run(5 * time.Minute)
		assert.Empty(t, s.log)
	})

	t.Run("simple strategy", func(t *testing.T) {
		s := newSim(t, at(10, 0), chargingInputs(), simple)
		s.run(5 * time.Minute)

		s.in.Connected = false
		d := s.step()
		assert.Equal(t, ActionDisconnect, d.Action)
		assert.False(t, d.ResetMode)
		assert.True(t, d.ResetAlwaysCharge)
	})
}

func TestOddInputsNeverPanic(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	odd := []float64{0, -500, 300, 3000, 1e9, math.NaN(), math.Inf(1), math.Inf(-1)}
	pick := func() float64 { return odd[r.IntN(len(odd))] }
	modes := []string{LpModeOff, LpModeNow, LpModeSmart, "", "pv"}
	always := []string{AlwaysChargeOff, AlwaysChargeOn, AlwaysChargeOnce, ""}

	for _, strategy := range []Strategy{StrategySmart, StrategySimple} {
		for _, mode := range []Mode{ModeOff, ModeShadow, ModeActive} {
			cfg := DefaultConfig()
			cfg.Mode, cfg.Strategy = mode, strategy
			cfg.StartHoldMin, cfg.StopHoldMin, cfg.RestartCooldownMin = 0, 0, 0
			c := New(cfg, State{}, madrid)

			now := at(6, 0)
			for range 5000 {
				in := Inputs{
					PV:            pick(),
					Home:          pick(),
					Connected:     r.IntN(4) > 0,
					Charging:      r.IntN(2) > 0,
					ChargePower:   pick(),
					LpMode:        modes[r.IntN(len(modes))],
					AlwaysCharge:  always[r.IntN(len(always))],
					SessionEnergy: pick(),
				}
				if r.IntN(3) > 0 {
					in.Soc = new(pick())
				}
				if r.IntN(3) > 0 {
					in.SessionSolarPercentage = new(pick())
				}
				if r.IntN(3) > 0 {
					in.Forecast = &Forecast{EnergyNext2h: pick(), EnergyRestOfDay: pick()}
					if r.IntN(2) > 0 {
						in.Forecast.SunStart = now.Add(time.Duration(r.IntN(20)-10) * time.Hour)
						in.Forecast.SunEnd = now.Add(time.Duration(r.IntN(20)-10) * time.Hour)
					}
				}

				d := c.Evaluate(now, in)

				if mode != ModeActive {
					require.Equal(t, ActionNone, d.Action)
				}
				if mode == ModeOff {
					require.Empty(t, d.Events)
				}
				if d.Action == ActionStart || d.Action == ActionResume {
					require.Equal(t, LpModeSmart, in.LpMode)
					require.Equal(t, AlwaysChargeOff, in.AlwaysCharge)
					require.True(t, in.Connected)
					require.NotNil(t, in.Forecast)
				}

				_, err := json.Marshal(c.Status())
				require.NoError(t, err)

				// mostly forward, sometimes early updates or clock jumps
				switch r.IntN(20) {
				case 0:
					now = now.Add(-time.Hour)
				case 1:
					now = now.Add(5 * time.Second)
				case 2:
					now = now.Add(5 * time.Hour)
				default:
					now = now.Add(cycleLen)
				}
			}
		}
	}
}
