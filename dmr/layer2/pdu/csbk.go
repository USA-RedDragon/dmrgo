package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
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

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.8 Ch_Timing (Channel Timing) PDU
type ChannelTimingPDU struct {
	SyncAge                 [11]bit.Bit `dmr:"bits:0-10,raw"`
	Generation              [5]bit.Bit  `dmr:"bits:11-15,raw"`
	LeaderIdentifier        [20]bit.Bit `dmr:"bits:16-35,raw"`
	NewLeader               bool        `dmr:"bit:36"`
	LeaderDynamicIdentifier [2]bit.Bit  `dmr:"bits:37-38,raw"`
	ChannelTimingOp0        bool        `dmr:"bit:39"`
	SourceIdentifier        [20]bit.Bit `dmr:"bits:40-59,raw"`
	Reserved                bool        `dmr:"bit:60"`
	SourceDynamicIdentifier [2]bit.Bit  `dmr:"bits:61-62,raw"`
	ChannelTimingOp1        bool        `dmr:"bit:63"`
}

func (pdu ChannelTimingPDU) ToString() string {
	return fmt.Sprintf("ChannelTimingPDU{ SyncAge: %011b, Generation: %05b, LeaderIdentifier: %s, NewLeader: %t, LeaderDynamicIdentifier: %02b, SourceIdentifier: %s, Reserved: %t, SourceDynamicIdentifier: %02b, ChannelTimingOp: [%t %t] }", pdu.SyncAge, pdu.Generation, string(pdu.LeaderIdentifier[:]), pdu.NewLeader, pdu.LeaderDynamicIdentifier, string(pdu.SourceIdentifier[:]), pdu.Reserved, pdu.SourceDynamicIdentifier, pdu.ChannelTimingOp0, pdu.ChannelTimingOp1)
}

// dmr:crc crc_ccitt
// dmr:crc_mask 0xA5A5
// dmr:input_size 96
// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.5 CSBK PDU
type CSBK struct {
	DataType elements.DataType `dmr:"-"`

	LastBlock   bool          `dmr:"bit:0"`
	ProtectFlag bool          `dmr:"bit:1"`
	CSBKOpcode  CSBKOpcode    `dmr:"bits:2-7"`
	FID         byte          `dmr:"bits:8-15"`
	FEC         fec.FECResult `dmr:"-"`

	BSOutboundActivationPDU                 *BSOutboundActivationPDU                 `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKBSOutboundActivationPDU"`
	UnitToUnitVoiceServiceRequestPDU        *UnitToUnitVoiceServiceRequestPDU        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUnitToUnitVoiceServiceRequestPDU"`
	UnitToUnitVoiceServiceAnswerResponsePDU *UnitToUnitVoiceServiceAnswerResponsePDU `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUnitToUnitVoiceServiceAnswerResponsePDU"`
	NegativeAcknowledgementPDU              *NegativeAcknowledgementPDU              `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKNegativeAcknowledgementPDU"`
	PreamblePDU                             *PreamblePDU                             `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKPreamblePDU"`
	ChannelTimingPDU                        *ChannelTimingPDU                        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKChannelTimingPDU"`

	crc uint16 `dmr:"-"` //nolint:unused
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
	return fmt.Sprintf("CSBK{ dataType: %s, LastBlock: %t, ProtectFlag: %t, CSBKOpcode: %v, FID: %d, crc: %04x, extraData: %s }", elements.DataTypeToName(csbk.DataType), csbk.LastBlock, csbk.ProtectFlag, csbk.CSBKOpcode, csbk.FID, csbk.crc, extraData)
}

func (csbk *CSBK) GetDataType() elements.DataType {
	return csbk.DataType
}
