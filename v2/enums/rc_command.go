package enums

import "fmt"

// ETSI TS 102 361-2 - 7.2.20 Tier 2 Reverse Channel (RC) Command
type RCCommand uint8

const (
	RCCeaseTransmissionCommand RCCommand = 0b0100
	RCCeaseTransmissionRequest RCCommand = 0b0101
)

func (rc RCCommand) String() string {
	switch rc {
	case RCCeaseTransmissionCommand:
		return "Cease Transmission Command"
	case RCCeaseTransmissionRequest:
		return "Cease Transmission Request"
	}
	return fmt.Sprintf("Reserved RCCommand(%d)", int(rc))
}
