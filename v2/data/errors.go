package data

// ConfigError is the error of NewReceiver for a config field with a value
// out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "data: " + e.Field + " is out of range"
}

var (
	errColorCode error = &ConfigError{Field: "ColorCode"}
	errSource    error = &ConfigError{Field: "Source"}
	errMaxBlocks error = &ConfigError{Field: "MaxBlocks"}
	errPeers     error = &ConfigError{Field: "Peers"}
)
