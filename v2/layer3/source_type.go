package layer3

const unknownName = "Unknown"

// ETSI TS 102 361-2 - 7.2.5 Source Type
type SourceType uint8

const (
	SourceTypeBS SourceType = 0 // Base Station
	SourceTypeMS SourceType = 1 // Mobile Station
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
