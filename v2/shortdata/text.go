package shortdata

import (
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
)

// DecodeText appends the text in data, coded as format says, to dst as UTF-8 and
// returns dst. The text ends at the first NUL character, so the pad octets
// of the last block are dropped, and its trailing DEL characters, which a
// block filled with 1 bits leaves, are dropped too. Binary data is appended unchanged, NUL
// octets included. BCD holds the dialed digits 0 to 9, '*' and '#', two
// to an octet with the first in the high nibble, and ends at the first
// nibble 1111 (DigitNULL). 7-bit text is ISO/IEC 646 characters packed
// without gaps, most significant bit first; the bits after the last whole
// character and the trailing DEL characters, which pad bits of 1 give, are
// dropped. Octets 0x80 to 0x9F of ISO/IEC 8859 are the C1 controls. UTF-16
// and UTF-32 without a byte order mark are big-endian; a byte order mark
// selects the order and is dropped. A reserved BCD digit, an octet that
// the ISO/IEC 8859 part does not define, invalid UTF-8 and an invalid
// UTF-16 or UTF-32 code unit give U+FFFD. It returns dst unchanged and
// ErrFormat for a reserved format. It allocates only to grow dst. (ETSI TS
// 102 361-1 clause 9.3.38, ETSI TS 102 361-4 clauses 7.2.9, B.3.3 and
// B.3.4)
func DecodeText(dst []byte, format enums.DDFormat, data []byte) ([]byte, error) {
	from := len(dst)
	switch format {
	case enums.DDUTF8:
		return wire.TrimDEL(utf8Text(dst, data), from), nil
	case enums.DDISO8859Part1, enums.DDISO8859Part2, enums.DDISO8859Part3, enums.DDISO8859Part4, enums.DDISO8859Part5,
		enums.DDISO8859Part6, enums.DDISO8859Part7, enums.DDISO8859Part8, enums.DDISO8859Part9, enums.DDISO8859Part10,
		enums.DDISO8859Part11, enums.DDISO8859Part13, enums.DDISO8859Part14, enums.DDISO8859Part15, enums.DDISO8859Part16:
		return wire.TrimDEL(iso8859Text(dst, format, data), from), nil
	case enums.DDUTF16BE:
		return wire.AppendUTF8(dst, data, true), nil
	case enums.DDUTF16LE:
		return wire.AppendUTF8(dst, data, false), nil
	case enums.DDUTF16:
		switch {
		case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
			return wire.AppendUTF8(dst, data[2:], false), nil
		case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
			return wire.AppendUTF8(dst, data[2:], true), nil
		}
		return wire.AppendUTF8(dst, data, true), nil
	case enums.DDUTF32BE:
		return wire.TrimDEL(utf32Text(dst, data, 0, true), from), nil
	case enums.DDUTF32LE:
		return wire.TrimDEL(utf32Text(dst, data, 0, false), from), nil
	case enums.DDUTF32:
		return wire.TrimDEL(utf32BOMText(dst, data), from), nil
	case enums.DDBinary:
		return append(dst, data...), nil
	case enums.DDBCD:
		return bcdText(dst, data), nil
	case enums.DD7Bit:
		return septetText(dst, data), nil
	}
	return dst, ErrFormat
}

func utf8Text(dst, data []byte) []byte {
	for i := 0; i < len(data); {
		if data[i] == 0 {
			break
		}
		r, n := utf8.DecodeRune(data[i:])
		dst = wire.AppendRune(dst, r)
		i += n
	}
	return dst
}

func iso8859Text(dst []byte, format enums.DDFormat, data []byte) []byte {
	for _, c := range data {
		if c == 0 {
			break
		}
		r := rune(c)
		if c >= 0xA0 && format != enums.DDISO8859Part1 {
			r = rune(iso8859High[format-enums.DDISO8859Part2][c-0xA0])
			if r == 0 {
				r = utf8.RuneError
			}
		}
		dst = wire.AppendRune(dst, r)
	}
	return dst
}

func utf32BOMText(dst, data []byte) []byte {
	switch {
	case len(data) >= 4 && data[0] == 0xFF && data[1] == 0xFE && data[2] == 0 && data[3] == 0:
		return utf32Text(dst, data, 4, false)
	case len(data) >= 4 && data[0] == 0 && data[1] == 0 && data[2] == 0xFE && data[3] == 0xFF:
		return utf32Text(dst, data, 4, true)
	}
	return utf32Text(dst, data, 0, true)
}

func bcdText(dst, data []byte) []byte {
	for _, c := range data {
		for _, d := range [2]byte{c >> 4, c & 0xF} {
			if d == 0xF {
				return dst
			}
			dst = wire.AppendRune(dst, bcdDigit(d))
		}
	}
	return dst
}

func septetText(dst, data []byte) []byte {
	del := 0
	for k := range len(data) * 8 / 7 {
		c := septet(data, 7*k)
		switch c {
		case 0:
			return dst
		case 0x7F:
			del++
			continue
		}
		for ; del > 0; del-- {
			dst = append(dst, 0x7F)
		}
		dst = append(dst, c)
	}
	return dst
}

