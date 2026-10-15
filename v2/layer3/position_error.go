package layer3

// PositionError is the 3-bit accuracy of a GPS position (ETSI TS 102 361-2
// clause 7.2.15).
type PositionError uint8

const (
	PositionLessThan2M    PositionError = iota // less than 2 m
	PositionLessThan20M                        // less than 20 m
	PositionLessThan200M                       // less than 200 m
	PositionLessThan2KM                        // less than 2 km
	PositionLessThan20KM                       // less than 20 km
	PositionLessThan200KM                      // less than or equal to 200 km
	PositionMoreThan200KM                      // more than 200 km
	PositionErrorUnknown                       // position error not known
)

func (pe PositionError) String() string {
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
		return unknownName
	}
}
