package voice

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// Errors of a Transmitter. Match them with errors.Is.

const pkgName = "voice"

var (
	// ErrBusy is returned by Start, and by Relay for a CallStart, while a
	// transmission runs, and by Interrupter.Interrupt while a request runs.
	ErrBusy = errors.New("voice: transmission in progress")
	// ErrQueueFull is returned when the queue of vocoder slots is full.
	ErrQueueFull = errors.New("voice: queue full")
	// ErrNotActive is returned for voice when no call takes it: before
	// Start or after Stop.
	ErrNotActive = errors.New("voice: no call takes voice")
	// ErrTimedOut is returned for voice after the transmit timeout T_TO,
	// until Stop (ETSI TS 102 361-2 clause 6.1).
	ErrTimedOut = errors.New("voice: transmit timeout")

	errNilEvent = errors.New("voice: nil Event")
)

var (
	errColorCode   error = &dmrgo.ConfigError{Type: pkgName, Field: "ColorCode"}
	errSource      error = &dmrgo.ConfigError{Type: pkgName, Field: "Source"}
	errHeaders     error = &dmrgo.ConfigError{Type: pkgName, Field: "Headers"}
	errTerminators error = &dmrgo.ConfigError{Type: pkgName, Field: "Terminators"}
	errQueue       error = &dmrgo.ConfigError{Type: pkgName, Field: "Queue"}
	errAccess      error = &dmrgo.ConfigError{Type: pkgName, Field: "Access"}
	errRetries     error = &dmrgo.ConfigError{Type: pkgName, Field: "Retries"}
	errRepeats     error = &dmrgo.ConfigError{Type: pkgName, Field: "Repeats"}
	errCommand     error = &dmrgo.ConfigError{Type: pkgName, Field: "Command"}
	errChanAuth    error = &dmrgo.ConfigError{Type: pkgName, Field: "ChanAuth"}
	errCall        error = &dmrgo.ConfigError{Type: pkgName, Field: "Call"}
	errTalkerAlias error = &dmrgo.ConfigError{Type: pkgName, Field: "TalkerAlias"}
	errGPS         error = &dmrgo.ConfigError{Type: pkgName, Field: "GPS"}
)
