package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/crc"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
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

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.6 BS Outbound Activation (BS_Dwn_Act) PDU
type BSOutboundActivationPDU struct {
	Reserved      uint16      `dmr:"bits:0-15"`
	BSAddress     [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress [24]bit.Bit `dmr:"bits:40-63,raw"`
}

func (pdu BSOutboundActivationPDU) ToString() string {
	return fmt.Sprintf("BSOutboundActivationPDU{ Reserved: %d, BSAddress: %s, SourceAddress: %s }", pdu.Reserved, string(pdu.BSAddress[:]), string(pdu.SourceAddress[:]))
}

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.2 UU_V_Req PDU
type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	Reserved       byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

func (pdu UnitToUnitVoiceServiceRequestPDU) ToString() string {
	return fmt.Sprintf("UnitToUnitVoiceServiceRequestPDU{ ServiceOptions: %d, Reserved: %d, TargetAddress: %s, SourceAddress: %s }", pdu.ServiceOptions, pdu.Reserved, string(pdu.TargetAddress[:]), string(pdu.SourceAddress[:]))
}

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.3 UU_Ans_Rsp PDU
type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	AnswerResponse byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

func (pdu UnitToUnitVoiceServiceAnswerResponsePDU) ToString() string {
	return fmt.Sprintf("UnitToUnitVoiceServiceAnswerResponsePDU{ ServiceOptions: %d, AnswerResponse: %d, TargetAddress: %s, SourceAddress: %s }", pdu.ServiceOptions, pdu.AnswerResponse, string(pdu.TargetAddress[:]), string(pdu.SourceAddress[:]))
}

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.5 NACK_Rsp PDU
type NegativeAcknowledgementPDU struct {
	AdditionalInfo bool        `dmr:"bit:0"`
	SourceType     bool        `dmr:"bit:1"`
	ServiceType    [6]bit.Bit  `dmr:"bits:2-7,raw"`
	ReasonCode     byte        `dmr:"bits:8-15"`
	SourceAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	TargetAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

func (pdu NegativeAcknowledgementPDU) ToString() string {
	return fmt.Sprintf("NegativeAcknowledgementPDU{ AdditionalInfo: %t, SourceType: %t, ServiceType: %08b, ReasonCode: %d, SourceAddress: %s, TargetAddress: %s }", pdu.AdditionalInfo, pdu.SourceType, pdu.ServiceType, pdu.ReasonCode, string(pdu.SourceAddress[:]), string(pdu.TargetAddress[:]))
}

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.7 Pre PDU
type PreamblePDU struct {
	// 1 = data content follows, 0 = CSBK follows
	Data bool `dmr:"bit:0"`
	// 1 = target address is a group, 0 = individual
	Group              bool        `dmr:"bit:1"`
	Reserved           [6]bit.Bit  `dmr:"bits:2-7,raw"`
	CSBKBlocksToFollow byte        `dmr:"bits:8-15"`
	TargetAddress      [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress      [24]bit.Bit `dmr:"bits:40-63,raw"`
}

func (pdu PreamblePDU) ToString() string {
	return fmt.Sprintf("PreamblePDU{ Data: %t, Group: %t, Reserved: %08b, CSBKBlocksToFollow: %d, TargetAddress: %v, SourceAddress: %v }", pdu.Data, pdu.Group, pdu.Reserved, pdu.CSBKBlocksToFollow, pdu.TargetAddress[:], pdu.SourceAddress[:])
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
	FEC         fec.FECResult

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
	if !crc.CheckCRCCCITT(dataBytes[:]) {
		fmt.Println("CSBK: CRC check failed")
		csbk.FEC = fec.FECResult{BitsChecked: 96, Uncorrectable: true}
		return false
	}

	csbk.FEC = fec.FECResult{BitsChecked: 96}

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

	var pduBits [64]bit.Bit
	for i := range 64 {
		pduBits[i] = infoBits[16+i]
	}
	switch csbk.CSBKOpcode {
	case CSBKBSOutboundActivationPDU:
		decoded, _ := DecodeBSOutboundActivationPDU(pduBits)
		csbk.BSOutboundActivationPDU = &decoded
	case CSBKUnitToUnitVoiceServiceRequestPDU:
		decoded, _ := DecodeUnitToUnitVoiceServiceRequestPDU(pduBits)
		csbk.UnitToUnitVoiceServiceRequestPDU = &decoded
	case CSBKUnitToUnitVoiceServiceAnswerResponsePDU:
		decoded, _ := DecodeUnitToUnitVoiceServiceAnswerResponsePDU(pduBits)
		csbk.UnitToUnitVoiceServiceAnswerResponsePDU = &decoded
	case CSBKNegativeAcknowledgementPDU:
		decoded, _ := DecodeNegativeAcknowledgementPDU(pduBits)
		csbk.NegativeAcknowledgementPDU = &decoded
	case CSBKPreamblePDU:
		decoded, _ := DecodePreamblePDU(pduBits)
		csbk.PreamblePDU = &decoded
	case CSBKChannelTimingPDU:
		csbk.ChannelTimingPDU = &ChannelTimingPDU{}
		if !csbk.ChannelTimingPDU.DecodeFromBits(pduBits) {
			return false
		}
	default:
		fmt.Printf("CSBK: unknown opcode: %08b\n", byte(csbk.CSBKOpcode))
		return false
	}

	return true
}
