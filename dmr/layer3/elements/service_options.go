package elements

import "fmt"

// ETSI TS 102 361-2 V2.4.1 (2017-10) - 7.2.1  Service Options
type ServiceOptions struct {
	IsEmergency         bool
	IsPrivacy           bool
	Reserved            [2]byte
	IsBroadcast         bool
	IsOpenVoiceCallMode bool
	PriorityLevel       uint8
}

func (so *ServiceOptions) ToString() string {
	return fmt.Sprintf("{ IsEmergency: %t, IsBroadcast: %t, IsOpenVoiceCallMode: %t, PriorityLevel: %d, IsPrivacy: %t }", so.IsEmergency, so.IsBroadcast, so.IsOpenVoiceCallMode, so.PriorityLevel, so.IsPrivacy)
}

func NewServiceOptionsFromBits(infoBits [8]byte) *ServiceOptions {
	var reservedBits [2]byte

	var priority uint8
	if infoBits[6] == 1 {
		priority += 1
	}
	if infoBits[7] == 1 {
		priority += 2
	}

	copy(reservedBits[:], infoBits[2:4])
	so := ServiceOptions{
		IsEmergency:         infoBits[0] == 1,
		IsPrivacy:           infoBits[1] == 1,
		Reserved:            reservedBits,
		IsBroadcast:         infoBits[4] == 1,
		IsOpenVoiceCallMode: infoBits[5] == 1,
		PriorityLevel:       priority,
	}

	return &so
}

func (so *ServiceOptions) ToByte() byte {
	var b byte
	if so.IsEmergency {
		b |= 1 << 7
	}
	if so.IsPrivacy {
		b |= 1 << 6
	}
	if so.Reserved[0] == 1 {
		b |= 1 << 5
	}
	if so.Reserved[1] == 1 {
		b |= 1 << 4
	}
	if so.IsBroadcast {
		b |= 1 << 3
	}
	if so.IsOpenVoiceCallMode {
		b |= 1 << 2
	}

	// Priority mapping from NewServiceOptionsFromBits:
	// infoBits[6] (Bit 1) adds 1 -> Low bit
	// infoBits[7] (Bit 0) adds 2 -> High bit

	// Priority bit 0 (value 1) -> Bit 1 of byte
	if so.PriorityLevel&1 != 0 {
		b |= 1 << 1
	}
	// Priority bit 1 (value 2) -> Bit 0 of byte
	if so.PriorityLevel&2 != 0 {
		b |= 1
	}

	return b
}
