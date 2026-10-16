// Package bit holds the Bit type, one bit stored in a byte, and
// FieldRangeError, the error of the PDU Encode methods. The API of dmrgo
// takes and returns bits packed in bytes, the first bit in the most
// significant bit of byte 0.
package bit

import (
	"fmt"
)

// Bit is one bit stored in a byte, 0 or 1. A function that reads a Bit uses
// only its low bit, so a Bit of 2 reads as 0 and a Bit of 0xFF reads as 1.
type Bit byte

// FieldRangeError is the error of a PDU Encode method when a field value
// does not fit in its on-air bits, or for a coordinate when it is outside
// [Min, Max]. Each field has one static FieldRangeError, so an Encode that
// fails does not allocate; it does not hold the value that failed.
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
