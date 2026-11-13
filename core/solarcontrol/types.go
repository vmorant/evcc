package solarcontrol

import "time"

// Loadpoint mode and always charge values as used by the evcc loadpoint
const (
	LpModeOff   = "off"
	LpModeNow   = "now"
	LpModeSmart = "smart"

	AlwaysChargeOff  = "off"
	AlwaysChargeOn   = "on"
	AlwaysChargeOnce = "once"
)

// Forecast is the solar forecast condensed for one cycle. Energies are scaled
// the same way the UI shows them.
type Forecast struct {
	EnergyNext2h    float64   // Wh between the start of the current hour and two hours later
	EnergyRestOfDay float64   // Wh from now until the end of the local day
	SunStart        time.Time // start of the first slot of today with forecast > 0, zero if none
	SunEnd          time.Time // end of the last slot of today with forecast > 0, zero if none
}

// Inputs are the measurements of one control cycle
type Inputs struct {
	PV          float64  // W, site PV power
	Home        float64  // W, home consumption without loadpoints
	Soc         *float64 // %, vehicle soc, nil if unknown
	Connected   bool
	Charging    bool
	ChargePower float64 // W

	LpMode       string // LpMode* constant
	AlwaysCharge string // AlwaysCharge* constant

	Forecast *Forecast // nil if no solar forecast is available

	// session values, only used for event details
	SessionEnergy          float64  // Wh charged in the current session
	SessionSolarPercentage *float64 // % solar share of the current session
}

// Action is what the caller must apply to the loadpoint
type Action string

const (
	ActionNone       Action = ""
	ActionStart      Action = "start"      // set always charge on
	ActionResume     Action = "resume"     // set always charge on, triggered by connecting
	ActionStop       Action = "stop"       // set always charge off
	ActionDisconnect Action = "disconnect" // reset mode now and/or always charge on, see Decision
)

// Decision is the result of one evaluation. At most one action per cycle.
// Only returned with an action in ModeActive. The caller applies the action
// and then reports the outcome with Controller.Applied, which counts it and
// records its events.
type Decision struct {
	Action Action

	// ActionDisconnect only
	ResetMode         bool // set loadpoint mode now -> smart
	ResetAlwaysCharge bool // set always charge on -> off

	// Events for logging and notifications. With an action this is the pending
	// event, recorded in the history only by a successful Applied.
	Events []Event
}

// EventType classifies history events
type EventType string

const (
	EventStart      EventType = "start"
	EventStop       EventType = "stop"
	EventResume     EventType = "resume"
	EventDisconnect EventType = "disconnect"
	EventShadow     EventType = "shadow"   // controller would have acted (shadow mode)
	EventExternal   EventType = "external" // always charge changed by someone else (shadow mode)
)

// StopRule identifies which rule triggered a stop
type StopRule string

const (
	StopSunLow    StopRule = "sunLow"    // smart: pv average below stop threshold
	StopPower     StopRule = "power"     // simple A: pv average below stop power
	StopShare     StopRule = "share"     // simple B: solar share below limit while sun declines
	StopEndOfDay  StopRule = "endOfDay"  // simple C: pv average below end of day power while sun declines
	StopManualOff StopRule = "manualOff" // external: always charge switched off by someone else
)

// Event is a history entry. Text is composed by the UI from type, action and values.
type Event struct {
	Time     time.Time `json:"time"`
	Type     EventType `json:"type"`
	Strategy Strategy  `json:"strategy"`
	Action   Action    `json:"action,omitempty"` // shadow/external: the action that (would have) happened
	Rule     StopRule  `json:"rule,omitempty"`   // stop events: rule that triggered

	// optional details
	Surplus         *float64 `json:"surplus,omitempty"`         // W, smart: average surplus at start
	PV              *float64 `json:"pv,omitempty"`              // W, average pv at stop, simple: also at start
	Threshold       *float64 `json:"threshold,omitempty"`       // W, threshold that was crossed
	Energy          *float64 `json:"energy,omitempty"`          // kWh charged in session
	SolarPercentage *float64 `json:"solarPercentage,omitempty"` // % solar share of session
	ResetMode       bool     `json:"resetMode,omitempty"`       // disconnect: mode now -> smart
	ResetAlways     bool     `json:"resetAlways,omitempty"`     // disconnect: always charge on -> off
}

// ReasonCode explains the current decision, see Reason
type ReasonCode string

