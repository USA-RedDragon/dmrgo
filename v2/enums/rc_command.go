package enums

import "fmt"

// RCCommand is the 4-bit Tier 2 Reverse Channel (RC) command. Values that
// have no constant are reserved (ETSI TS 102 361-2 clause 7.2.20).
type RCCommand uint8

// The RCCommand values.
const (
	RCCeaseTransmissionCommand RCCommand = 0b0100 // cease transmission command
	RCCeaseTransmissionRequest RCCommand = 0b0101 // cease transmission request
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
