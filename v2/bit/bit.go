// Package bit holds DMR bits unpacked, one Bit per bit, and converts Bit
// slices and arrays to and from packed bytes and unsigned integers, the first
// bit most significant. dmrgo uses packed bytes only at I/O boundaries, such
// as a 33-byte burst. A function panics when a slice is shorter than the bits
// it reads or writes.
package bit

import (
	"fmt"
	"slices"
)

// Bit is one bit stored in a byte, 0 or 1. The functions of this module that
// read a Bit use only its low bit, so a Bit of 2 reads as 0 and a Bit of 0xFF
// reads as 1.
type Bit byte

// FieldRangeError is the error of a PDU Encode method when a field value
// does not fit in its on-air bits, or for a coordinate when it is outside
// [Min, Max].
type FieldRangeError struct {
	// Type and Field name the PDU type and its field.
	Type, Field string
	// Value is the field value.
	Value any
	// Bits is the width of the field in bits.
	Bits int
	// Signed is true for a two's complement field.
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
	return AppendPack(make([]byte, 0, (len(bits)+7)/8), bits)
}

// AppendPack appends the bytes that Pack returns for bits to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendPack(dst []byte, bits []Bit) []byte {
	n, size := len(dst), (len(bits)+7)/8
	dst = slices.Grow(dst, size)[:n+size]
	data := dst[n:]
	clear(data)
	for i, b := range bits {
		if b&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return dst
}

// Unpack is the reverse of Pack: it returns the len(data)*8 bits of data.
func Unpack(data []byte) []Bit {
	return AppendUnpack(make([]Bit, 0, len(data)*8), data)
}

// AppendUnpack appends the bits that Unpack returns for data to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendUnpack(dst []Bit, data []byte) []Bit {
	n, size := len(dst), len(data)*8
	dst = slices.Grow(dst, size)[:n+size]
	bits := dst[n:]
	for i := range bits {
		bits[i] = Bit(data[i/8]>>(7-(i%8))) & 1
	}
	return dst
}

// Unpack32 returns the 32 bits of 4 bytes, such as an embedded signalling
// field, as Unpack does.
func Unpack32(data [4]byte) [32]Bit {
	var bits [32]Bit
	for i := range 32 {
		if (data[i/8]>>(7-(i%8)))&1 != 0 {
			bits[i] = 1
		}
	}
	return bits
}

// Pack32 packs 32 bits, such as an embedded signalling field, into 4 bytes,
// as Pack does.
func Pack32(bits [32]Bit) [4]byte {
	var data [4]byte
	for i := range 32 {
		if bits[i]&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// Unpack96 returns the 96 bits of a 12-byte BPTC(196,96) payload, as Unpack
// does.
func Unpack96(data [12]byte) [96]Bit {
	var bits [96]Bit
	for i := range 96 {
		if (data[i/8]>>(7-(i%8)))&1 != 0 {
			bits[i] = 1
		}
	}
	return bits
}

// Pack96 packs 96 bits, such as the information bits of a CSBK or data
// header, into 12 bytes, as Pack does.
func Pack96(bits [96]Bit) [12]byte {
	var data [12]byte
	for i := range 96 {
		if bits[i]&1 == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// Unpack264 returns the 264 bits of a 33-byte burst, as Unpack does.
func Unpack264(data [33]byte) [264]Bit {
	var bits [264]Bit
	for i := 0; i < 264; i++ {
		if (data[i/8]>>(7-(i%8)))&1 != 0 {
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
