package enums

import "fmt"

// ETSI TS 102 361-2 — Table B.3 / ETSI TS 102 361-4 — Table B.2
// Short Link Control Opcode (SLCO)
type SLCO uint8

const (
	SLCONullMessage    SLCO = 0b0000
	SLCOActivityUpdate SLCO = 0b0001
	SLCOCSysParms      SLCO = 0b0010 // ETSI TS 102 361-4 §7.1.2
	SLCOPSysParms      SLCO = 0b0011 // ETSI TS 102 361-4 §7.1.2
)

func (slco SLCO) String() string {
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
	if slco >= 0b1100 && slco <= 0b1111 {
		return fmt.Sprintf("Manufacturer Selectable SLCO(%d)", int(slco))
	}
	return fmt.Sprintf("Reserved SLCO(%d)", int(slco))
}
