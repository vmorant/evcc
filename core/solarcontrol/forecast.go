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
	panic("not implemented")
}
