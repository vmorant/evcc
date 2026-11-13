package solarcontrol

import "time"

// fixed by the legacy trend automations, not configurable
const (
	simpleStopPowerHold    = 10 * time.Minute
	simpleStopShareHold    = 10 * time.Minute
	simpleStopEndOfDayHold = 15 * time.Minute
)

func (c *Controller) updateSimpleTimers(cy *cycle) {
	now, in := cy.now, cy.in
	on := in.AlwaysCharge == AlwaysChargeOn

	pv30Valid := c.pv30.valid(now)
	pv30 := c.pv30.mean()

	c.stopPower.updateGated(now, on && in.Charging, pv30Valid, pv30 < c.cfg.SimpleStopPower)
	c.stopShare.updateGated(now, on && in.Charging, c.share15.valid(now), c.share15.mean() < c.cfg.SimpleShareStop)
	c.stopEndOfDay.updateGated(now, on, pv30Valid, pv30 < c.cfg.SimpleEndOfDayPower)
}

func (c *Controller) proposeSimple(cy *cycle) proposal {
	now := cy.now

	if cy.in.AlwaysCharge == AlwaysChargeOn {
		switch {
		case c.stopPower.satisfied(now, simpleStopPowerHold):
			return proposal{action: ActionStop, event: c.stopEvent(cy, StopPower, new(c.cfg.SimpleStopPower))}
		case c.stopShare.satisfied(now, simpleStopShareHold) && c.sunDeclining:
			// threshold is a share in %, not a power, so it is left out of the event
			return proposal{action: ActionStop, event: c.stopEvent(cy, StopShare, nil)}
		case c.stopEndOfDay.satisfied(now, simpleStopEndOfDayHold) && c.sunDeclining:
			return proposal{action: ActionStop, event: c.stopEvent(cy, StopEndOfDay, new(c.cfg.SimpleEndOfDayPower))}
		}
		return proposal{}
	}

	if c.simpleReason(cy).Code != ReasonReady {
		return proposal{}
	}

	return proposal{action: ActionStart, event: c.startEvent(cy, EventStart)}
}

// simpleReason evaluates the start conditions in reason order, without daily
// limit and start hours
func (c *Controller) simpleReason(cy *cycle) Reason {
	now := cy.now

	charging := Reason{
		Code:      ReasonCharging,
		Threshold: new(c.cfg.SimpleStopPower),
		Remaining: c.simpleStopRemaining(now),
	}
	if c.pv30.valid(now) {
		charging.Value = new(c.pv30.mean())
	}

	if r, ok := c.blockedByCommon(cy, charging); ok {
		return r
	}

	if r, ok := c.cooldownReason(now); ok {
		return r
	}

	if !c.pv30.valid(now) {
		return Reason{Code: ReasonWarmup}
	}

	if pv := c.pv30.mean(); pv <= cy.simpleStartThreshold {
		return Reason{Code: ReasonSurplusLow, Value: new(pv), Threshold: new(cy.simpleStartThreshold)}
	}

	if cy.forecastCoverage == nil {
		return Reason{Code: ReasonForecastMissing}
	}
	if cov := *cy.forecastCoverage; cov <= c.cfg.SimpleForecastShare {
		return Reason{Code: ReasonForecastLow, Value: new(cov), Threshold: new(c.cfg.SimpleForecastShare)}
	}

	return Reason{Code: ReasonReady, Remaining: new(0.0)}
}

// sunFirm is the simple start condition on the pv average
func (c *Controller) sunFirm(cy *cycle) bool {
	return c.pv30.valid(cy.now) && c.pv30.mean() > cy.simpleStartThreshold
}

// simpleStopRemaining returns the seconds until the first running stop timer fires, nil if none runs
func (c *Controller) simpleStopRemaining(now time.Time) *float64 {
	var res *float64
	for _, t := range []struct {
		timer *holdTimer
		hold  time.Duration
	}{
		{&c.stopPower, simpleStopPowerHold},
		{&c.stopShare, simpleStopShareHold},
		{&c.stopEndOfDay, simpleStopEndOfDayHold},
	} {
		if r := t.timer.remaining(now, t.hold); r != nil && (res == nil || *r < *res) {
			res = r
		}
	}
	return res
}
