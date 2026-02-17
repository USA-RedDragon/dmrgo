package elements

import "github.com/USA-RedDragon/dmrgo/dmr/bit"

// ETSI TS 102 361-2 V2.4.1 (2017-10) - 7.2.15 Position Error
type PositionError struct {
	PositionLessThan2M    bool
	PositionLessThan20M   bool
	PositionLessThan200M  bool
	PositionLessThan2KM   bool
	PositionLessThan20KM  bool
	PositionLessThan200KM bool
	PositionMoreThan200KM bool

	PositionErrorUnknown bool
}

func (pe *PositionError) ToString() string {
	ret := "PositionError{ "

	switch {
	case pe.PositionErrorUnknown:
		ret += "PositionErrorUnknown: true"
	case pe.PositionMoreThan200KM:
		ret += "PositionMoreThan200KM: true"
	case pe.PositionLessThan200KM:
		ret += "PositionLessThan200KM: true"
	case pe.PositionLessThan20KM:
		ret += "PositionLessThan20KM: true"
	case pe.PositionLessThan2KM:
		ret += "PositionLessThan2KM: true"
	case pe.PositionLessThan200M:
		ret += "PositionLessThan200M: true"
	case pe.PositionLessThan20M:
		ret += "PositionLessThan20M: true"
	case pe.PositionLessThan2M:
		ret += "PositionLessThan2M: true"
	}

	ret += " }"
	return ret
}

func NewPositionErrorFromBits(infoBits [3]bit.Bit) *PositionError {
	intForm := 0
	for i := 0; i < 3; i++ {
		if infoBits[i] == 1 {
			intForm |= 1 << (2 - i)
		}
	}

	switch intForm {
	case 0b000:
		return &PositionError{PositionLessThan2M: true}
	case 0b001:
		return &PositionError{PositionLessThan20M: true}
	case 0b010:
		return &PositionError{PositionLessThan200M: true}
	case 0b011:
		return &PositionError{PositionLessThan2KM: true}
	case 0b100:
		return &PositionError{PositionLessThan20KM: true}
	case 0b101:
		return &PositionError{PositionLessThan200KM: true}
	case 0b110:
		return &PositionError{PositionMoreThan200KM: true}
	case 0b111:
		return &PositionError{PositionErrorUnknown: true}
	}

	return nil
}