// AppendText appends the UTF-8 text to dst, coded as format says, and
// returns dst. Binary data is the text unchanged. BCD takes the characters
// 0 to 9, '*' and '#', and fills the last octet with the nibble 1111 when
// the number of digits is odd. 7-bit text takes the characters below
// U+0080 and fills the last octet with bits of 1. UTF-16 and UTF-32 are
// written big-endian after a byte order mark. It writes no NUL. It returns
// dst unchanged and ErrFormat for a reserved format, or ErrCharacter for a
// character that BCD, 7-bit text or the ISO/IEC 8859 part cannot hold.
// Invalid UTF-8 gives U+FFFD, or ErrCharacter for BCD, 7-bit text and
// ISO/IEC 8859. It allocates only to grow dst. (ETSI TS 102 361-1 clause
// 9.3.38, ETSI TS 102 361-4 clauses 7.2.9, B.3.3 and B.3.4)
func AppendText(dst []byte, format enums.DDFormat, text []byte) ([]byte, error) {
	switch format {
	case enums.DDUTF8:
		for i := 0; i < len(text); {
			r, n := utf8.DecodeRune(text[i:])
			dst = wire.AppendRune(dst, r)
			i += n
		}
		return dst, nil
	case enums.DDISO8859Part1, enums.DDISO8859Part2, enums.DDISO8859Part3, enums.DDISO8859Part4, enums.DDISO8859Part5,
		enums.DDISO8859Part6, enums.DDISO8859Part7, enums.DDISO8859Part8, enums.DDISO8859Part9, enums.DDISO8859Part10,
		enums.DDISO8859Part11, enums.DDISO8859Part13, enums.DDISO8859Part14, enums.DDISO8859Part15, enums.DDISO8859Part16:
		n := len(dst)
		for i := 0; i < len(text); {
			r, k := utf8.DecodeRune(text[i:])
			c, ok := iso8859Octet(format, r)
			if !ok || r == utf8.RuneError {
				return dst[:n], ErrCharacter
			}
			dst = append(dst, c)
			i += k
		}
		return dst, nil
	case enums.DDUTF16BE:
		return wire.AppendUTF16(dst, text, true), nil
	case enums.DDUTF16:
		return wire.AppendUTF16(append(dst, 0xFE, 0xFF), text, true), nil
	case enums.DDUTF16LE:
		return wire.AppendUTF16(dst, text, false), nil
	case enums.DDUTF32BE:
		return appendUTF32(dst, text, true), nil
	case enums.DDUTF32:
		return appendUTF32(append(dst, 0, 0, 0xFE, 0xFF), text, true), nil
	case enums.DDUTF32LE:
		return appendUTF32(dst, text, false), nil
	case enums.DDBinary:
		return append(dst, text...), nil
	case enums.DDBCD:
		n := len(dst)
		for i := 0; i < len(text); i += 2 {
			hi, ok1 := bcdNibble(text[i])
			lo, ok2 := byte(0xF), true
			if i+1 < len(text) {
				lo, ok2 = bcdNibble(text[i+1])
			}
			if !ok1 || !ok2 {
				return dst[:n], ErrCharacter
			}
			dst = append(dst, hi<<4|lo)
		}
		return dst, nil
	case enums.DD7Bit:
		n := len(dst)
		var acc uint16
		bits := 0
		for _, c := range text {
			if c >= 0x80 {
				return dst[:n], ErrCharacter
			}
			acc = acc<<7 | uint16(c)
			bits += 7
			if bits >= 8 {
				bits -= 8
				dst = append(dst, byte(acc>>bits&0xFF))
			}
		}
		if bits > 0 {
			dst = append(dst, byte(acc<<(8-bits)&0xFF)|byte(0xFF>>bits))
		}
		return dst, nil
	}
	return dst, ErrFormat
}

func utf32Text(dst, data []byte, start int, bigEndian bool) []byte {
	for i := start; i+3 < len(data); i += 4 {
		var r uint32
		if bigEndian {
			r = uint32(data[i])<<24 | uint32(data[i+1])<<16 | uint32(data[i+2])<<8 | uint32(data[i+3])
		} else {
			r = uint32(data[i+3])<<24 | uint32(data[i+2])<<16 | uint32(data[i+1])<<8 | uint32(data[i])
		}
		if r == 0 {
			break
		}
		if r > utf8.MaxRune {
			r = utf8.RuneError
		}
		dst = wire.AppendRune(dst, rune(r))
	}
	return dst
}

func appendUTF32(dst, text []byte, bigEndian bool) []byte {
	for i := 0; i < len(text); {
		r, n := utf8.DecodeRune(text[i:])
		i += n
		u := uint32(r & 0x1FFFFF)
		if bigEndian {
			dst = append(dst, byte(u>>24), byte(u>>16), byte(u>>8&0xFF), byte(u&0xFF))
		} else {
			dst = append(dst, byte(u&0xFF), byte(u>>8&0xFF), byte(u>>16), byte(u>>24))
		}
	}
	return dst
}

func iso8859Octet(format enums.DDFormat, r rune) (byte, bool) {
	switch {
	case r < 0xA0:
		return byte(r & 0xFF), r >= 0
	case format == enums.DDISO8859Part1:
		return byte(r & 0xFF), r <= 0xFF
	}
	row := &iso8859High[format-enums.DDISO8859Part2]
	for j := range row {
		if rune(row[j]) == r {
			return byte(0xA0 + j), true
		}
	}
	return 0, false
}

func bcdDigit(d byte) rune {
	switch {
	case d <= 9:
		return rune('0' + d)
	case d == 0xA:
		return '*'
	case d == 0xB:
		return '#'
	}
	return utf8.RuneError
}

func bcdNibble(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c == '*':
		return 0xA, true
	case c == '#':
		return 0xB, true
	}
	return 0, false
}

func septet(data []byte, bit int) byte {
	i := bit / 8
	w := uint16(data[i]) << 8
	if i+1 < len(data) {
		w |= uint16(data[i+1])
	}
	return byte(w >> (9 - bit%8) & 0x7F)
}
