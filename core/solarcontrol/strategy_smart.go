package solarcontrol

func (c *Controller) proposeSmart(cy *cycle) proposal {
	now := cy.now

	if cy.in.AlwaysCharge == AlwaysChargeOn {
		if !c.stopHold.satisfied(now, minutes(c.cfg.StopHoldMin)) {
			return proposal{}
		}
		return proposal{
			action: ActionStop,
			event:  c.stopEvent(cy, StopSunLow, clonePtr(cy.pvStopThreshold)),
		}
	}

	if c.smartReason(cy).Code != ReasonReady {
		return proposal{}
	}

	// connecting resumes on the current surplus without waiting for the hold
	if cy.connectEdge {
		return proposal{action: ActionResume, event: c.startEvent(cy, EventResume)}
	}

	if c.startHold.satisfied(now, minutes(c.cfg.StartHoldMin)) {
		return proposal{action: ActionStart, event: c.startEvent(cy, EventStart)}
	}

	return proposal{}
}

// smartReason evaluates the start conditions in reason order. ReasonReady means
// only the start hold is pending.
func (c *Controller) smartReason(cy *cycle) Reason {
	now := cy.now

	charging := Reason{
		Code:      ReasonCharging,
		Threshold: cy.pvStopThreshold,
		Remaining: c.stopHold.remaining(now, minutes(c.cfg.StopHoldMin)),
	}
	if c.pv15.valid(now) {
		charging.Value = new(c.pv15.mean())
	}

	if r, ok := c.blockedByCommon(cy, charging); ok {
		return r
	}

	if c.st.StartsToday >= c.cfg.MaxStartsPerDay {
		return Reason{Code: ReasonDailyLimit, Count: new(c.st.StartsToday), Max: new(c.cfg.MaxStartsPerDay)}
	}

	if r, ok := c.cooldownReason(now); ok {
		return r
	}

	if h := now.In(c.loc).Hour(); h < c.cfg.StartHourFrom || h >= c.cfg.StartHourTo {
		return Reason{Code: ReasonOutsideHours, From: new(c.cfg.StartHourFrom), To: new(c.cfg.StartHourTo)}
	}

	if !c.surplus15.valid(now) || !c.pv15.valid(now) {
		return Reason{Code: ReasonWarmup}
	}

	if cy.startThreshold == nil {
		return Reason{Code: ReasonSocUnknown}
	}
	threshold := *cy.startThreshold

	if avg := c.surplus15.mean(); avg <= threshold {
		return Reason{Code: ReasonSurplusLow, Value: new(avg), Threshold: new(threshold)}
	}

	if cy.forecastSurplus2h == nil {
		return Reason{Code: ReasonForecastMissing}
	}
	if fs := *cy.forecastSurplus2h; fs <= threshold {
		return Reason{Code: ReasonForecastLow, Value: new(fs), Threshold: new(threshold)}
	}

	hold := minutes(c.cfg.StartHoldMin)
	remaining := c.startHold.remaining(now, hold)
	if remaining == nil {
		remaining = new(hold.Seconds())
	}

	return Reason{Code: ReasonReady, Remaining: remaining}
}
