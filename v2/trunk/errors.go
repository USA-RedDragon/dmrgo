package trunk

import "errors"

// ConfigError is the error of NewFollower, NewMS and NewTSCC for a config
// field with a value out of range. Match it with errors.As.
type ConfigError struct {
	// Field is the name of the config field.
	Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	return "trunk: " + e.Field + " is out of range"
}

// Errors of the tables and machines. Match them with errors.Is.
var (
	// ErrChannelNumber is returned for a channel number that names no
	// channel: CHNULL, ChannelAbsolute or over 12 bits.
	ErrChannelNumber = errors.New("trunk: channel number out of range")
	// ErrPlanFull is returned when a ChannelPlan or HuntList has no room.
	ErrPlanFull = errors.New("trunk: table full")
	// ErrBusy is returned by MS.Call while a registration or call request
	// runs.
	ErrBusy = errors.New("trunk: request in progress")
	// ErrNotOnControl is returned by MS.Call when the MS is not on a
	// confirmed TSCC.
	ErrNotOnControl = errors.New("trunk: not on a TSCC")
)

var (
	errHunt       error = &ConfigError{Field: "Hunt"}
	errTalkgroups error = &ConfigError{Field: "Talkgroups"}
	errSysCode    error = &ConfigError{Field: "SysCode"}
	errAddress    error = &ConfigError{Field: "Address"}
	errTarget     error = &ConfigError{Field: "target"}
)
