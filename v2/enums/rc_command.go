package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// RCCommand is the 4-bit Reverse Channel (RC) command. Tier 2 uses the
// cease values, and Tier III adds the power control values. Values that
// have no constant are reserved (ETSI TS 102 361-2 clause 7.2.20, ETSI TS
// 102 361-4 V1.12.1 clause 6.4.14.1, Table 6.32).
type RCCommand uint8

// The RCCommand values.
const (
	RCIncreasePower            RCCommand = 0b0000 // increase power by one step, Tier III
	RCDecreasePower            RCCommand = 0b0001 // decrease power by one step, Tier III
	RCHighestPower             RCCommand = 0b0010 // set power to highest, Tier III
	RCLowestPower              RCCommand = 0b0011 // set power to lowest, Tier III
	RCCeaseTransmissionCommand RCCommand = 0b0100 // cease transmission command
	RCCeaseTransmissionRequest RCCommand = 0b0101 // cease transmission request
)

// String returns the name of the RCCommand. It allocates only for a value
// without a name.
func (rc RCCommand) String() string {
	switch rc {
	case RCIncreasePower:
		return "Increase Power"
	case RCDecreasePower:
		return "Decrease Power"
	case RCHighestPower:
		return "Highest Power"
	case RCLowestPower:
		return "Lowest Power"
	case RCCeaseTransmissionCommand:
		return "Cease Transmission Command"
	case RCCeaseTransmissionRequest:
		return "Cease Transmission Request"
	}
	return "RCCommand(" + text.Int(int64(rc)) + ")"
}
