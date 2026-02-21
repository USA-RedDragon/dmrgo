package elements

// ETSI TS 102 361-2 - 7.2.15 Position Error
type PositionError uint8

const (
	PositionLessThan2M    PositionError = iota // 0b000
	PositionLessThan20M                        // 0b001
	PositionLessThan200M                       // 0b010
	PositionLessThan2KM                        // 0b011
	PositionLessThan20KM                       // 0b100
	PositionLessThan200KM                      // 0b101
	PositionMoreThan200KM                      // 0b110
	PositionErrorUnknown                       // 0b111
)

func PositionErrorToName(pe PositionError) string {
	switch pe {
	case PositionLessThan2M:
		return "PositionLessThan2M"
	case PositionLessThan20M:
		return "PositionLessThan20M"
	case PositionLessThan200M:
		return "PositionLessThan200M"
	case PositionLessThan2KM:
		return "PositionLessThan2KM"
	case PositionLessThan20KM:
		return "PositionLessThan20KM"
	case PositionLessThan200KM:
		return "PositionLessThan200KM"
	case PositionMoreThan200KM:
		return "PositionMoreThan200KM"
	case PositionErrorUnknown:
		return "PositionErrorUnknown"
	default:
		return "Unknown"
	}
}

func (pe PositionError) ToString() string {
	return "PositionError{ " + PositionErrorToName(pe) + " }"
}
