package dmrgo

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// FieldRangeError is the error of an encoder or setter when a field value
// does not fit in its on-air bits, when a padding bit of a packed field is
// set, or for a coordinate when it is outside [Min, Max]. Each field has one
// static FieldRangeError, so a call that fails does not allocate; it does
// not hold the value that failed.
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
	// Min and Max are the range of a coordinate field; both are 0 for an
	// integer field.
	Min, Max float64
}

// Error returns the message of e. It allocates the message.
func (e *FieldRangeError) Error() string {
	if e.Min != e.Max {
		return e.Type + "." + e.Field + ": value is outside [" + text.Float(e.Min) + ", " + text.Float(e.Max) + "]"
	}
	signed := ""
	if e.Signed {
		signed = "signed "
	}
	return e.Type + "." + e.Field + ": value does not fit in " + text.Int(e.Bits) + " " + signed + "bits"
}
