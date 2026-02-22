package enums

import "fmt"

// ETSI TS 102 361-2 — Table B.3 / ETSI TS 102 361-4 — Table B.2
// Short Link Control Opcode (SLCO)
type SLCO int

const (
	SLCONullMessage    SLCO = 0b0000
	SLCOActivityUpdate SLCO = 0b0001
	SLCOCSysParms      SLCO = 0b0010 // ETSI TS 102 361-4 §7.1.2
	SLCOPSysParms      SLCO = 0b0011 // ETSI TS 102 361-4 §7.1.2
)

func SLCOToName(slco SLCO) string {
	switch slco {
	case SLCONullMessage:
		return "Null Message"
	case SLCOActivityUpdate:
		return "Activity Update"
	case SLCOCSysParms:
		return "C_SYS_Parms"
	case SLCOPSysParms:
		return "P_SYS_Parms"
	}
	return fmt.Sprintf("Reserved SLCO(%d)", int(slco))
}

func SLCOFromInt(i int) SLCO {
	switch SLCO(i) {
	case SLCONullMessage:
		return SLCONullMessage
	case SLCOActivityUpdate:
		return SLCOActivityUpdate
	case SLCOCSysParms:
		return SLCOCSysParms
	case SLCOPSysParms:
		return SLCOPSysParms
	}
	return SLCO(i)
}