const (
	ReasonDisabled        ReasonCode = "disabled"
	ReasonNotConnected    ReasonCode = "notConnected"
	ReasonCharging        ReasonCode = "charging"
	ReasonManualNow       ReasonCode = "manualNow"
	ReasonManualOff       ReasonCode = "manualOff"
	ReasonManualOnce      ReasonCode = "manualOnce"
	ReasonDailyLimit      ReasonCode = "dailyLimit"
	ReasonCooldown        ReasonCode = "cooldown"
	ReasonOutsideHours    ReasonCode = "outsideHours"
	ReasonWarmup          ReasonCode = "warmup"
	ReasonSocUnknown      ReasonCode = "socUnknown"
	ReasonSurplusLow      ReasonCode = "surplusLow"
	ReasonForecastLow     ReasonCode = "forecastLow"
	ReasonForecastMissing ReasonCode = "forecastMissing"
	ReasonReady           ReasonCode = "ready"
)

// Reason is the single explanation published per cycle. Fields are set per code:
//
//	charging:     value = pv average, threshold = stop threshold, remaining = until stop (only while a stop timer runs)
//	dailyLimit:   count, max
//	cooldown:     remaining
//	outsideHours: from, to
//	surplusLow:   value = average surplus (smart) or pv (simple), threshold = start threshold
//	forecastLow:  value = forecast surplus W (smart) or coverage % (simple), threshold
//	ready:        remaining = until start, nil when starting now
type Reason struct {
	Code      ReasonCode `json:"code"`
	Value     *float64   `json:"value,omitempty"`
	Threshold *float64   `json:"threshold,omitempty"`
	Remaining *float64   `json:"remaining,omitempty"` // seconds
	Count     *int       `json:"count,omitempty"`
	Max       *int       `json:"max,omitempty"`
	From      *int       `json:"from,omitempty"`
	To        *int       `json:"to,omitempty"`
}

// Values are the current measurements and derived thresholds. Nil means unknown.
type Values struct {
	Surplus           float64  `json:"surplus"`           // W
	SurplusAvg15      float64  `json:"surplusAvg15"`      // W
	PvAvg15           float64  `json:"pvAvg15"`           // W
	PvAvg30           float64  `json:"pvAvg30"`           // W
	SolarShareAvg15   float64  `json:"solarShareAvg15"`   // %
	StartThreshold    *float64 `json:"startThreshold"`    // W, smart: needs soc; simple: simple start threshold
	StopThreshold     *float64 `json:"stopThreshold"`     // W, smart only
	PvStopThreshold   *float64 `json:"pvStopThreshold"`   // W, smart only
	ForecastSurplus2h *float64 `json:"forecastSurplus2h"` // W
	ForecastCoverage  *float64 `json:"forecastCoverage"`  // %
	Soc               *float64 `json:"soc"`               // %
	SunDeclining      *bool    `json:"sunDeclining"`
}

// Valid reports which moving averages cover enough of their window
type Valid struct {
	SurplusAvg15    bool `json:"surplusAvg15"`
	PvAvg15         bool `json:"pvAvg15"`
	PvAvg30         bool `json:"pvAvg30"`
	SolarShareAvg15 bool `json:"solarShareAvg15"`
}

// Status is the published state of the controller
type Status struct {
	Mode               Mode       `json:"mode"`
	Strategy           Strategy   `json:"strategy"`
	Reason             Reason     `json:"reason"`
	Values             Values     `json:"values"`
	Valid              Valid      `json:"valid"`
	StartsToday        int        `json:"startsToday"`
	MaxStartsPerDay    int        `json:"maxStartsPerDay"`
	CooldownRemaining  float64    `json:"cooldownRemaining"`  // seconds, 0 if none
	StartHoldRemaining *float64   `json:"startHoldRemaining"` // seconds, nil if the start condition does not hold
	StopHoldRemaining  *float64   `json:"stopHoldRemaining"`  // seconds, nil if no stop timer runs
	SunOk              bool       `json:"sunOk"`
	LastStartAt        *time.Time `json:"lastStartAt"`
	LastStopAt         *time.Time `json:"lastStopAt"`
	Events             []Event    `json:"events"` // most recent first
}

// State is the persistent part of the controller
type State struct {
	StartsToday int       `json:"startsToday"`
	StartsDate  string    `json:"startsDate"` // local date YYYY-MM-DD of StartsToday
	LastStartAt time.Time `json:"lastStartAt"`
	LastStopAt  time.Time `json:"lastStopAt"`
	Events      []Event   `json:"events"` // most recent first, at most MaxEvents
}

// MaxEvents is the number of history events kept
const MaxEvents = 30
