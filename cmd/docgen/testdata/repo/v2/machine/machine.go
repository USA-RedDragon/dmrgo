// Package machine is a small state machine for the docgen tests.
//
// # Rates
//
// A Lamp blinks at these rates:
//   - slow, every [DefaultWait]
//   - fast, every period
//
// # Other
//
// Not copied.
package machine

import (
	"errors"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// DefaultWait is the wait of a Lamp that Config does not set.
const DefaultWait = 250 * time.Millisecond

// MaxTries is the largest Config.Tries.
const MaxTries = 3

var (
	// ErrBusy is returned by Start while the Lamp is on.
	ErrBusy = errors.New("machine: busy")
	// ErrOff is returned by Stop while the Lamp is off.
	ErrOff = errors.New("machine: off")
)

// Config configures NewLamp.
type Config struct {
	// Wait is how long the Lamp stays on. 0 means DefaultWait.
	Wait time.Duration
	// Tries is the number of blinks. 0 means 1, and it is at most MaxTries.
	Tries int
	// Name is a label.
	Name string
}

// LampState is the state of a Lamp.
type LampState uint8

// The LampState values.
const (
	// LampOff gives no light.
	LampOff LampState = iota
	// LampOn gives light.
	LampOn
	// LampBroken never gives light again.
	LampBroken
)

// Lamp turns on and off.
type Lamp struct {
	state LampState
	wait  time.Duration
	tries int
	until time.Duration
}

// NewLamp returns a Lamp. It returns a *dmrgo.ConfigError for a Tries
// above MaxTries.
func NewLamp(cfg Config) (*Lamp, error) {
	if cfg.Tries > MaxTries {
		return nil, &dmrgo.ConfigError{Field: "Tries"}
	}
	l := &Lamp{wait: cfg.Wait, tries: cfg.Tries}
	if l.wait == 0 {
		l.wait = DefaultWait
	}
	if cfg.Tries == 0 {
		l.tries = 1
	}
	return l, nil
}

// State returns the state of the Lamp.
func (l *Lamp) State() LampState { return l.state }

// Start turns the Lamp on. It returns ErrBusy while the Lamp is on.
func (l *Lamp) Start(now time.Duration) error {
	if l.state == LampOn {
		return ErrBusy
	}
	if l.state == LampOff {
		l.state, l.until = LampOn, now+l.wait
	}
	return nil
}

// Stop turns the Lamp off. It returns ErrOff while the Lamp is off.
func (l *Lamp) Stop() error {
	if l.state != LampOn {
		return ErrOff
	}
	l.off()
	return nil
}

// Tick turns the Lamp off at the end of its wait.
func (l *Lamp) Tick(now time.Duration) {
	if l.state == LampOn && now >= l.until {
		l.off()
	}
}

// Break breaks the Lamp.
func (l *Lamp) Break() { l.state = LampBroken }

func (l *Lamp) off() { l.state = LampOff }

// Counter counts and has no State method.
type Counter struct{ n int }

// Add adds one.
func (c *Counter) Add() { c.n++ }

// Clear sets the count to 0.
func (c *Counter) Clear() { c.n = 0 }

// Len returns the count.
func (c *Counter) Len() int { return c.n }

const period = 30 * time.Millisecond

var hold = 4 * period

// Side is one side of a Pair.
type Side uint8

// The Side values.
const (
	// SideA is the first side.
	SideA Side = iota + 1
	// SideB is the second side.
	SideB
)

// SideState is the state of one side of a Pair.
type SideState uint8

// The SideState values.
const (
	// SideDown is down.
	SideDown SideState = iota
	// SideUp is up.
	SideUp
)

// Pair has two sides.
type Pair struct{ up [3]bool }

// Raise raises side s.
func (p *Pair) Raise(s Side) {
	if s == SideA || s == SideB {
		p.up[s] = true
	}
}

// SideState returns the state of side s.
func (p *Pair) SideState(s Side) SideState {
	if s != SideA && s != SideB || !p.up[s] {
		return SideDown
	}
	return SideUp
}
