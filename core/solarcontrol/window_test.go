package solarcontrol

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWindow(t *testing.T) {
	t0 := at(10, 0)
	w := newWindow(10 * time.Minute)

	assert.False(t, w.valid(t0))
	assert.Equal(t, 0.0, w.mean())

	w.add(t0, 100)
	w.add(t0.Add(30*time.Second), 200)
	assert.Equal(t, 150.0, w.mean())
	assert.False(t, w.valid(t0.Add(30*time.Second)))

	// valid once the samples span 80 % of the window
	assert.False(t, w.valid(t0.Add(7*time.Minute+59*time.Second)))
	assert.True(t, w.valid(t0.Add(8*time.Minute)))

	// samples closer than 10 s and non-finite values are ignored
	w.add(t0.Add(35*time.Second), 1e6)
	w.add(t0.Add(time.Minute), math.NaN())
	w.add(t0.Add(time.Minute), math.Inf(1))
	assert.Equal(t, 150.0, w.mean())
	w.add(t0.Add(40*time.Second), 300)
	assert.Equal(t, 200.0, w.mean())

	// samples leave the window after its duration
	w.add(t0.Add(10*time.Minute), 400)
	assert.Equal(t, 300.0, w.mean())
	assert.True(t, w.valid(t0.Add(10*time.Minute)))

	w.prune(t0.Add(20 * time.Minute))
	assert.Empty(t, w.samples)
	assert.False(t, w.valid(t0.Add(20*time.Minute)))
}

func TestWindowClockBackwards(t *testing.T) {
	t0 := at(10, 0)
	w := newWindow(10 * time.Minute)
	for i := range 20 {
		w.add(t0.Add(time.Duration(i)*cycleLen), 100)
	}
	require.True(t, w.valid(t0.Add(10*time.Minute)))

	w.add(t0.Add(-time.Hour), 500)
	assert.Equal(t, 500.0, w.mean())
	assert.False(t, w.valid(t0.Add(-time.Hour)))
}

func TestSolarShareFreeze(t *testing.T) {
	in := chargingInputs()
	in.Home = 2300 // surplus 700 W at 1400 W charge power
	s := newSim(t, at(10, 0), in, nil)
	s.run(15 * time.Minute)

	st := s.c.Status()
	require.True(t, st.Valid.SolarShareAvg15)
	assert.InDelta(t, 50, st.Values.SolarShareAvg15, 1e-9)

	// below 200 W charge power no samples are taken and the window keeps its value
	s.in.ChargePower = 150
	s.in.Home = 300
	s.run(time.Hour)
	st = s.c.Status()
	assert.True(t, st.Valid.SolarShareAvg15)
	assert.InDelta(t, 50, st.Values.SolarShareAvg15, 1e-9)

	// charging again replaces the stale samples
	s.in.ChargePower = 1400
	s.step()
	st = s.c.Status()
	assert.False(t, st.Valid.SolarShareAvg15)
	assert.InDelta(t, 100, st.Values.SolarShareAvg15, 1e-9)
}

func TestEarlyUpdatesDoNotBias(t *testing.T) {
	s := newSim(t, at(10, 0), sunnyInputs(), nil)
	s.run(20 * time.Minute)

	// a burst of extra updates right after the last cycle is dropped
	last := s.now.Add(-cycleLen)
	s.in.PV = 0
	for i := range 5 {
		s.c.Evaluate(last.Add(time.Duration(i+1)*time.Second), s.in)
	}
	assert.InDelta(t, 3000, s.c.Status().Values.PvAvg15, 1e-9)
}
