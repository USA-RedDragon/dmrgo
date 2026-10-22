package dmrgo

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// ErrNilReceiver is returned by a method of the library that returns an
// error when it is called on a nil pointer. Match it with errors.Is.
var ErrNilReceiver = errors.New("dmrgo: method called on a nil receiver")

// ErrNilArgument is returned by a function or method of the library that
// returns an error when a pointer, func or interface argument it needs is
// nil. Match it with errors.Is.
var ErrNilArgument = errors.New("dmrgo: nil argument")

// ErrZeroValue is returned by a method of a state machine that starts work,
// such as Start, Request or Send, when the machine is a zero value and not
// made with its constructor. Match it with errors.Is.
var ErrZeroValue = errors.New("dmrgo: zero value; make it with its constructor")

// FieldRangeError is the error of an encoder or setter when a field value
// does not fit in its on-air bits, when a padding bit of a packed field is
// set, or for a coordinate or a narrowed integer when it is outside [Min,
// Max]. The mmdvm decoders also return it for a value that the modem
// firmware refuses. Each field has one static FieldRangeError, so a call
// that fails does not allocate; it does not hold the value that failed.
type FieldRangeError struct {
	// Type and Field name the PDU type and its field, such as
	// "FLCGroupVoice" and "GroupAddress". For a value that a method takes,
	// Field is the method, such as "SysCode" and "Unpack14". For a value
	// that a package-level function or type takes, Type is the package and
	// Field the function or type, such as "vocoder" and "AMBEFrame".
	Type, Field string
	// Bits is the width of the field in bits.
	Bits int
	// Signed is true for a two's complement field.
	Signed bool
	// Min and Max are the range of a coordinate field, or of an integer
	// field that takes fewer values than its bits hold; both are 0 for
	// another field.
	Min, Max float64
}

// Error returns the message of e. It allocates the message.
func (e *FieldRangeError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Min != e.Max {
		return e.Type + "." + e.Field + ": value is outside [" + text.Float(e.Min) + ", " + text.Float(e.Max) + "]"
	}
	signed := ""
	if e.Signed {
		signed = "signed "
	}
	return e.Type + "." + e.Field + ": value does not fit in " + text.Int(int64(e.Bits)) + " " + signed + "bits"
}

// ConfigError is the error of a constructor, and of a machine method that
// takes a config, option or argument field, for a field with a value out of
// range. Most fields have one static ConfigError, so a call that fails does
// not allocate. Match it with errors.As.
type ConfigError struct {
	// Type is the package of the call, such as "voice" or "trunk", and Field
	// the name of the field, such as "ColorCode" or "Header.Group".
	Type, Field string
}

// Error returns the message of e. It allocates the message.
func (e *ConfigError) Error() string {
	if e == nil {
		return "<nil>"
	}
	return e.Type + ": " + e.Field + " is out of range"
}
