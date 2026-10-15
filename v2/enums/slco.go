package enums

import "fmt"

// SLCO is the 4-bit Short Link Control Opcode of a short LC. Values 12 to 15
// are manufacturer selectable, and other values that have no constant are
// reserved (ETSI TS 102 361-2 Table B.3, ETSI TS 102 361-4 Table B.2).
type SLCO uint8

// The SLCO values.
const (
	SLCONullMessage    SLCO = 0b0000 // Nul_Msg, null message
	SLCOActivityUpdate SLCO = 0b0001 // Act_Updt, activity update
	SLCOCSysParms      SLCO = 0b0010 // C_SYS_Parms (ETSI TS 102 361-4 clause 7.1.2)
	SLCOPSysParms      SLCO = 0b0011 // P_SYS_Parms (ETSI TS 102 361-4 clause 7.1.2)
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
