// Package solarcontrol decides when to start and stop solar charging by
// toggling always charge on a smart mode loadpoint. It is pure logic: inputs
// are passed in per cycle, actions are returned, nothing is written directly.
package solarcontrol

import (
	"fmt"
)

// Mode selects whether the controller acts on the loadpoint
type Mode string

const (
	ModeOff    Mode = "off"    // no evaluation side effects, no writes
	ModeShadow Mode = "shadow" // evaluate and record what would happen, no writes
	ModeActive Mode = "active" // evaluate and act
)

// Strategy selects the start/stop rule set
type Strategy string

const (
	StrategySmart  Strategy = "smart"
	StrategySimple Strategy = "simple"
)

// Config holds all user adjustable parameters
type Config struct {
	Mode     Mode     `json:"mode"`
	Strategy Strategy `json:"strategy"`

	// both strategies
	MinChargePower     float64 `json:"minChargePower"`     // W
	RestartCooldownMin int     `json:"restartCooldownMin"` // minutes after a solar stop

	// smart
	SocLow            float64 `json:"socLow"`            // %
	SocHigh           float64 `json:"socHigh"`           // %
	StartShareLowSoc  float64 `json:"startShareLowSoc"`  // % of minChargePower at or below socLow
	StartShareHighSoc float64 `json:"startShareHighSoc"` // % of minChargePower at or above socHigh
	StopHysteresis    float64 `json:"stopHysteresis"`    // % of start threshold
	BaseHomeLoad      float64 `json:"baseHomeLoad"`      // W
	MaxStartsPerDay   int     `json:"maxStartsPerDay"`
	StartHourFrom     int     `json:"startHourFrom"` // local hour, inclusive
	StartHourTo       int     `json:"startHourTo"`   // local hour, exclusive
	StartHoldMin      int     `json:"startHoldMin"`  // minutes surplus must stay above threshold
	StopHoldMin       int     `json:"stopHoldMin"`   // minutes sun must stay low

	// simple
	SimpleStartShare    float64 `json:"simpleStartShare"`    // % of minChargePower
	SimpleForecastShare float64 `json:"simpleForecastShare"` // %
	SimpleStopPower     float64 `json:"simpleStopPower"`     // W
	SimpleShareStop     float64 `json:"simpleShareStop"`     // %
	SimpleEndOfDayPower float64 `json:"simpleEndOfDayPower"` // W
}

// DefaultConfig returns the default configuration. The controller starts disabled.
func DefaultConfig() Config {
	return Config{
		Mode:     ModeOff,
		Strategy: StrategySmart,

		MinChargePower:     1400,
		RestartCooldownMin: 60,

		SocLow:            20,
		SocHigh:           75,
		StartShareLowSoc:  50,
		StartShareHighSoc: 60,
		StopHysteresis:    50,
		BaseHomeLoad:      350,
		MaxStartsPerDay:   2,
		StartHourFrom:     8,
		StartHourTo:       17,
		StartHoldMin:      10,
		StopHoldMin:       15,

		SimpleStartShare:    50,
		SimpleForecastShare: 55,
		SimpleStopPower:     800,
		SimpleShareStop:     35,
		SimpleEndOfDayPower: 690,
	}
}

// ValidationError reports the first invalid config field by its json name
type ValidationError struct {
	Field string `json:"field"`
	Msg   string `json:"error"`
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Msg)
}

type floatRange struct {
	field    string
	val      float64
	min, max float64
}

type intRange struct {
	field    string
	val      int
	min, max int
}

// Validate checks ranges and cross-field constraints
func (c Config) Validate() error {
	switch c.Mode {
	case ModeOff, ModeShadow, ModeActive:
	default:
		return &ValidationError{"mode", fmt.Sprintf("invalid value %q", c.Mode)}
	}

	switch c.Strategy {
	case StrategySmart, StrategySimple:
	default:
		return &ValidationError{"strategy", fmt.Sprintf("invalid value %q", c.Strategy)}
	}

	for _, r := range []floatRange{
		{"minChargePower", c.MinChargePower, 500, 11000},
		{"socLow", c.SocLow, 0, 100},
		{"socHigh", c.SocHigh, 0, 100},
		{"startShareLowSoc", c.StartShareLowSoc, 10, 150},
		{"startShareHighSoc", c.StartShareHighSoc, 10, 150},
		{"stopHysteresis", c.StopHysteresis, 10, 100},
		{"baseHomeLoad", c.BaseHomeLoad, 100, 1000},
		{"simpleStartShare", c.SimpleStartShare, 10, 100},
		{"simpleForecastShare", c.SimpleForecastShare, 10, 100},
		{"simpleStopPower", c.SimpleStopPower, 0, 2000},
		{"simpleShareStop", c.SimpleShareStop, 0, 100},
		{"simpleEndOfDayPower", c.SimpleEndOfDayPower, 0, 2000},
	} {
		// NaN fails both comparisons, reject it explicitly
		if !(r.val >= r.min && r.val <= r.max) {
			return &ValidationError{r.field, fmt.Sprintf("must be between %g and %g", r.min, r.max)}
		}
	}

	for _, r := range []intRange{
		{"restartCooldownMin", c.RestartCooldownMin, 0, 120},
		{"maxStartsPerDay", c.MaxStartsPerDay, 1, 10},
		{"startHourFrom", c.StartHourFrom, 0, 24},
		{"startHourTo", c.StartHourTo, 0, 24},
		{"startHoldMin", c.StartHoldMin, 0, 60},
		{"stopHoldMin", c.StopHoldMin, 0, 60},
	} {
		if r.val < r.min || r.val > r.max {
			return &ValidationError{r.field, fmt.Sprintf("must be between %d and %d", r.min, r.max)}
		}
	}

	if c.SocLow >= c.SocHigh {
		return &ValidationError{"socHigh", "must be greater than socLow"}
	}

	if c.StartHourFrom >= c.StartHourTo {
		return &ValidationError{"startHourTo", "must be greater than startHourFrom"}
	}

	return nil
}
