package dmrgo

import "fmt"

// FieldRangeError is the error of an encoder when a field value does not
// fit in its on-air bits, when a padding bit of a packed field is set, or
// for a coordinate when it is outside [Min, Max]. Each field has one static
// FieldRangeError, so an encoder that fails does not allocate; it does not
// hold the value that failed.
type FieldRangeError struct {
	// Type and Field name the PDU type and its field.
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
		return fmt.Sprintf("%s.%s: value is outside [%v, %v]", e.Type, e.Field, e.Min, e.Max)
	}
	signed := ""
	if e.Signed {
		signed = "signed "
	}
	return fmt.Sprintf("%s.%s: value does not fit in %d %sbits", e.Type, e.Field, e.Bits, signed)
}
