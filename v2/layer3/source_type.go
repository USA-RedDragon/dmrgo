package layer3

const unknownName = "Unknown"

// SourceType is the 1-bit Source Type: it tells whether a BS or an MS sent
// the PDU (ETSI TS 102 361-2 clause 7.2.5, Table 7.14).
type SourceType uint8

// The SourceType values.
const (
	SourceTypeBS SourceType = 0 // BS sourced
	SourceTypeMS SourceType = 1 // MS sourced
)

func (st SourceType) String() string {
	switch st {
	case SourceTypeBS:
		return "BS"
	case SourceTypeMS:
		return "MS"
	default:
		return unknownName
	}
}
