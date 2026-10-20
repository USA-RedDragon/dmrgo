package data

import "errors"

// ConfigError is the error of NewReceiver, NewSender and Sender.Send for a
// config field or message field with a value out of range. Match it with
// errors.As.
type ConfigError struct {
	// Field is the name of the field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "data: " + e.Field + " is out of range"
}

// ErrBusy is returned by Sender.Send while a message is in progress. Match
// it with errors.Is.
var ErrBusy = errors.New("data: message in progress")

var (
	errColorCode    error = &ConfigError{Field: "ColorCode"}
	errSource       error = &ConfigError{Field: "Source"}
	errMaxBlocks    error = &ConfigError{Field: "MaxBlocks"}
	errPeers        error = &ConfigError{Field: "Peers"}
	errAccess       error = &ConfigError{Field: "Access"}
	errPreambles    error = &ConfigError{Field: "Preambles"}
	errRetries      error = &ConfigError{Field: "Retries"}
	errHeader       error = &ConfigError{Field: "Header"}
	errMessageGroup error = &ConfigError{Field: "Header.Group"}
)
