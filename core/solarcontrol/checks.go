package solarcontrol

import "time"

const (
	phaseStart = "start"
	phaseStop  = "stop"
)

// checks lists the conditions of the current phase for the status
func (c *Controller) checks(cy *cycle) (string, []Check) {
	simple := c.cfg.Strategy == StrategySimple

	if cy.in.AlwaysCharge == AlwaysChargeOn {
		if simple {
			return phaseStop, c.simpleStopChecks(cy)
		}
		return phaseStop, c.smartStopChecks(cy)
	}

	if simple {
		return phaseStart, c.simpleStartChecks(cy)
	}
	return phaseStart, c.smartStartChecks(cy)
}

func (c *Controller) smartStartChecks(cy *cycle) []Check {
	now := cy.now
	hold := minutes(c.cfg.StartHoldMin)
	from, to := c.cfg.StartHourFrom, c.cfg.StartHourTo
	h := now.In(c.loc).Hour()

	var avg *float64
	if cy.surplusKnown && c.surplus15.valid(now) {
		avg = new(c.surplus15.mean())
	}
	surplus := above("surplus", avg, clonePtr(cy.startThreshold))

	holdCheck := Check{Key: "hold", Ok: c.startHold.satisfied(now, hold), Known: surplus.Known}
	if !holdCheck.Ok {
		holdCheck.Remaining = c.startHold.remaining(now, hold)
	}

	return append(c.commonStartChecks(cy),
		Check{Key: "hours", Ok: h >= from && h < to, Known: true, Value: new(float64(from)), Threshold: new(float64(to))},
		surplus,
		holdCheck,
		above("forecast", clonePtr(cy.forecastSurplus2h), clonePtr(cy.startThreshold)),
		c.cooldownCheck(now),
		Check{
			Key:       "dailyLimit",
			Ok:        c.st.StartsToday < c.cfg.MaxStartsPerDay,
			Known:     true,
			Value:     new(float64(c.st.StartsToday)),
			Threshold: new(float64(c.cfg.MaxStartsPerDay)),
		},
	)
}

func (c *Controller) simpleStartChecks(cy *cycle) []Check {
	var pv *float64
	if cy.pvKnown && c.pv30.valid(cy.now) {
		pv = new(c.pv30.mean())
	}

	return append(c.commonStartChecks(cy),
		above("sun", pv, new(cy.simpleStartThreshold)),
		above("forecast", clonePtr(cy.forecastCoverage), new(c.cfg.SimpleForecastShare)),
		c.cooldownCheck(cy.now),
	)
}

func (c *Controller) commonStartChecks(cy *cycle) []Check {
	in := cy.in
	return []Check{
		{Key: "connected", Ok: in.Connected, Known: true},
		{Key: "mode", Ok: in.LpMode == LpModeSmart && in.AlwaysCharge == AlwaysChargeOff, Known: true},
	}
}

func (c *Controller) cooldownCheck(now time.Time) Check {
	ck := Check{Key: "cooldown", Ok: true, Known: true}
	if cd := c.cooldownRemaining(now); cd > 0 {
		ck.Ok = false
		ck.Remaining = new(cd.Seconds())
	}
	return ck
}

func (c *Controller) smartStopChecks(cy *cycle) []Check {
	now := cy.now

	var pv *float64
	if c.pv15.valid(now) {
		pv = new(c.pv15.mean())
	}

	return []Check{
		{Key: "sun", Ok: c.sunOk, Known: pv != nil && cy.pvStopThreshold != nil, Value: pv, Threshold: clonePtr(cy.pvStopThreshold)},
		stopRuleCheck(now, "stopHold", nil, nil, &c.stopHold, minutes(c.cfg.StopHoldMin), true),
	}
}

func (c *Controller) simpleStopChecks(cy *cycle) []Check {
	now := cy.now

	var pv30, share *float64
	if c.pv30.valid(now) {
		pv30 = new(c.pv30.mean())
	}
	if c.share15.frozenValid() {
		share = new(c.share15.mean())
	}

	sunDeclining := Check{Key: "sunDeclining", Ok: !c.sunDeclining, Known: cy.sunDeclining != nil}

	power := stopRuleCheck(now, "power", pv30, new(c.cfg.SimpleStopPower), &c.stopPower, simpleStopPowerHold, true)
	power.Known = pv30 != nil

	shareCheck := stopRuleCheck(now, "share", share, new(c.cfg.SimpleShareStop), &c.stopShare, simpleStopShareHold, c.sunDeclining)
	shareCheck.Known = share != nil

	endOfDay := stopRuleCheck(now, "endOfDay", clonePtr(pv30), new(c.cfg.SimpleEndOfDayPower), &c.stopEndOfDay, simpleStopEndOfDayHold, c.sunDeclining)
	endOfDay.Known = pv30 != nil

	return []Check{power, shareCheck, endOfDay, sunDeclining}
}

// stopRuleCheck is not Ok while the rule's timer runs and can fire
func stopRuleCheck(now time.Time, key string, value, threshold *float64, t *holdTimer, hold time.Duration, armed bool) Check {
	ck := Check{Key: key, Ok: true, Known: true, Value: value, Threshold: threshold}
	if r := t.remaining(now, hold); armed && r != nil {
		ck.Ok = false
		ck.Remaining = r
	}
	return ck
}

// above checks value > threshold, unknown without either
func above(key string, value, threshold *float64) Check {
	known := value != nil && threshold != nil
	return Check{
		Key:       key,
		Ok:        known && *value > *threshold,
		Known:     known,
		Value:     value,
		Threshold: threshold,
	}
}

// forecastWindow returns the first contiguous run of forecast slots from now
// until the end of the local day whose power meets the start threshold.
// Smart only considers the start hours. Nil if there is none.
func (c *Controller) forecastWindow(cy *cycle, slots []Rate) (*time.Time, *time.Time) {
	now := cy.now
	from := now
	_, to := localDay(now, c.loc)

	var qualifies func(power float64) bool
	if c.cfg.Strategy == StrategySimple {
		qualifies = func(power float64) bool { return power > cy.simpleStartThreshold }
	} else {
		if cy.startThreshold == nil || !finite(cy.in.Home) {
			return nil, nil
		}
		threshold, home := *cy.startThreshold, cy.in.Home
		qualifies = func(power float64) bool { return power-home > threshold }

		y, m, d := now.In(c.loc).Date()
		from = latest(from, time.Date(y, m, d, c.cfg.StartHourFrom, 0, 0, 0, c.loc))
		to = earliest(to, time.Date(y, m, d, c.cfg.StartHourTo, 0, 0, 0, c.loc))
	}

	var start, end time.Time
	for _, s := range slots {
		lo, hi := latest(s.Start, from), earliest(s.End, to)
		if !hi.After(lo) {
			continue
		}

		running := !start.IsZero()
		if !qualifies(s.Value) || running && lo.After(end) {
			if running {
				break
			}
			continue
		}

		if !running {
			start = lo
		}
		end = hi
	}

	if start.IsZero() {
		return nil, nil
	}
	return new(start), new(end)
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func cloneChecks(checks []Check) []Check {
	res := make([]Check, 0, len(checks))
	for _, ck := range checks {
		ck.Value = clonePtr(ck.Value)
		ck.Threshold = clonePtr(ck.Threshold)
		ck.Remaining = clonePtr(ck.Remaining)
		res = append(res, ck)
	}
	return res
}
