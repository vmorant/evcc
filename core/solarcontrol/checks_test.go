package solarcontrol

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkKeys(checks []Check) []string {
	var res []string
	for _, ck := range checks {
		res = append(res, ck.Key)
	}
	return res
}

func checkByKey(t *testing.T, checks []Check, key string) Check {
	t.Helper()
	for _, ck := range checks {
		if ck.Key == key {
			return ck
		}
	}
	require.Failf(t, "missing check", key)
	return Check{}
}

func TestChecksSmartStart(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), nil)
	s.run(15 * time.Minute) // last cycle 10:14:30, hold since 10:12

	st := s.c.Status()
	assert.Equal(t, "start", st.ChecksPhase)
	assert.Equal(t, []string{"connected", "mode", "hours", "surplus", "hold", "forecast", "cooldown", "dailyLimit"}, checkKeys(st.Checks))

	for _, ck := range st.Checks {
		assert.True(t, ck.Known, ck.Key)
		assert.Equal(t, ck.Key != "hold", ck.Ok, ck.Key)
	}

	hours := checkByKey(t, st.Checks, "hours")
	assert.Equal(t, 8.0, *hours.Value)
	assert.Equal(t, 17.0, *hours.Threshold)

	surplus := checkByKey(t, st.Checks, "surplus")
	assert.InDelta(t, 2700, *surplus.Value, 1e-9)
	assert.InDelta(t, 776.36, *surplus.Threshold, 0.01)

	assert.Equal(t, 450.0, *checkByKey(t, st.Checks, "hold").Remaining)
	assert.InDelta(t, 2700, *checkByKey(t, st.Checks, "forecast").Value, 1e-9)
	assert.Nil(t, checkByKey(t, st.Checks, "cooldown").Remaining)

	limit := checkByKey(t, st.Checks, "dailyLimit")
	assert.Equal(t, 0.0, *limit.Value)
	assert.Equal(t, 2.0, *limit.Threshold)

	// hold satisfied
	s.run(8 * time.Minute)
	hold := checkByKey(t, s.c.Status().Checks, "hold")
	assert.True(t, hold.Ok)
	assert.Nil(t, hold.Remaining)
}

func TestChecksSmartStartBlocked(t *testing.T) {
	in := sunnyInputs()
	in.Connected = false
	in.Soc = nil
	st := State{StartsDate: "2026-06-15", StartsToday: 2, LastStopAt: at(17, 30)}
	s := newSimState(t, at(18, 0), in, nil, st)
	s.run(15 * time.Minute)

	checks := s.c.Status().Checks
	assert.Equal(t, "start", s.c.Status().ChecksPhase)
	for _, key := range []string{"connected", "hours", "dailyLimit", "cooldown"} {
		ck := checkByKey(t, checks, key)
		assert.False(t, ck.Ok, key)
		assert.True(t, ck.Known, key)
	}
	assert.Equal(t, 15*60.0+30, *checkByKey(t, checks, "cooldown").Remaining)

	// without soc there is no threshold
	for _, key := range []string{"surplus", "hold", "forecast"} {
		ck := checkByKey(t, checks, key)
		assert.False(t, ck.Ok, key)
		assert.False(t, ck.Known, key)
	}
	assert.Nil(t, checkByKey(t, checks, "surplus").Threshold)
	assert.InDelta(t, 2700, *checkByKey(t, checks, "surplus").Value, 1e-9)

	// missing pv
	s.in.Soc = new(50.0)
	s.in.PV = math.NaN()
	s.step()
	surplus := checkByKey(t, s.c.Status().Checks, "surplus")
	assert.False(t, surplus.Known)
	assert.Nil(t, surplus.Value)
}

func TestChecksSimpleStart(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), simple)
	s.run(10 * time.Minute)

	st := s.c.Status()
	assert.Equal(t, "start", st.ChecksPhase)
	assert.Equal(t, []string{"connected", "mode", "sun", "forecast", "cooldown"}, checkKeys(st.Checks))

	sun := checkByKey(t, st.Checks, "sun")
	assert.False(t, sun.Known, "warmup")
	assert.False(t, sun.Ok)
	assert.Equal(t, 700.0, *sun.Threshold)

	forecast := checkByKey(t, st.Checks, "forecast")
	assert.True(t, forecast.Ok)
	assert.Equal(t, 55.0, *forecast.Threshold)

	s.run(15 * time.Minute)
	sun = checkByKey(t, s.c.Status().Checks, "sun")
	assert.True(t, sun.Known)
	assert.True(t, sun.Ok)
	assert.InDelta(t, 3000, *sun.Value, 1e-9)
}

