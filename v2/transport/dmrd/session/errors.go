package session

// ConfigError is the error of NewPeer, NewMaster and NewBridge for a
// config field with a value out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "session: " + e.Field + " is out of range"
}

var (
	errDialect error = &ConfigError{Field: "Dialect"}
	errConfig  error = &ConfigError{Field: nameConfig}
	errOptions error = &ConfigError{Field: nameOptions}
	errTimer   error = &ConfigError{Field: "timer"}
)
