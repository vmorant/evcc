package solarcontrol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hourlyRates returns one slot per local hour of the day with the given power
func hourlyRates(day time.Time, power func(h int) float64) []Rate {
	var res []Rate
	for h := range 24 {
		start := time.Date(day.Year(), day.Month(), day.Day(), h, 0, 0, 0, madrid)
		res = append(res, Rate{Start: start, End: start.Add(time.Hour), Value: power(h)})
	}
	return res
}

func TestNewForecast(t *testing.T) {
	power := func(h int) float64 {
		switch {
		case h >= 9 && h < 17:
			return 2000
		case h >= 7 && h < 20:
			return 500
		default:
			return 0
		}
	}

	day := at(0, 0)
	rates := hourlyRates(day, power)
	// neighbour days must not move sun start and end
	rates = append(hourlyRates(day.AddDate(0, 0, -1), func(int) float64 { return 100 }), rates...)
	rates = append(rates, hourlyRates(day.AddDate(0, 0, 1), func(int) float64 { return 100 })...)

	f := NewForecast(at(10, 20), madrid, rates, nil, 1.5)
	require.NotNil(t, f)

	// 10:00 to 12:00 at 2000 W
	assert.InDelta(t, 4000*1.5, f.EnergyNext2h, 1e-9)
	// 10:20 to 17:00 at 2000 W, 17:00 to 20:00 at 500 W, pro rata for the current slot
	assert.InDelta(t, (2000*(6+40.0/60)+500*3)*1.5, f.EnergyRestOfDay, 1e-9)
	assert.Equal(t, at(7, 0), f.SunStart)
	assert.Equal(t, at(20, 0), f.SunEnd)
}

func TestNewForecastNoSun(t *testing.T) {
	rates := hourlyRates(at(0, 0), func(int) float64 { return 0 })
	f := NewForecast(at(10, 20), madrid, rates, nil, 1)
	require.NotNil(t, f)
	assert.Zero(t, f.EnergyNext2h)
	assert.True(t, f.SunStart.IsZero())
	assert.True(t, f.SunEnd.IsZero())
}

func TestNewForecastNoRates(t *testing.T) {
	assert.Nil(t, NewForecast(at(10, 0), madrid, nil, nil, 1))
}

func TestNewForecastEnergyFunc(t *testing.T) {
	// DST change, the local day has 25 hours
	now := time.Date(2026, 10, 25, 12, 30, 15, 0, madrid)
	rates := []Rate{{Start: now, End: now.Add(time.Hour), Value: 1000}}

	type call struct{ from, to time.Time }
	var calls []call
	energy := func(from, to time.Time) float64 {
		calls = append(calls, call{from, to})
		return 100
	}

	f := NewForecast(now, madrid, rates, energy, 2)
	require.NotNil(t, f)
	assert.Equal(t, 200.0, f.EnergyNext2h)
	assert.Equal(t, 200.0, f.EnergyRestOfDay)

	hour := time.Date(2026, 10, 25, 12, 0, 0, 0, madrid)
	require.Len(t, calls, 2)
	assert.Equal(t, call{hour, hour.Add(2 * time.Hour)}, calls[0])
	assert.Equal(t, call{now, time.Date(2026, 10, 26, 0, 0, 0, 0, madrid)}, calls[1])
}

func TestForecastDerivedValues(t *testing.T) {
	s := newSim(t, at(16, 0), sunnyInputs(), nil)
	s.step()

	v := s.c.Status().Values
	// 6000 Wh over two hours minus 300 W home
	assert.InDelta(t, 2700, *v.ForecastSurplus2h, 1e-9)
	// 15000 Wh over the 2 h until sun end at 1400 W
	assert.InDelta(t, 15000.0/2/1400*100, *v.ForecastCoverage, 1e-9)
	assert.True(t, *v.SunDeclining)

	// after sun end the coverage uses half an hour
	s = newSim(t, at(19, 0), sunnyInputs(), nil)
	s.step()
	assert.InDelta(t, 15000/0.5/1400*100, *s.c.Status().Values.ForecastCoverage, 1e-9)

	s = newSim(t, at(12, 0), sunnyInputs(), nil)
	s.in.Forecast = nil
	s.step()
	v = s.c.Status().Values
	assert.Nil(t, v.ForecastSurplus2h)
	assert.Nil(t, v.ForecastCoverage)
	assert.Nil(t, v.SunDeclining)
}

func TestSunDecliningEarliestStart(t *testing.T) {
	s := newSim(t, at(9, 0), sunnyInputs(), nil)
	s.step()
	assert.False(t, *s.c.Status().Values.SunDeclining)

	// rate feeds that start at the current slot would move the midpoint to 15:45
	s.now = at(13, 30)
	s.in.Forecast.SunStart = at(13, 30)
	s.step()
	assert.True(t, *s.c.Status().Values.SunDeclining)

	// a new day forgets the cached start
	s.now = time.Date(2026, 6, 16, 13, 30, 0, 0, madrid)
	s.in.Forecast.SunStart = s.now
	s.in.Forecast.SunEnd = time.Date(2026, 6, 16, 18, 0, 0, 0, madrid)
	s.step()
	assert.False(t, *s.c.Status().Values.SunDeclining)
}
