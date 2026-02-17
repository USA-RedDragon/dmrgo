package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
)

type CSBKOpcode byte

const (
	CSBKBSOutboundActivationPDU                 CSBKOpcode = 0b00111000
	CSBKUnitToUnitVoiceServiceRequestPDU        CSBKOpcode = 0b00000100
	CSBKUnitToUnitVoiceServiceAnswerResponsePDU CSBKOpcode = 0b00000101
	CSBKNegativeAcknowledgementPDU              CSBKOpcode = 0b00100110
	CSBKPreamblePDU                             CSBKOpcode = 0b00111101
	CSBKChannelTimingPDU                        CSBKOpcode = 0b00000111
)

func (opcode CSBKOpcode) ToString() string {
	switch opcode {
	case CSBKBSOutboundActivationPDU:
		return "BS Outbound Activation PDU"
	case CSBKUnitToUnitVoiceServiceRequestPDU:
		return "Unit-to-Unit Voice Service Request PDU"
	case CSBKUnitToUnitVoiceServiceAnswerResponsePDU:
		return "Unit-to-Unit Voice Service Answer/Response PDU"
	case CSBKNegativeAcknowledgementPDU:
		return "Negative Acknowledgement PDU"
	case CSBKPreamblePDU:
		return "Preamble PDU"
	case CSBKChannelTimingPDU:
		return "Channel Timing PDU"
	default:
		return fmt.Sprintf("Unknown CSBKOpcode: %08b", byte(opcode))
	}
}

type BSOutboundActivationPDU struct {
	Reserved      uint16
	BSAddress     [24]bit.Bit
	SourceAddress [24]bit.Bit
}

func (pdu *BSOutboundActivationPDU) DecodeFromBits(bits [64]bit.Bit) bool {
	for i := range 16 {
		pdu.Reserved <<= 1
		pdu.Reserved |= uint16(bits[i])
	}

	for i := range 24 {
		pdu.BSAddress[i] = bits[16+i]
	}

	for i := range 24 {
		pdu.SourceAddress[i] = bits[40+i]
	}

	return true
}

func (pdu BSOutboundActivationPDU) ToString() string {
	return fmt.Sprintf("BSOutboundActivationPDU{ Reserved: %d, BSAddress: %s, SourceAddress: %s }", pdu.Reserved, string(pdu.BSAddress[:]), string(pdu.SourceAddress[:]))
}

type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions byte
	Reserved       byte
	TargetAddress  [24]bit.Bit
	SourceAddress  [24]bit.Bit
}

func (pdu *UnitToUnitVoiceServiceRequestPDU) DecodeFromBits(bits [64]bit.Bit) bool {
	for i := range 8 {
		pdu.ServiceOptions <<= 1
		pdu.ServiceOptions |= byte(bits[8+i])
	}

	for i := range 8 {
		pdu.Reserved <<= 1
		pdu.Reserved |= byte(bits[16+i])
	}

	for i := range 24 {
		pdu.TargetAddress[i] = bits[24+i]
	}

	for i := range 24 {
		pdu.SourceAddress[i] = bits[48+i]
	}

	return true
}

func (pdu UnitToUnitVoiceServiceRequestPDU) ToString() string {
	return fmt.Sprintf("UnitToUnitVoiceServiceRequestPDU{ ServiceOptions: %d, Reserved: %d, TargetAddress: %s, SourceAddress: %s }", pdu.ServiceOptions, pdu.Reserved, string(pdu.TargetAddress[:]), string(pdu.SourceAddress[:]))
}

type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions byte
	AnswerResponse byte
	TargetAddress  [24]bit.Bit
	SourceAddress  [24]bit.Bit
}

func (pdu UnitToUnitVoiceServiceAnswerResponsePDU) ToString() string {
	return fmt.Sprintf("UnitToUnitVoiceServiceAnswerResponsePDU{ ServiceOptions: %d, AnswerResponse: %d, TargetAddress: %s, SourceAddress: %s }", pdu.ServiceOptions, pdu.AnswerResponse, string(pdu.TargetAddress[:]), string(pdu.SourceAddress[:]))
}

func (pdu *UnitToUnitVoiceServiceAnswerResponsePDU) DecodeFromBits(bits [64]bit.Bit) bool {
	for i := range 8 {
		pdu.ServiceOptions <<= 1
		pdu.ServiceOptions |= byte(bits[8+i])
	}

	for i := range 8 {
		pdu.AnswerResponse <<= 1
		pdu.AnswerResponse |= byte(bits[16+i])
	}

	for i := range 24 {
		pdu.TargetAddress[i] = bits[24+i]
	}

	for i := range 24 {
		pdu.SourceAddress[i] = bits[48+i]
	}

	return true
}

type NegativeAcknowledgementPDU struct {
	AdditionalInfo bool
	SourceType     bool
	ServiceType    [6]bit.Bit
	ReasonCode     byte
	SourceAddress  [24]bit.Bit
	TargetAddress  [24]bit.Bit
}

