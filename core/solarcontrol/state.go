package solarcontrol

import "time"

const dateLayout = "2006-01-02"

// rollover resets the daily start counter when the local date changed
func (c *Controller) rollover(now time.Time) {
	if today := now.In(c.loc).Format(dateLayout); c.st.StartsDate != today {
		c.st.StartsDate = today
		c.st.StartsToday = 0
		c.rev++
	}
}

// countStart records a start for the daily counter
func (c *Controller) countStart(now time.Time) {
	c.st.StartsToday++
	c.st.LastStartAt = now
	c.rev++
}

// countStop records a solar stop, which starts the restart cooldown
func (c *Controller) countStop(now time.Time) {
	c.st.LastStopAt = now
	c.rev++
}

// addEvent prepends ev to the history
func (c *Controller) addEvent(ev Event, d *Decision) {
	c.st.Events = append([]Event{ev}, c.st.Events...)
	if len(c.st.Events) > MaxEvents {
		c.st.Events = c.st.Events[:MaxEvents]
	}
	d.Events = append(d.Events, cloneEvent(ev))
	c.rev++
}

func (c *Controller) cooldownRemaining(now time.Time) time.Duration {
	if c.st.LastStopAt.IsZero() {
		return 0
	}
	return max(0, minutes(c.cfg.RestartCooldownMin)-now.Sub(c.st.LastStopAt))
}

func cloneState(st State) State {
	st.Events = cloneEvents(st.Events)
	return st
}

// cloneEvents deep copies events, never returning nil
func cloneEvents(events []Event) []Event {
	res := make([]Event, 0, len(events))
	for _, ev := range events {
		res = append(res, cloneEvent(ev))
	}
	return res
}

func cloneEvent(ev Event) Event {
	ev.Surplus = clonePtr(ev.Surplus)
	ev.PV = clonePtr(ev.PV)
	ev.Threshold = clonePtr(ev.Threshold)
	ev.Energy = clonePtr(ev.Energy)
	ev.SolarPercentage = clonePtr(ev.SolarPercentage)
	return ev
}

func clonePtr[T any](p *T) *T {
	if p == nil {
		return nil
	}
	return new(*p)
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return new(t)
}
