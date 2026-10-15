package layer3

// ETSI TS 102 361-2 - 7.2.18 Talker Alias Data Format
type TalkerAliasDataFormat uint8

const (
	TalkerAliasSevenBit TalkerAliasDataFormat = iota
	TalkerAliasISOEightBit
	TalkerAliasUTF8
	TalkerAliasUTF16
)

func (ta TalkerAliasDataFormat) String() string {
	switch ta {
	case TalkerAliasSevenBit:
		return "646 (7-bit)"
	case TalkerAliasISOEightBit:
		return "ISO (Latin)"
	case TalkerAliasUTF8:
		return "UTF-8"
	case TalkerAliasUTF16:
		return "UTF-16 BE"
	default:
		return unknownName
	}
}
