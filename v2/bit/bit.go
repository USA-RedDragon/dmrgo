// Package bit provides the Bit type and utility functions for converting between
// packed byte representations and unpacked bit arrays used throughout the DMR codec.
//
// Throughout the dmrgo codebase, individual bits are stored as Bit values (0 or 1)
// in arrays/slices, NOT as packed bitfields. For example, [96]Bit holds 96 individual
// bits. Packed bytes are only used at I/O boundaries (the 33-byte input and encoded output).
package bit

// Bit represents a single bit as a byte value (0 or 1).
// This type replaces the ambiguous use of byte for unpacked bit arrays throughout
// the codebase, providing compile-time type safety at zero runtime cost.
type Bit byte

// BitsToUint8 extracts a uint8 value from a contiguous run of bits.
// Bits are read MSB-first from bits[start] through bits[start+length-1].
func BitsToUint8(bits []Bit, start, length int) uint8 {
	var val uint8
	for i := 0; i < length; i++ {
		val <<= 1
		val |= uint8(bits[start+i])
	}
	return val
}

// BitsToUint16 extracts a uint16 value from a contiguous run of bits.
// Bits are read MSB-first from bits[start] through bits[start+length-1].
func BitsToUint16(bits []Bit, start, length int) uint16 {
	var val uint16
	for i := 0; i < length; i++ {
		val <<= 1
		val |= uint16(bits[start+i])
	}
	return val
}

// BitsToUint32 extracts a uint32 value from a contiguous run of bits.
// Bits are read MSB-first from bits[start] through bits[start+length-1].
func BitsToUint32(bits []Bit, start, length int) uint32 {
	var val uint32
	for i := 0; i < length; i++ {
		val <<= 1
		val |= uint32(bits[start+i])
	}
	return val
}

// BitsToInt extracts a signed int value from a contiguous run of bits.
// Bits are read MSB-first from bits[start] through bits[start+length-1].
func BitsToInt(bits []Bit, start, length int) int {
	var val int
	for i := 0; i < length; i++ {
		val <<= 1
		val |= int(bits[start+i])
	}
	return val
}

// BitsToBool returns true if the bit at the given index is 1.
func BitsToBool(bits []Bit, index int) bool {
	return bits[index] == 1
}

// BitsFromUint8 converts a uint8 value into a slice of Bit of the given length.
// The MSB of the value maps to the first element of the returned slice.
func BitsFromUint8(val uint8, length int) []Bit {
	bits := make([]Bit, length)
	for i := 0; i < length; i++ {
		bits[i] = Bit((val >> (length - 1 - i)) & 1)
	}
	return bits
}

// BitsFromUint16 converts a uint16 value into a slice of Bit of the given length.
func BitsFromUint16(val uint16, length int) []Bit {
	bits := make([]Bit, length)
	for i := 0; i < length; i++ {
		bits[i] = Bit((val >> (length - 1 - i)) & 1)
	}
	return bits
}

// BitsFromUint32 converts a uint32 value into a slice of Bit of the given length.
func BitsFromUint32(val uint32, length int) []Bit {
	bits := make([]Bit, length)
	for i := 0; i < length; i++ {
		bits[i] = Bit((val >> (length - 1 - i)) & 1)
	}
	return bits
}

// PackBits packs a slice of individual Bit values into packed bytes (MSB-first).
// The number of output bytes is ceil(len(bits)/8).
func PackBits(bits []Bit) []byte {
	n := (len(bits) + 7) / 8
	data := make([]byte, n)
	for i, b := range bits {
		if b == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// BitsToValue converts up to 8 individual Bit values into a single byte.
// The first element is the most-significant bit. For example,
// BitsToValue([]{1,0,1}) returns 5.
func BitsToValue(bits []Bit) byte {
	var v byte
	n := len(bits)
	for i, b := range bits {
		if b == 1 {
			v |= 1 << uint(n-1-i) //nolint:gosec // n-1-i is always non-negative: i ranges [0, n-1]
		}
	}
	return v
}

// UnpackBits unpacks packed bytes into individual Bit values (MSB-first).
// Returns a slice of len(data)*8 Bit values.
func UnpackBits(data []byte) []Bit {
	bits := make([]Bit, len(data)*8)
	for i := range bits {
		if (data[i/8]>>(7-(i%8)))&1 == 1 { //nolint:gosec // i/8 bounded: i < len(data)*8, so i/8 < len(data)
			bits[i] = 1
		}
	}
	return bits
}

// UnpackBytesToBits264 converts a [33]byte packed array into a [264]Bit unpacked array.
// This replaces the bytesToBits function in burst.go.
func UnpackBytesToBits264(data [33]byte) [264]Bit {
	var bits [264]Bit
	for i := 0; i < 264; i++ {
		if (data[i/8]>>(7-(i%8)))&1 != 0 { //nolint:gosec // i/8 bounded: i < 264, so i/8 <= 32 < 33
			bits[i] = 1
		}
	}
	return bits
}

// PackBits264 converts a [264]Bit unpacked array into a [33]byte packed array.
func PackBits264(bits [264]Bit) [33]byte {
	var data [33]byte
	for i := 0; i < 264; i++ {
		if bits[i] == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}
