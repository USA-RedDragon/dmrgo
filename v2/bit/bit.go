// Package bit provides the Bit type and utility functions for converting between
// packed byte representations and unpacked bit arrays used throughout the DMR codec.
//
// Throughout the dmrgo codebase, individual bits are stored as Bit values (0 or 1)
// in arrays/slices, NOT as packed bitfields. For example, [96]Bit holds 96 individual
// bits. Packed bytes are only used at I/O boundaries (the 33-byte input and encoded output).
//
// The functions index their arguments directly, so a slice shorter than the
// bits they read or write panics, as an out of range index does.
package bit

import "fmt"

// Bit represents a single bit as a byte value (0 or 1).
// This type replaces the ambiguous use of byte for unpacked bit arrays throughout
// the codebase, providing compile-time type safety at zero runtime cost.
//
// Functions of this module that read a Bit use only its low bit, so a Bit
// of 2 reads as 0 and a Bit of 0xFF reads as 1.
type Bit byte

// FieldRangeError is the error of a PDU Encode method when a field value
// does not fit in its on-air bits, or for a coordinate when it is outside
// [Min, Max].
type FieldRangeError struct {
	// Type and Field name the PDU type and its field.
	Type, Field string
	// Value is the field value.
	Value any
	// Bits is the width of the field and Signed is set for a two's
	// complement field.
	Bits   int
	Signed bool
	// Min and Max are the range of a coordinate field; both are 0 for an
	// integer field.
	Min, Max float64
}

func (e *FieldRangeError) Error() string {
	if e.Min != e.Max {
		return fmt.Sprintf("%s.%s: value %v is outside [%v, %v]", e.Type, e.Field, e.Value, e.Min, e.Max)
	}
	signed := ""
	if e.Signed {
		signed = "signed "
	}
	return fmt.Sprintf("%s.%s: value %d does not fit in %d %sbits", e.Type, e.Field, e.Value, e.Bits, signed)
}

// Mask clears all but the low bit of each element of bits in place.
func Mask(bits []Bit) {
	for i := range bits {
		bits[i] &= 1
	}
}

// Uint returns bits as an unsigned integer, the first bit most significant.
// It reads at most the last 64 bits of a longer slice.
func Uint(bits []Bit) uint64 {
	var v uint64
	for _, b := range bits {
		v = v<<1 | uint64(b&1)
	}
	return v
}

// PutUint writes the low len(dst) bits of v into dst, the most significant
// of them first. Bits of dst beyond the 64th from the end are set to 0.
func PutUint(dst []Bit, v uint64) {
	for i := len(dst) - 1; i >= 0; i-- {
		dst[i] = Bit(v & 1)
		v >>= 1
	}
}

// Pack packs bits into bytes, the first bit in the most significant bit of
// the first byte. The last byte is padded with 0 bits.
func Pack(bits []Bit) []byte {
	data := make([]byte, (len(bits)+7)/8)
	for i, b := range bits {
		if b&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// Unpack is the reverse of Pack: it returns the len(data)*8 bits of data.
func Unpack(data []byte) []Bit {
	bits := make([]Bit, len(data)*8)
	for i := range bits {
		if (data[i/8]>>(7-(i%8)))&1 == 1 { //nolint:gosec // i/8 bounded: i < len(data)*8, so i/8 < len(data)
			bits[i] = 1
		}
	}
	return bits
}

// Unpack264 returns the 264 bits of a 33-byte burst, as Unpack does.
func Unpack264(data [33]byte) [264]Bit {
	var bits [264]Bit
	for i := 0; i < 264; i++ {
		if (data[i/8]>>(7-(i%8)))&1 != 0 { //nolint:gosec // i/8 bounded: i < 264, so i/8 <= 32 < 33
			bits[i] = 1
		}
	}
	return bits
}

// Pack264 packs the 264 bits of a burst into 33 bytes, as Pack does.
func Pack264(bits [264]Bit) [33]byte {
	var data [33]byte
	for i := 0; i < 264; i++ {
		if bits[i]&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}
