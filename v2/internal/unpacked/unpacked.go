// Package unpacked holds bits one Bit per bit, the first bit most
// significant, and converts them to and from packed bytes, for the
// bit-per-bit reference codecs and the tests. The library API takes
// packed bytes. A function panics when a slice is shorter than the bits it
// reads or writes.
package unpacked

// Bit is one bit stored in a byte, 0 or 1. A function that reads a Bit uses
// only its low bit, so a Bit of 2 reads as 0 and a Bit of 0xFF reads as 1.
type Bit byte

// Mask clears all but the low bit of each element of b in place, so code
// reads a soft-decision or out-of-range Bit as 0 or 1.
func Mask(b []Bit) {
	for i := range b {
		b[i] &= 1
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
// the first byte. The last byte is padded with 0 bits. It allocates the
// slice; AppendPack does not.
func Pack(bits []Bit) []byte {
	return AppendPack(make([]byte, 0, (len(bits)+7)/8), bits)
}

// AppendPack appends the bytes that Pack returns for bits to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendPack(dst []byte, bits []Bit) []byte {
	for i := 0; i < len(bits); i += 8 {
		var b byte
		for j, x := range bits[i:min(i+8, len(bits))] {
			b |= byte(x&1) << (7 - j)
		}
		dst = append(dst, b)
	}
	return dst
}

// Unpack is the reverse of Pack: it returns the len(data)*8 bits of data.
// It allocates the slice; AppendUnpack does not.
func Unpack(data []byte) []Bit {
	return AppendUnpack(make([]Bit, 0, len(data)*8), data)
}

// AppendUnpack appends the bits that Unpack returns for data to dst and
// returns the extended slice. It allocates only when dst lacks the capacity.
func AppendUnpack(dst []Bit, data []byte) []Bit {
	for _, b := range data {
		dst = append(dst, Bit(b>>7&1), Bit(b>>6&1), Bit(b>>5&1), Bit(b>>4&1), Bit(b>>3&1), Bit(b>>2&1), Bit(b>>1&1), Bit(b&1))
	}
	return dst
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

// UnpackAt writes len(dst) bits of src from bit start into dst, one Bit
// per bit.
func UnpackAt(dst []Bit, src []byte, start int) {
	if start&7 == 0 {
		for i := range dst {
			dst[i] = Bit(src[start>>3+i>>3] >> (7 - i&7) & 1)
		}
		return
	}
	for i := range dst {
		j := start + i
		dst[i] = Bit(src[j>>3] >> (7 - j&7) & 1)
	}
}

// PackAt writes the low bits of src into dst from bit start. The other bits
// of dst stay as they are.
func PackAt(dst []byte, start int, src []Bit) {
	if start&7 == 0 {
		whole := len(src) &^ 7
		for i := 0; i < whole; i += 8 {
			var x byte
			for _, b := range src[i : i+8] {
				x = x<<1 | byte(b&1)
			}
			dst[start>>3+i>>3] = x
		}
		src, start = src[whole:], start+whole
	}
	for i, b := range src {
		j := start + i
		dst[j>>3] = dst[j>>3]&^(0x80>>(j&7)) | byte(b&1)<<(7-j&7)
	}
}