func TestChecksSmartStop(t *testing.T) {
	in := chargingInputs()
	in.PV = 500
	s := newSim(t, at(15, 0), in, nil)

	s.run(10 * time.Minute)
	st := s.c.Status()
	assert.Equal(t, "stop", st.ChecksPhase)
	assert.Equal(t, []string{"sun", "stopHold"}, checkKeys(st.Checks))
	sun := checkByKey(t, st.Checks, "sun")
	assert.True(t, sun.Ok, "initial sun ok")
	assert.False(t, sun.Known)
	assert.True(t, checkByKey(t, st.Checks, "stopHold").Ok)

	s.run(5 * time.Minute) // 15:14:30, sun low since 15:12
	st = s.c.Status()
	sun = checkByKey(t, st.Checks, "sun")
	assert.False(t, sun.Ok)
	assert.True(t, sun.Known)
	assert.InDelta(t, 500, *sun.Value, 1e-9)
	assert.InDelta(t, 738.18, *sun.Threshold, 0.01)

	hold := checkByKey(t, st.Checks, "stopHold")
	assert.False(t, hold.Ok)
	assert.Equal(t, 750.0, *hold.Remaining)
}

func TestChecksSimpleStop(t *testing.T) {
	in := chargingInputs()
	in.PV, in.Home = 700, 400 // share 21 %, pv below stop power, above end of day power
	s := newSim(t, at(14, 0), in, simple)
	s.run(15*time.Minute + 30*time.Second) // last cycle 14:15, share timer since 14:12

	st := s.c.Status()
	assert.Equal(t, "stop", st.ChecksPhase)
	assert.Equal(t, []string{"power", "share", "endOfDay", "sunDeclining"}, checkKeys(st.Checks))

	power := checkByKey(t, st.Checks, "power")
	assert.True(t, power.Ok)
	assert.False(t, power.Known, "warmup")
	assert.Nil(t, power.Value)

	share := checkByKey(t, st.Checks, "share")
	assert.False(t, share.Ok)
	assert.True(t, share.Known)
	assert.InDelta(t, 300.0/1400*100, *share.Value, 1e-9)
	assert.Equal(t, 35.0, *share.Threshold)
	assert.Equal(t, 420.0, *share.Remaining)

	s.run(9*time.Minute + 30*time.Second) // last cycle 14:24:30, power timer since 14:24
	st = s.c.Status()
	power = checkByKey(t, st.Checks, "power")
	assert.False(t, power.Ok)
	assert.True(t, power.Known)
	assert.InDelta(t, 700, *power.Value, 1e-9)
	assert.Equal(t, 800.0, *power.Threshold)
	assert.Equal(t, 570.0, *power.Remaining)

	endOfDay := checkByKey(t, st.Checks, "endOfDay")
	assert.True(t, endOfDay.Ok)
	assert.Nil(t, endOfDay.Remaining)
	assert.Equal(t, 690.0, *endOfDay.Threshold)

	declining := checkByKey(t, st.Checks, "sunDeclining")
	assert.False(t, declining.Ok)
	assert.True(t, declining.Known)

	// while the sun rises the share rule cannot fire
	s = newSim(t, at(10, 0), in, simple)
	s.run(25 * time.Minute)
	share = checkByKey(t, s.c.Status().Checks, "share")
	assert.True(t, share.Ok)
	assert.Nil(t, share.Remaining)
	assert.True(t, checkByKey(t, s.c.Status().Checks, "sunDeclining").Ok)

	s.in.Forecast = nil
	s.step()
	assert.False(t, checkByKey(t, s.c.Status().Checks, "sunDeclining").Known)
}

