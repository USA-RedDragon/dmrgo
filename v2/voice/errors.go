package voice

// ConfigError is the error of NewReceiver for a config field with a value
// out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

func (e *ConfigError) Error() string {
	return "voice: " + e.Field + " is out of range"
}

// errColorCode is the error of NewReceiver for a color code above 15.
var errColorCode error = &ConfigError{Field: "ColorCode"}
