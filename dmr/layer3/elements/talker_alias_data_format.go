package elements

import "github.com/USA-RedDragon/dmrgo/dmr/bit"

// ETSI TS 102 361-2 V2.4.1 (2017-10) - 7.2.18 Talker Alias Data Format
type TalkerAliasDataFormat uint8

const (
	SevenBitCharacters TalkerAliasDataFormat = iota
	ISOEightBitCharacters
	UTF8Characters
	UTF16Characters
)

func NewTalkerAliasDataFormatFromBits(infoBits [2]bit.Bit) TalkerAliasDataFormat {
	var ta TalkerAliasDataFormat
	if infoBits[0] == 1 {
		ta += 1
	}
	if infoBits[1] == 1 {
		ta += 2
	}

	return ta
}

func TalkerAliasDataFormatToName(ta TalkerAliasDataFormat) string {
	switch ta {
	case SevenBitCharacters:
		return "646 (7-bit)"
	case ISOEightBitCharacters:
		return "ISO (Latin)"
	case UTF8Characters:
		return "UTF-8"
	case UTF16Characters:
		return "UTF-16 LE"
	default:
		return "Unknown"
	}
}
