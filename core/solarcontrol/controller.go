package solarcontrol

import (
	"sync"
	"time"
)

// Controller evaluates the solar charging rules once per control cycle.
// All methods are safe for concurrent use.
type Controller struct {
	mu  sync.Mutex
	loc *time.Location
	cfg Config
	st  State
	rev uint64
}

// New creates a controller from persisted config and state. loc is the local
// time zone for start hours and the daily counter, nil means time.Local.
func New(cfg Config, st State, loc *time.Location) *Controller {
	if loc == nil {
		loc = time.Local
	}
	return &Controller{cfg: cfg, st: st, loc: loc}
}

// Evaluate runs one cycle and returns what the caller must apply.
// Returns an action only in ModeActive.
func (c *Controller) Evaluate(now time.Time, in Inputs) Decision {
	panic("not implemented")
}

// Status returns the state published after the last evaluation
func (c *Controller) Status() Status {
	panic("not implemented")
}

// Config returns the current config
func (c *Controller) Config() Config {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cfg
}

// SetConfig validates and applies a new config
func (c *Controller) SetConfig(cfg Config) error {
	panic("not implemented")
}

// State returns a copy of the persistent state
func (c *Controller) State() State {
	panic("not implemented")
}

// Revision increases whenever config or persistent state change, so callers
// know when to persist.
func (c *Controller) Revision() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rev
}

// Reset clears the daily start counter and the restart cooldown
func (c *Controller) Reset(now time.Time) {
	panic("not implemented")
}
