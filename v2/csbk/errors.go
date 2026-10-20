package csbk

import "errors"

// ConfigError is the error of a constructor, and of a request or wakeup,
// for a field with a value out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config, option or argument field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "csbk: " + e.Field + " is out of range"
}

// Errors of the machines. Match them with errors.Is.
var (
	// ErrBusy is returned by a request or a wakeup while another one runs.
	ErrBusy = errors.New("csbk: request in progress")
	// ErrNoAddresses is returned by Requester.Request for a standard CSBK
	// without both a target and a source address.
	ErrNoAddresses = errors.New("csbk: CSBK without target and source addresses")
)

var (
	errColorCode  error = &ConfigError{Field: "ColorCode"}
	errSource     error = &ConfigError{Field: "Source"}
	errRetries    error = &ConfigError{Field: "Retries"}
	errAttempts   error = &ConfigError{Field: "Attempts"}
	errAddress    error = &ConfigError{Field: "Address"}
	errTarget     error = &ConfigError{Field: "Target"}
	errSourceAddr error = &ConfigError{Field: "SourceAddress"}
	errPreambles  error = &ConfigError{Field: "Preambles"}
	errAccess     error = &ConfigError{Field: "Access"}
	errMatch      error = &ConfigError{Field: "Match"}
	errOACSU      error = &ConfigError{Field: "OACSU"}
)
