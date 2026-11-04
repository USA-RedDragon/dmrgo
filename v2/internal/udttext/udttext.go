// Package udttext packs and unpacks the character formats of UDT appended
// data, which the trunk and sms packages share: ISO 7-bit and 8-bit
// characters and UTF-16BE (ETSI TS 102 361-4 clauses B.3.4, B.3.5, B.3.8
// and B.3.9). It does not allocate except to grow a slice of the caller.
package udttext

import (
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
)

// MaxOctets is the most octets of appended data in a UDT: four blocks of
// 12 octets, less the CRC of the last block.
const MaxOctets = 4*12 - 2

// Width returns the bits of one symbol of the format f: 7 for ISO7, 8 for
// ISO8 and LIP, 16 for UTF-16BE and Mixed, and 0 for the others.
func Width(f enums.UDTFormat) int {
	switch f {
	case enums.UDTISO7:
		return 7
	case enums.UDTISO8, enums.UDTLIP:
		return 8
	case enums.UDTUTF16BE, enums.UDTMixed:
		return 16
	case enums.UDTBinary, enums.UDTAddress, enums.UDTBCD, enums.UDTNMEA, enums.UDTIP, enums.UDTManufacturer1,
		enums.UDTManufacturer2:
	}
	return 0
}

// Bits returns the bits of appended data in blocks blocks.
func Bits(blocks int) int {
	return 96*blocks - 16
}

// Blocks returns the fewest blocks, 1 to 4, that hold n bits, or 0 when
// four do not.
func Blocks(n int) int {
	for b := 1; b <= 4; b++ {
		if n <= Bits(b) {
			return b
		}
	}
	return 0
}

// PadNibbles returns the whole nibbles in free bits, or 0 for a count out
// of range of the 5-bit Pad Nibble.
func PadNibbles(free int) uint8 {
	if free < 0 || free >= 4*32 {
		return 0
	}
	return uint8(free / 4)
}

// Fill sets bits from to end of data to 1.
func Fill(data []byte, from, end int) {
	for i := from; i < end; i++ {
		bits.SetBit(data, i, 1)
	}
}

// Count returns the symbols of width w that the UTF-8 text takes, with
// two for a character above U+FFFF in UTF-16.
func Count(text []byte, w int) int {
	n := 0
	for i := 0; i < len(text); {
		r, k := utf8.DecodeRune(text[i:])
		i += k
		n++
		if w == 16 && r >= 0x10000 {
			n++
		}
	}
	return n
}

// Set writes the symbols of width w of the UTF-8 text into data from bit
// start, and returns the bit after the last one. ok is false for a
// character that the width cannot hold. data must hold w*Count(text, w)
// bits from start. Invalid UTF-8 gives U+FFFD.
func Set(data []byte, start, w int, text []byte) (end int, ok bool) {
	o := start
	for i := 0; i < len(text); {
		r, k := utf8.DecodeRune(text[i:])
		i += k
		switch {
		case r < 0, w == 7 && r >= 0x80, w == 8 && r > 0xFF:
			return o, false
		case w == 16 && r >= 0x10000:
			r -= 0x10000
			bits.SetField(data, o, 16, uint64(0xD800+r>>10))
			bits.SetField(data, o+16, 16, uint64(0xDC00+r&0x3FF))
			o += 32
			continue
		}
		bits.SetField(data, o, w, uint64(r))
		o += w
	}
	return o, true
}

// Append appends to dst as UTF-8 the symbols of width w that fit in room
// bits of data from bit start, and returns dst. The text ends at the first
// NUL character of 7 or 8 bits, and drops its trailing DEL characters, the
// 1 bits of a block that a sender fills without pad nibbles. data must hold
// room bits from start, and start must be a whole octet. It allocates only
// to grow dst.
func Append(dst, data []byte, start, room, w int) []byte {
	if w == 0 || room < 0 {
		return dst
	}
	n := room / w
	from := len(dst)
	if w == 16 {
		return wire.TrimDEL(wire.AppendUTF8(dst, data[start/8:start/8+2*n], true), from)
	}
	for i := range n {
		c := bits.Field(data, start+w*i, w) & 0xFF
		if c == 0 {
			break
		}
		dst = wire.AppendRune(dst, rune(c))
	}
	return wire.TrimDEL(dst, from)
}
