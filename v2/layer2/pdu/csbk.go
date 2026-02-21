package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
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

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.2 UU_V_Req PDU
type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	Reserved       byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.3.3 UU_Ans_Rsp PDU
type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	AnswerResponse byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
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

func (csbk *CSBK) GetDataType() elements.DataType {
	return csbk.DataType
}
