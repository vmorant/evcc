package solarcontrol

import (
	"math"
	"time"
)

const (
	// minSampleSpacing drops samples from extra early loop updates that would bias the mean
	minSampleSpacing = 10 * time.Second

	// validCoverage is the share of the window duration the samples must cover
	validCoverage = 0.8

	// maxSampleGap is the longest time between samples that still counts as
	// continuous data, both for window coverage and for the start hold
	maxSampleGap = 2 * time.Minute
)

type sample struct {
	t time.Time
	v float64
}

// window is a time based moving average
type window struct {
	dur     time.Duration
	samples []sample
}

func newWindow(dur time.Duration) *window {
	return &window{dur: dur}
}

// add prunes old samples and appends v. Non-finite values and samples closer
// than minSampleSpacing to the previous one are ignored.
func (w *window) add(now time.Time, v float64) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		w.prune(now)
		return
	}

	if n := len(w.samples); n > 0 {
		last := w.samples[n-1].t
		if now.Before(last) {
			// clock went backwards, the stored samples are meaningless
			w.samples = w.samples[:0]
		} else if now.Sub(last) < minSampleSpacing {
			return
		}
	}

	w.prune(now)
	w.samples = append(w.samples, sample{t: now, v: v})
}

// prune drops samples that left the window
func (w *window) prune(now time.Time) {
	i := 0
	for i < len(w.samples) && now.Sub(w.samples[i].t) >= w.dur {
		i++
	}
	if i > 0 {
		w.samples = append(w.samples[:0], w.samples[i:]...)
	}
}

// mean returns the arithmetic mean of the samples, 0 without samples
func (w *window) mean() float64 {
	if len(w.samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range w.samples {
		sum += s.v
	}
	return sum / float64(len(w.samples))
}

// valid reports whether the samples cover enough of the window and the newest is recent.
// Gaps between samples count up to maxSampleGap.
func (w *window) valid(now time.Time) bool {
	n := len(w.samples)
	if n == 0 || now.Sub(w.samples[n-1].t) > maxSampleGap {
		return false
	}

	var covered time.Duration
	for i := 1; i < n; i++ {
		prev := w.samples[i-1].t
		if now.Sub(prev) >= w.dur {
			continue
		}
		covered += min(w.samples[i].t.Sub(prev), maxSampleGap)
	}

	return covered >= time.Duration(validCoverage*float64(w.dur))
}

// frozenValid is valid at the time of the newest sample, for windows that keep
// their value while no samples are taken
func (w *window) frozenValid() bool {
	if len(w.samples) == 0 {
		return false
	}
	return w.valid(w.samples[len(w.samples)-1].t)
}
