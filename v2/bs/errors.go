package bs

import "errors"

// ConfigError is the error of New for a config field with a value out of
// range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "bs: " + e.Field + " is out of range"
}

// ErrSlot is returned for a slot other than enums.Slot1 or enums.Slot2.
// Match it with errors.Is.
var ErrSlot = errors.New("bs: slot must be 1 or 2")

var (
	errMode        error = &ConfigError{Field: "Mode"}
	errColorCode   error = &ConfigError{Field: "ColorCode"}
	errActivation  error = &ConfigError{Field: "Activation"}
	errPending     error = &ConfigError{Field: "Pending"}
	errHeaders     error = &ConfigError{Field: "Headers"}
	errDataQueue   error = &ConfigError{Field: "DataQueue"}
	errTerminators error = &ConfigError{Field: "Terminators"}
	errChanAuth    error = &ConfigError{Field: "ChannelAuth"}
)
