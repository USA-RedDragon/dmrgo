package layer3

// TalkerAliasDataFormat is the 2-bit character encoding of a talker alias
// (ETSI TS 102 361-2 clause 7.2.18, Table 7.25).
type TalkerAliasDataFormat uint8

const (
	TalkerAliasSevenBit    TalkerAliasDataFormat = iota // 7-bit characters
	TalkerAliasISOEightBit                              // ISO 8-bit characters
	TalkerAliasUTF8                                     // Unicode UTF-8
	TalkerAliasUTF16                                    // Unicode UTF-16BE
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