func (pdu NegativeAcknowledgementPDU) ToString() string {
	return fmt.Sprintf("NegativeAcknowledgementPDU{ AdditionalInfo: %t, SourceType: %t, ServiceType: %08b, ReasonCode: %d, SourceAddress: %s, TargetAddress: %s }", pdu.AdditionalInfo, pdu.SourceType, pdu.ServiceType, pdu.ReasonCode, string(pdu.SourceAddress[:]), string(pdu.TargetAddress[:]))
}

func (pdu *NegativeAcknowledgementPDU) DecodeFromBits(bits [64]bit.Bit) bool {
	pdu.AdditionalInfo = bits[0] == 1
	pdu.SourceType = bits[1] == 1

	for i := range 6 {
		pdu.ServiceType[i] = bits[2+i]
	}

	for i := range 8 {
		pdu.ReasonCode <<= 1
		pdu.ReasonCode |= byte(bits[8+i])
	}

	for i := range 24 {
		pdu.SourceAddress[i] = bits[16+i]
	}

	for i := range 24 {
		pdu.TargetAddress[i] = bits[40+i]
	}

	return true
}

type PreamblePDU struct {
	// 1 = data content follows, 0 = CSBK follows
	Data bool
	// 1 = target address is a group, 0 = individual
	Group              bool
	Reserved           [6]bit.Bit
	CSBKBlocksToFollow byte
	TargetAddress      [24]bit.Bit
	SourceAddress      [24]bit.Bit
}

func (pdu PreamblePDU) ToString() string {
	return fmt.Sprintf("PreamblePDU{ Data: %t, Group: %t, Reserved: %08b, CSBKBlocksToFollow: %d, TargetAddress: %v, SourceAddress: %v }", pdu.Data, pdu.Group, pdu.Reserved, pdu.CSBKBlocksToFollow, pdu.TargetAddress[:], pdu.SourceAddress[:])
}

func (pdu *PreamblePDU) DecodeFromBits(bits [64]bit.Bit) bool {
	pdu.Data = bits[0] == 1
	pdu.Group = bits[1] == 1

	for i := range 6 {
		pdu.Reserved[i] = bits[2+i]
	}

	for i := range 8 {
		pdu.CSBKBlocksToFollow <<= 1
		pdu.CSBKBlocksToFollow |= byte(bits[8+i])
	}

	for i := range 24 {
		pdu.TargetAddress[i] = bits[16+i]
	}

	for i := range 24 {
		pdu.SourceAddress[i] = bits[40+i]
	}

	return true
}

type ChannelTimingPDU struct {
	SyncAge                 [11]bit.Bit
	Generation              [5]bit.Bit
	LeaderIdentifier        [20]bit.Bit
	NewLeader               bool
	LeaderDynamicIdentifier [2]bit.Bit
	ChannelTimingOp         [2]bool
	SourceIdentifier        [20]bit.Bit
	Reserved                bool
	SourceDynamicIdentifier [2]bit.Bit
}

func (pdu ChannelTimingPDU) ToString() string {
	return fmt.Sprintf("ChannelTimingPDU{ SyncAge: %011b, Generation: %05b, LeaderIdentifier: %s, NewLeader: %t, LeaderDynamicIdentifier: %02b, SourceIdentifier: %s, Reserved: %t, SourceDynamicIdentifier: %02b, ChannelTimingOp: %v }", pdu.SyncAge, pdu.Generation, string(pdu.LeaderIdentifier[:]), pdu.NewLeader, pdu.LeaderDynamicIdentifier, string(pdu.SourceIdentifier[:]), pdu.Reserved, pdu.SourceDynamicIdentifier, pdu.ChannelTimingOp)
}

func (pdu *ChannelTimingPDU) DecodeFromBits(bits [64]bit.Bit) bool {
	for i := range 11 {
		pdu.SyncAge[i] = bits[i]
	}

	for i := range 5 {
		pdu.Generation[i] = bits[11+i]
	}

	for i := range 20 {
		pdu.LeaderIdentifier[i] = bits[16+i]
	}

	pdu.NewLeader = bits[36] == 1

	for i := range 2 {
		pdu.LeaderDynamicIdentifier[i] = bits[37+i]
	}

	pdu.ChannelTimingOp[0] = bits[39] == 1

	for i := range 20 {
		pdu.SourceIdentifier[i] = bits[40+i]
	}

	pdu.Reserved = bits[60] == 1

	for i := range 2 {
		pdu.SourceDynamicIdentifier[i] = bits[61+i]
	}

	pdu.ChannelTimingOp[1] = bits[63] == 1

	return true
}

type CSBK struct {
	dataType elements.DataType

	LastBlock   bool
	ProtectFlag bool
	CSBKOpcode  CSBKOpcode
	FID         byte

	BSOutboundActivationPDU                 *BSOutboundActivationPDU
	UnitToUnitVoiceServiceRequestPDU        *UnitToUnitVoiceServiceRequestPDU
	UnitToUnitVoiceServiceAnswerResponsePDU *UnitToUnitVoiceServiceAnswerResponsePDU
	NegativeAcknowledgementPDU              *NegativeAcknowledgementPDU
	PreamblePDU                             *PreamblePDU
	ChannelTimingPDU                        *ChannelTimingPDU

	crc uint16
}

