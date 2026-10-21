package session

// ConfigError is the error of NewPeer and NewMaster for a config field
// with a value out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "session: " + e.Field + " is out of range"
}

var (
	errID       error = &ConfigError{Field: "config"}
	errTimer    error = &ConfigError{Field: "timer"}
	errMaxPeers error = &ConfigError{Field: "MaxPeers"}
)
