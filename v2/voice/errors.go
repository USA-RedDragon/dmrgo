package voice

import "errors"

// ConfigError is the error of NewReceiver, NewTransmitter,
// NewInterrupter, Transmitter.Start and Interrupter.Interrupt for a config
// field, call field or argument with a value out of range. Match it with
// errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "voice: " + e.Field + " is out of range"
}

// Errors of a Transmitter. Match them with errors.Is.
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
)

var (
	errColorCode   error = &ConfigError{Field: "ColorCode"}
	errSource      error = &ConfigError{Field: "Source"}
	errHeaders     error = &ConfigError{Field: "Headers"}
	errTerminators error = &ConfigError{Field: "Terminators"}
	errQueue       error = &ConfigError{Field: "Queue"}
	errAccess      error = &ConfigError{Field: "Access"}
	errRetries     error = &ConfigError{Field: "Retries"}
	errRepeats     error = &ConfigError{Field: "Repeats"}
	errCommand     error = &ConfigError{Field: "Command"}
	errChanAuth    error = &ConfigError{Field: "ChanAuth"}
	errCall        error = &ConfigError{Field: "Call"}
	errTalkerAlias error = &ConfigError{Field: "TalkerAlias"}
	errGPS         error = &ConfigError{Field: "GPS"}
)
