package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// DDFormat is the 6-bit Defined Data format (DD) of a defined data header:
// the coding of the data in its blocks. Values that have no constant are
// reserved (ETSI TS 102 361-1 clause 9.3.38, Table 9.50).
type DDFormat uint8

// The DDFormat values. ISO/IEC 8859-12 does not exist, so 8859-13 follows
// 8859-11.
const (
	DDBinary        DDFormat = 0b000000 // binary
	DDBCD           DDFormat = 0b000001 // BCD
	DD7Bit          DDFormat = 0b000010 // 7-bit characters
	DDISO8859Part1  DDFormat = 0b000011 // 8-bit ISO/IEC 8859-1
	DDISO8859Part2  DDFormat = 0b000100 // 8-bit ISO/IEC 8859-2
	DDISO8859Part3  DDFormat = 0b000101 // 8-bit ISO/IEC 8859-3
	DDISO8859Part4  DDFormat = 0b000110 // 8-bit ISO/IEC 8859-4
	DDISO8859Part5  DDFormat = 0b000111 // 8-bit ISO/IEC 8859-5
	DDISO8859Part6  DDFormat = 0b001000 // 8-bit ISO/IEC 8859-6
	DDISO8859Part7  DDFormat = 0b001001 // 8-bit ISO/IEC 8859-7
	DDISO8859Part8  DDFormat = 0b001010 // 8-bit ISO/IEC 8859-8
	DDISO8859Part9  DDFormat = 0b001011 // 8-bit ISO/IEC 8859-9
	DDISO8859Part10 DDFormat = 0b001100 // 8-bit ISO/IEC 8859-10
	DDISO8859Part11 DDFormat = 0b001101 // 8-bit ISO/IEC 8859-11
	DDISO8859Part13 DDFormat = 0b001110 // 8-bit ISO/IEC 8859-13
	DDISO8859Part14 DDFormat = 0b001111 // 8-bit ISO/IEC 8859-14
	DDISO8859Part15 DDFormat = 0b010000 // 8-bit ISO/IEC 8859-15
	DDISO8859Part16 DDFormat = 0b010001 // 8-bit ISO/IEC 8859-16
	DDUTF8          DDFormat = 0b010010 // Unicode UTF-8
	DDUTF16         DDFormat = 0b010011 // Unicode UTF-16, big-endian unless a byte order mark says otherwise
	DDUTF16BE       DDFormat = 0b010100 // Unicode UTF-16BE
	DDUTF16LE       DDFormat = 0b010101 // Unicode UTF-16LE
	DDUTF32         DDFormat = 0b010110 // Unicode UTF-32
	DDUTF32BE       DDFormat = 0b010111 // Unicode UTF-32BE
	DDUTF32LE       DDFormat = 0b011000 // Unicode UTF-32LE
)

// String returns the name of f. It allocates only for a value without a
// name.
func (f DDFormat) String() string {
	switch {
	case f == DDBinary:
		return "Binary"
	case f == DDBCD:
		return "BCD"
	case f == DD7Bit:
		return "7-bit"
	case f >= DDISO8859Part1 && f <= DDISO8859Part11:
		return "ISO 8859-" + text.Int(int64(f-DDISO8859Part1+1))
	case f >= DDISO8859Part13 && f <= DDISO8859Part16:
		return "ISO 8859-" + text.Int(int64(f-DDISO8859Part13+13))
	case f == DDUTF8:
		return "UTF-8"
	case f == DDUTF16:
		return "UTF-16"
	case f == DDUTF16BE:
		return "UTF-16BE"
	case f == DDUTF16LE:
		return "UTF-16LE"
	case f == DDUTF32:
		return "UTF-32"
	case f == DDUTF32BE:
		return "UTF-32BE"
	case f == DDUTF32LE:
		return "UTF-32LE"
	}
	return "Reserved DD(" + text.Int(int64(f)) + ")"
}
