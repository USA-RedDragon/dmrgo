// Package dmrgo is the fixture root package of the docgen tests.
package dmrgo

// ConfigError is the error of a constructor for a field with a value out
// of range.
type ConfigError struct{ Field string }

func (e *ConfigError) Error() string { return "bad " + e.Field }
