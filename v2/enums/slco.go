package enums

import "fmt"

// ETSI TS 102 361-2 - Table B.3 Short Link Control Opcode (SLCO)
type SLCO int

const (
	SLCONullMessage    SLCO = 0b0000
	SLCOActivityUpdate SLCO = 0b0001
)

func SLCOToName(slco SLCO) string {
	switch slco {
	case SLCONullMessage:
		return "Null Message"
	case SLCOActivityUpdate:
		return "Activity Update"
	}
	return fmt.Sprintf("Reserved SLCO(%d)", int(slco))
}

func SLCOFromInt(i int) SLCO {
	switch SLCO(i) {
	case SLCONullMessage:
		return SLCONullMessage
	case SLCOActivityUpdate:
		return SLCOActivityUpdate
	}
	return SLCO(i)
}