func TestChecksBeforeEvaluate(t *testing.T) {
	c := New(DefaultConfig(), State{}, madrid)
	st := c.Status()
	assert.Empty(t, st.ChecksPhase)
	assert.NotNil(t, st.Checks)

	b, err := json.Marshal(st)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"checks":[]`)
	assert.Contains(t, string(b), `"checksPhase":""`)
}

func TestChecksAreCopies(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), nil)
	s.run(15 * time.Minute)

	st := s.c.Status()
	*checkByKey(t, st.Checks, "surplus").Value = -1
	st.Checks[0].Ok = false

	st = s.c.Status()
	assert.InDelta(t, 2700, *checkByKey(t, st.Checks, "surplus").Value, 1e-9)
	assert.True(t, st.Checks[0].Ok)
}

func TestStatusInstantValues(t *testing.T) {
	s := newSim(t, at(10, 0), chargingInputs(), nil)
	s.step()

	v := s.c.Status().Values
	assert.Equal(t, 3000.0, *v.PV)
	assert.Equal(t, 300.0, *v.Home)
	assert.Equal(t, 1400.0, v.ChargePower)
	assert.Equal(t, 15000.0, *v.ForecastEnergyRestOfDay)
	assert.Equal(t, at(18, 0), *v.SunEnd)
	assert.InDelta(t, 4.2, *v.SessionEnergy, 1e-9)
	assert.Equal(t, 80.0, *v.SessionSolarPercentage)

	s.in.PV, s.in.Home, s.in.ChargePower = math.NaN(), math.Inf(1), math.NaN()
	s.in.SessionSolarPercentage = new(math.NaN())
	s.in.Forecast = nil
	s.step()

	v = s.c.Status().Values
	assert.Nil(t, v.PV)
	assert.Nil(t, v.Home)
	assert.Zero(t, v.ChargePower)
	assert.Nil(t, v.ForecastEnergyRestOfDay)
	assert.Nil(t, v.SunEnd)
	assert.Nil(t, v.SessionSolarPercentage)
	assert.Nil(t, v.ForecastWindowStart)
	assert.Nil(t, v.ForecastWindowEnd)

	_, err := json.Marshal(s.c.Status())
	require.NoError(t, err)
}

func TestForecastWindow(t *testing.T) {
	// slot powers by local hour, others 0
	slots := func(power map[int]float64) []Rate {
		return hourlyRates(at(0, 0), func(h int) float64 { return power[h] })
	}
	sloped := map[int]float64{8: 500, 9: 1000, 10: 1500, 11: 2000, 12: 900, 13: 600, 14: 1200}

	for _, tc := range []struct {
		name       string
		strategy   Strategy
		now        time.Time
		slots      []Rate
		in         func(*Inputs)
		start, end time.Time // zero: no window
	}{
		{name: "simple from current slot", strategy: StrategySimple, now: at(10, 15), slots: slots(sloped), start: at(10, 15), end: at(13, 0)},
		{name: "smart subtracts home", strategy: StrategySmart, now: at(10, 15), slots: slots(sloped), start: at(10, 15), end: at(12, 0)},
		{name: "smart later today", strategy: StrategySmart, now: at(10, 15), slots: slots(map[int]float64{10: 500, 11: 2000, 12: 2000}), start: at(11, 0), end: at(13, 0)},
		{name: "smart clipped to start hours", strategy: StrategySmart, now: at(15, 30), slots: slots(map[int]float64{15: 2000, 16: 2000, 17: 2000, 18: 2000}), start: at(15, 30), end: at(17, 0)},
		{name: "smart before start hours", strategy: StrategySmart, now: at(6, 10), slots: slots(map[int]float64{6: 2000, 7: 2000, 8: 2000, 9: 2000}), start: at(8, 0), end: at(10, 0)},
		{name: "smart outside start hours", strategy: StrategySmart, now: at(16, 30), slots: slots(map[int]float64{16: 500, 17: 2000, 18: 2000})},
		{name: "simple ignores start hours", strategy: StrategySimple, now: at(16, 30), slots: slots(map[int]float64{16: 500, 17: 2000, 18: 2000}), start: at(17, 0), end: at(19, 0)},
		{name: "smart without soc", strategy: StrategySmart, now: at(10, 15), slots: slots(sloped), in: func(in *Inputs) { in.Soc = nil }},
		{name: "smart without home", strategy: StrategySmart, now: at(10, 15), slots: slots(sloped), in: func(in *Inputs) { in.Home = math.NaN() }},
		{name: "none qualifies", strategy: StrategySimple, now: at(10, 15), slots: slots(map[int]float64{10: 500, 11: 600})},
		{name: "sun already gone", strategy: StrategySimple, now: at(15, 15), slots: slots(sloped)},
		{name: "gap ends the run", strategy: StrategySimple, now: at(10, 15), slots: append(slots(map[int]float64{10: 2000})[10:11], slots(map[int]float64{12: 2000})[12:13]...), start: at(10, 15), end: at(11, 0)},
		{name: "ends with the day", strategy: StrategySimple, now: at(22, 30), slots: append(slots(map[int]float64{22: 2000, 23: 2000}), hourlyRates(at(0, 0).AddDate(0, 0, 1), func(int) float64 { return 2000 })...), start: at(22, 30), end: at(0, 0).AddDate(0, 0, 1)},
		{name: "no slots", strategy: StrategySimple, now: at(10, 15)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := sunnyInputs()
			in.Forecast.Slots = tc.slots
			if tc.in != nil {
				tc.in(&in)
			}
			s := newSim(t, tc.now, in, func(c *Config) { c.Strategy = tc.strategy })
			s.step()

			v := s.c.Status().Values
			if tc.start.IsZero() {
				assert.Nil(t, v.ForecastWindowStart)
				assert.Nil(t, v.ForecastWindowEnd)
				return
			}
			require.NotNil(t, v.ForecastWindowStart)
			require.NotNil(t, v.ForecastWindowEnd)
			assert.Equal(t, tc.start, *v.ForecastWindowStart)
			assert.Equal(t, tc.end, *v.ForecastWindowEnd)
		})
	}
}
