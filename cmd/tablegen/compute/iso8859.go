package compute

import (
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// ISO8859High returns the code points of octets 0xA0 to 0xFF of ISO/IEC
// 8859 parts 2 to 11 and 13 to 16, in the order of the DD formats 0x04 to
// 0x11, 0 for an octet that the part does not define. Octets below 0xA0
// are the same in every part. Part 11 is TIS 620 with U+00A0 at 0xA0, which
// golang.org/x/text does not have (ETSI TS 102 361-1 clause 9.3.38, ISO/IEC
// 8859).
func ISO8859High() [][]uint16 {
	parts := []*charmap.Charmap{
		charmap.ISO8859_2, charmap.ISO8859_3, charmap.ISO8859_4, charmap.ISO8859_5,
		charmap.ISO8859_6, charmap.ISO8859_7, charmap.ISO8859_8, charmap.ISO8859_9,
		charmap.ISO8859_10, nil, charmap.ISO8859_13, charmap.ISO8859_14,
		charmap.ISO8859_15, charmap.ISO8859_16,
	}
	out := make([][]uint16, len(parts))
	for i, cm := range parts {
		out[i] = make([]uint16, 96)
		for j := range 96 {
			c := byte(0xA0 + j)
			if cm == nil {
				out[i][j] = thai(c)
				continue
			}
			r := cm.DecodeByte(c)
			if r != utf8.RuneError {
				out[i][j] = uint16(r)
			}
		}
	}
	return out
}

func thai(c byte) uint16 {
	switch {
	case c == 0xA0:
		return 0xA0
	case c >= 0xA1 && c <= 0xDA, c >= 0xDF && c <= 0xFB:
		return 0x0E01 + uint16(c-0xA1)
	}
	return 0
}
