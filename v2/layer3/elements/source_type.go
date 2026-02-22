package elements

// ETSI TS 102 361-2 - 7.2.5 Source Type
type SourceType uint8

const (
	SourceTypeBS SourceType = 0 // Base Station
	SourceTypeMS SourceType = 1 // Mobile Station
)

func SourceTypeToName(st SourceType) string {
	switch st {
	case SourceTypeBS:
		return "BS"
	case SourceTypeMS:
		return "MS"
	default:
		return "Unknown"
	}
}

func (st SourceType) ToString() string {
	return "SourceType{ " + SourceTypeToName(st) + " }"
}
