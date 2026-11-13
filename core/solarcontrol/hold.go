package solarcontrol

import "time"

// holdTimer tracks how long a condition has been true without interruption
type holdTimer struct {
	active bool
	since  time.Time
}

// update starts the timer when cond becomes true and resets it when false
func (h *holdTimer) update(now time.Time, cond bool) {
	switch {
	case !cond:
		h.active = false
	case !h.active:
		h.active = true
		h.since = now
	}
}

// updateGated resets the timer while gate is false and otherwise updates it
// with cond, leaving it untouched while cond is unknown
func (h *holdTimer) updateGated(now time.Time, gate, known, cond bool) {
	switch {
	case !gate:
		h.update(now, false)
	case known:
		h.update(now, cond)
	}
}

// satisfied reports whether the condition held for at least hold
func (h *holdTimer) satisfied(now time.Time, hold time.Duration) bool {
	return h.active && now.Sub(h.since) >= hold
}

// remaining returns the seconds left until satisfied, nil if not running
func (h *holdTimer) remaining(now time.Time, hold time.Duration) *float64 {
	if !h.active {
		return nil
	}
	return new(max(0, (hold - now.Sub(h.since)).Seconds()))
}

func minutes(m int) time.Duration {
	return time.Duration(m) * time.Minute
}
