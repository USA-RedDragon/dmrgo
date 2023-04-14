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
