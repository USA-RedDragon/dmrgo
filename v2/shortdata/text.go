package shortdata

import (
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/wire"
)

// Text appends the text in data, coded as format says, to dst as UTF-8 and
// returns dst. The text ends at the first NUL character, so the pad octets
// of the last block are dropped. UTF-16 without a byte order mark is
// big-endian; a byte order mark selects the order and is dropped. Invalid
// UTF-8 gives U+FFFD. It returns dst unchanged and ErrFormat for a format
// that it does not support. It allocates only to grow dst. (ETSI TS 102
// 361-1 clause 9.3.38)
func Text(dst []byte, format enums.DDFormat, data []byte) ([]byte, error) {
	switch format {
	case enums.DDUTF8:
		for i := 0; i < len(data); {
			if data[i] == 0 {
				break
			}
			r, n := utf8.DecodeRune(data[i:])
			dst = wire.AppendRune(dst, r)
			i += n
		}
		return dst, nil
	case enums.DDISO8859Part1:
		for _, c := range data {
			if c == 0 {
				break
			}
			dst = wire.AppendRune(dst, rune(c))
		}
		return dst, nil
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
	case enums.DDBinary, enums.DDBCD, enums.DD7Bit, enums.DDISO8859Part2, enums.DDISO8859Part3, enums.DDISO8859Part4,
		enums.DDISO8859Part5, enums.DDISO8859Part6, enums.DDISO8859Part7, enums.DDISO8859Part8, enums.DDISO8859Part9,
		enums.DDISO8859Part10, enums.DDISO8859Part11, enums.DDISO8859Part13, enums.DDISO8859Part14, enums.DDISO8859Part15,
		enums.DDISO8859Part16, enums.DDUTF32, enums.DDUTF32BE, enums.DDUTF32LE:
	}
	return dst, ErrFormat
}

// AppendText appends the UTF-8 text to dst, coded as format says, and
// returns dst. UTF-16 is written big-endian after a byte order mark. It
// writes no NUL. It returns dst unchanged and ErrFormat for a format that
// it does not support, or ErrCharacter for a character that ISO/IEC 8859-1
// cannot hold. Invalid UTF-8 gives U+FFFD, or ErrCharacter for ISO/IEC
// 8859-1. It allocates only to grow dst. (ETSI TS 102 361-1 clause 9.3.38)
func AppendText(dst []byte, format enums.DDFormat, text []byte) ([]byte, error) {
	switch format {
	case enums.DDUTF8:
		for i := 0; i < len(text); {
			r, n := utf8.DecodeRune(text[i:])
			dst = wire.AppendRune(dst, r)
			i += n
		}
		return dst, nil
	case enums.DDISO8859Part1:
		n := len(dst)
		for i := 0; i < len(text); {
			r, k := utf8.DecodeRune(text[i:])
			if r < 0 || r > 0xFF || r == utf8.RuneError {
				return dst[:n], ErrCharacter
			}
			dst = append(dst, byte(r))
			i += k
		}
		return dst, nil
	case enums.DDUTF16BE:
		return wire.AppendUTF16(dst, text, true), nil
	case enums.DDUTF16:
		return wire.AppendUTF16(append(dst, 0xFE, 0xFF), text, true), nil
	case enums.DDUTF16LE:
		return wire.AppendUTF16(dst, text, false), nil
	case enums.DDBinary, enums.DDBCD, enums.DD7Bit, enums.DDISO8859Part2, enums.DDISO8859Part3, enums.DDISO8859Part4,
		enums.DDISO8859Part5, enums.DDISO8859Part6, enums.DDISO8859Part7, enums.DDISO8859Part8, enums.DDISO8859Part9,
		enums.DDISO8859Part10, enums.DDISO8859Part11, enums.DDISO8859Part13, enums.DDISO8859Part14, enums.DDISO8859Part15,
		enums.DDISO8859Part16, enums.DDUTF32, enums.DDUTF32BE, enums.DDUTF32LE:
	}
	return dst, ErrFormat
}