func (csbk *CSBK) ToString() string {
	var extraData string
	switch csbk.CSBKOpcode {
	case CSBKBSOutboundActivationPDU:
		extraData = csbk.BSOutboundActivationPDU.ToString()
	case CSBKUnitToUnitVoiceServiceRequestPDU:
		extraData = csbk.UnitToUnitVoiceServiceRequestPDU.ToString()
	case CSBKUnitToUnitVoiceServiceAnswerResponsePDU:
		extraData = csbk.UnitToUnitVoiceServiceAnswerResponsePDU.ToString()
	case CSBKNegativeAcknowledgementPDU:
		extraData = csbk.NegativeAcknowledgementPDU.ToString()
	case CSBKPreamblePDU:
		extraData = csbk.PreamblePDU.ToString()
	case CSBKChannelTimingPDU:
		extraData = csbk.ChannelTimingPDU.ToString()
	default:
		extraData = "Unknown or unparsed opcode data"
	}
	return fmt.Sprintf("CSBK{ dataType: %s, LastBlock: %t, ProtectFlag: %t, CSBKOpcode: %v, FID: %d, crc: %04x, extraData: %s }", elements.DataTypeToName(csbk.dataType), csbk.LastBlock, csbk.ProtectFlag, csbk.CSBKOpcode, csbk.FID, csbk.crc, extraData)
}

func (csbk *CSBK) GetDataType() elements.DataType {
	return csbk.dataType
}

func (csbk *CSBK) DecodeFromBits(infoBits []bit.Bit, dt elements.DataType) bool {
	if len(infoBits) != 96 {
		fmt.Println("CSBK: invalid infoBits length: ", len(infoBits))
		return false
	}

	csbk.dataType = dt

	// Pack 96 info bits into 12 bytes
	var dataBytes [12]byte
	for i := range 12 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(infoBits[i*8+j])
		}
	}

	// Apply CRC mask (XOR last 2 bytes with 0xA5)
	dataBytes[10] ^= 0xA5
	dataBytes[11] ^= 0xA5

	// CRC check using table-based CCITT matching MMDVM
	if !CheckCRCCCITT(dataBytes[:]) {
		fmt.Println("CSBK: CRC check failed")
		return false
	}

	// Extract the unmasked CRC for storage
	csbk.crc = uint16(dataBytes[10])<<8 | uint16(dataBytes[11])

	// lb is the first bit
	csbk.LastBlock = infoBits[0] == 1
	// pf is the second bit
	csbk.ProtectFlag = infoBits[1] == 1

	// csbko is the next 6 bits
	for i := range 6 {
		csbk.CSBKOpcode <<= 1
		csbk.CSBKOpcode |= CSBKOpcode(infoBits[2+i])
	}

	// FID is 8 bits, infoBits[8:16] as a byte
	for i := range 8 {
		csbk.FID <<= 1
		csbk.FID |= byte(infoBits[8+i])
	}

	var pdu [64]bit.Bit
	for i := range 64 {
		pdu[i] = infoBits[16+i]
	}
	switch csbk.CSBKOpcode {
	case CSBKBSOutboundActivationPDU:
		csbk.BSOutboundActivationPDU = &BSOutboundActivationPDU{}
		if !csbk.BSOutboundActivationPDU.DecodeFromBits(pdu) {
			return false
		}
	case CSBKUnitToUnitVoiceServiceRequestPDU:
		csbk.UnitToUnitVoiceServiceRequestPDU = &UnitToUnitVoiceServiceRequestPDU{}
		if !csbk.UnitToUnitVoiceServiceRequestPDU.DecodeFromBits(pdu) {
			return false
		}
	case CSBKUnitToUnitVoiceServiceAnswerResponsePDU:
		csbk.UnitToUnitVoiceServiceAnswerResponsePDU = &UnitToUnitVoiceServiceAnswerResponsePDU{}
		if !csbk.UnitToUnitVoiceServiceAnswerResponsePDU.DecodeFromBits(pdu) {
			return false
		}
	case CSBKNegativeAcknowledgementPDU:
		csbk.NegativeAcknowledgementPDU = &NegativeAcknowledgementPDU{}
		if !csbk.NegativeAcknowledgementPDU.DecodeFromBits(pdu) {
			return false
		}
	case CSBKPreamblePDU:
		csbk.PreamblePDU = &PreamblePDU{}
		if !csbk.PreamblePDU.DecodeFromBits(pdu) {
			return false
		}
	case CSBKChannelTimingPDU:
		csbk.ChannelTimingPDU = &ChannelTimingPDU{}
		if !csbk.ChannelTimingPDU.DecodeFromBits(pdu) {
			return false
		}
	default:
		fmt.Printf("CSBK: unknown opcode: %08b\n", byte(csbk.CSBKOpcode))
		return false
	}

	return true
}
