package solarcontrol

import "time"

// Rate is a forecast slot with its average power in W
type Rate struct {
	Start, End time.Time
	Value      float64
}

// EnergyFunc integrates forecast power between from and to, returning Wh
type EnergyFunc func(from, to time.Time) float64

// NewForecast condenses solar rates for one cycle. energy integrates the
// forecast (nil uses step integration of rates), scale multiplies the
// energies. Returns nil without rates.
func NewForecast(now time.Time, loc *time.Location, rates []Rate, energy EnergyFunc, scale float64) *Forecast {
	if len(rates) == 0 {
		return nil
	}
	if loc == nil {
		loc = time.Local
	}
	if energy == nil {
		energy = stepEnergy(rates)
	}

	local := now.In(loc)
	// subtracting the wall clock minutes stays correct across DST changes and half hour zones
	hourStart := now.Add(-time.Duration(local.Minute())*time.Minute -
		time.Duration(local.Second())*time.Second -
		time.Duration(local.Nanosecond()))
	dayStart, dayEnd := localDay(now, loc)

	f := &Forecast{
		EnergyNext2h:    energy(hourStart, hourStart.Add(2*time.Hour)) * scale,
		EnergyRestOfDay: energy(now, dayEnd) * scale,
	}

	for _, r := range rates {
		if r.Value <= 0 || !r.End.After(dayStart) || !r.Start.Before(dayEnd) {
			continue
		}
		start, end := r.Start, r.End
		if start.Before(dayStart) {
			start = dayStart
		}
		if end.After(dayEnd) {
			end = dayEnd
		}
		if f.SunStart.IsZero() || start.Before(f.SunStart) {
			f.SunStart = start
		}
		if end.After(f.SunEnd) {
			f.SunEnd = end
		}
	}

	return f
}

// stepEnergy integrates rates as constant power over each slot
func stepEnergy(rates []Rate) EnergyFunc {
	return func(from, to time.Time) float64 {
		var wh float64
		for _, r := range rates {
			start, end := r.Start, r.End
			if start.Before(from) {
				start = from
			}
			if end.After(to) {
				end = to
			}
			if end.After(start) {
				wh += r.Value * end.Sub(start).Hours()
			}
		}
		return wh
	}
}

// localDay returns the start of the local day of t and the start of the next one
func localDay(t time.Time, loc *time.Location) (time.Time, time.Time) {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc), time.Date(y, m, d+1, 0, 0, 0, 0, loc)
}
