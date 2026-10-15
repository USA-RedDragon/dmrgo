package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

type CSBKOpcode byte

const (
	// Tier II opcodes (ETSI TS 102 361-1 / TS 102 361-2)
	CSBKUnitToUnitVoiceServiceRequestPDU        CSBKOpcode = 0b00000100
	CSBKUnitToUnitVoiceServiceAnswerResponsePDU CSBKOpcode = 0b00000101
	CSBKChannelTimingPDU                        CSBKOpcode = 0b00000111
	CSBKNegativeAcknowledgementPDU              CSBKOpcode = 0b00100110
	CSBKBSOutboundActivationPDU                 CSBKOpcode = 0b00111000
	// CSBKTalkgroupDataGrantMultiItem is the Tier III TD_GRANT_MI opcode
	// (ETSI TS 102 361-4 Table B.1). It is the same value as
	// CSBKBSOutboundActivationPDU; see CSBK.AsTalkgroupDataGrantMultiItem.
	CSBKTalkgroupDataGrantMultiItem CSBKOpcode = 0b00111000
	CSBKPreamblePDU                 CSBKOpcode = 0b00111101

	// Tier III opcodes (ETSI TS 102 361-4 — Table B.1). The "UDT Header"
	// opcodes (C_UDTHD, C_UDTHU, ...) are not CSBKs: they travel in a Data
	// Header burst (Table 7.2), see UDTHeader and UDTOpcode.
	CSBKAloha                        CSBKOpcode = 0b00011001
	CSBKAhoy                         CSBKOpcode = 0b00011100
	CSBKAckvitation                  CSBKOpcode = 0b00011110
	CSBKRandomAccess                 CSBKOpcode = 0b00011111
	CSBKAckOutbound                  CSBKOpcode = 0b00100000
	CSBKAckInbound                   CSBKOpcode = 0b00100001
	CSBKAckOutboundPayload           CSBKOpcode = 0b00100010
	CSBKAckInboundPayload            CSBKOpcode = 0b00100011
	CSBKBroadcast                    CSBKOpcode = 0b00101000
	CSBKMaintenance                  CSBKOpcode = 0b00101010
	CSBKClear                        CSBKOpcode = 0b00101110
	CSBKProtect                      CSBKOpcode = 0b00101111
	CSBKPrivateVoiceGrant            CSBKOpcode = 0b00110000
	CSBKTalkgroupVoiceGrant          CSBKOpcode = 0b00110001
	CSBKBroadcastTalkgroupVoiceGrant CSBKOpcode = 0b00110010
	CSBKPrivateDataGrant             CSBKOpcode = 0b00110011
	CSBKTalkgroupDataGrant           CSBKOpcode = 0b00110100
	CSBKDuplexPrivateVoiceGrant      CSBKOpcode = 0b00110101
	CSBKDuplexPrivateDataGrant       CSBKOpcode = 0b00110110
	CSBKPrivateDataGrantMultiItem    CSBKOpcode = 0b00110111
	CSBKMove                         CSBKOpcode = 0b00111001
)

func (opcode CSBKOpcode) String() string {
	switch opcode {
	case CSBKBSOutboundActivationPDU:
		return "BS Outbound Activation PDU or TD_GRANT_MI PDU"
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
	case CSBKAloha:
		return "C_ALOHA PDU"
	case CSBKAhoy:
		return "C_AHOY/P_AHOY PDU"
	case CSBKAckvitation:
		return "C_ACKVIT PDU"
	case CSBKRandomAccess:
		return "C_RAND PDU"
	case CSBKAckOutbound:
		return "C_ACKD PDU"
	case CSBKAckInbound:
		return "C_ACKU PDU"
	case CSBKAckOutboundPayload:
		return "P_ACKD PDU"
	case CSBKAckInboundPayload:
		return "P_ACKU PDU"
	case CSBKBroadcast:
		return "C_BCAST PDU"
	case CSBKMaintenance:
		return "P_MAINT PDU"
	case CSBKClear:
		return "P_CLEAR PDU"
	case CSBKProtect:
		return "P_PROTECT PDU"
	case CSBKPrivateVoiceGrant:
		return "PV_GRANT PDU"
	case CSBKTalkgroupVoiceGrant:
		return "TV_GRANT PDU"
	case CSBKBroadcastTalkgroupVoiceGrant:
		return "BTV_GRANT PDU"
	case CSBKPrivateDataGrant:
		return "PD_GRANT PDU"
	case CSBKTalkgroupDataGrant:
		return "TD_GRANT PDU"
	case CSBKDuplexPrivateVoiceGrant:
		return "PV_GRANT_DX PDU"
	case CSBKDuplexPrivateDataGrant:
		return "PD_GRANT_DX PDU"
	case CSBKPrivateDataGrantMultiItem:
		return "PD_GRANT_MI PDU"
	case CSBKMove:
		return "C_MOVE PDU"
	default:
		return fmt.Sprintf("Unknown CSBKOpcode: %08b", byte(opcode))
	}
}

// ETSI TS 102 361-1 - 9.3.6 BS Outbound Activation (BS_Dwn_Act) PDU
type BSOutboundActivationPDU struct {
	Reserved      uint16 `dmr:"bits:0-15"`
	BSAddress     uint32 `dmr:"bits:16-39"`
	SourceAddress uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-1 - 9.3.2 UU_V_Req PDU
type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"`
	Reserved       byte                  `dmr:"bits:8-15"`
	TargetAddress  uint32                `dmr:"bits:16-39"`
	SourceAddress  uint32                `dmr:"bits:40-63"`
}

// ETSI TS 102 361-1 - 9.3.3 UU_Ans_Rsp PDU
type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"`
	AnswerResponse byte                  `dmr:"bits:8-15"`
	TargetAddress  uint32                `dmr:"bits:16-39"`
	SourceAddress  uint32                `dmr:"bits:40-63"`
}

// ETSI TS 102 361-1 - 9.3.5 NACK_Rsp PDU
type NegativeAcknowledgementPDU struct {
	AdditionalInfo layer3.AdditionalInformationField `dmr:"bits:0-0,delegate,noptr"`
	SourceType     layer3.SourceType                 `dmr:"bits:1-1,delegate,noptr"`
	ServiceType    CSBKOpcode                        `dmr:"bits:2-7"`
	ReasonCode     byte                              `dmr:"bits:8-15"`
	SourceAddress  uint32                            `dmr:"bits:16-39"`
	TargetAddress  uint32                            `dmr:"bits:40-63"`
}

// ETSI TS 102 361-1 - 9.3.7 Pre PDU
type PreamblePDU struct {
	// 1 = data content follows, 0 = CSBK follows
	Data bool `dmr:"bit:0"`
	// 1 = target address is a group, 0 = individual
	Group              bool   `dmr:"bit:1"`
	Reserved           uint8  `dmr:"bits:2-7"`
	CSBKBlocksToFollow byte   `dmr:"bits:8-15"`
	TargetAddress      uint32 `dmr:"bits:16-39"`
	SourceAddress      uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-2 - 7.1.2.6 CT_CSBK (Channel Timing) PDU, Table 7.8.
// ChannelTimingOp1 is the opcode MSB and ChannelTimingOp0 the LSB.
type ChannelTimingPDU struct {
	SyncAge                 uint16 `dmr:"bits:0-10"`
	Generation              uint8  `dmr:"bits:11-15"`
	LeaderIdentifier        uint32 `dmr:"bits:16-35"`
	NewLeader               bool   `dmr:"bit:36"`
	LeaderDynamicIdentifier uint8  `dmr:"bits:37-38"`
	ChannelTimingOp1        bool   `dmr:"bit:39"`
	SourceIdentifier        uint32 `dmr:"bits:40-59"`
	Reserved                bool   `dmr:"bit:60"`
	SourceDynamicIdentifier uint8  `dmr:"bits:61-62"`
	ChannelTimingOp0        bool   `dmr:"bit:63"`
}

// ── Tier III CSBK sub-PDU structs (ETSI TS 102 361-4 §7.1.1) ──

// ETSI TS 102 361-4 §7.1.1.1.1 PV_GRANT PDU
type PrivateVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	Reserved        bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TV_GRANT PDU
type TalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	LateEntry       bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 BTV_GRANT PDU
type BroadcastTalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	LateEntry       bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT PDU
type PrivateDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	HiRate          bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TD_GRANT PDU
type TalkgroupDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	HiRate          bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PV_GRANT_DX PDU
type DuplexPrivateVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	Reserved        bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	CallDirection   bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT_DX PDU
type DuplexPrivateDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	HiRate          bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	CallDirection   bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT_MI PDU
type PrivateDataGrantMultiItemPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	HiRate          bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TD_GRANT_MI PDU
// NOTE: Shares opcode 0x38 with BSOutboundActivationPDU; disambiguated via the
// trunkingMode argument of DecodeCSBK.
type TalkgroupDataGrantMultiItemPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	LogicalChannel  bool   `dmr:"bit:12"`
	HiRate          bool   `dmr:"bit:13"`
	Emergency       bool   `dmr:"bit:14"`
	Offset          bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.2 C_MOVE PDU
type MovePDU struct {
	Reserved1       uint16 `dmr:"bits:0-8"`
	Mask            uint8  `dmr:"bits:9-13"`
	Reserved2       uint8  `dmr:"bits:14-18"`
	Reg             bool   `dmr:"bit:19"`
	Backoff         uint8  `dmr:"bits:20-23"`
	Reserved3       uint8  `dmr:"bits:24-27"`
	PhysicalChannel uint16 `dmr:"bits:28-39"`
	MSAddress       uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.3 C_ALOHA PDU
type AlohaPDU struct {
	Reserved1    bool   `dmr:"bit:0"`
	TSCCAS       bool   `dmr:"bit:1"`
	SiteTSSync   bool   `dmr:"bit:2"`
	Version      uint8  `dmr:"bits:3-5"`
	Offset       bool   `dmr:"bit:6"`
	ActiveConn   bool   `dmr:"bit:7"`
	Mask         uint8  `dmr:"bits:8-12"`
	ServiceFunc  uint8  `dmr:"bits:13-14"`
	NRandWait    uint8  `dmr:"bits:15-18"`
	Reg          bool   `dmr:"bit:19"`
	Backoff      uint8  `dmr:"bits:20-23"`
	SysIdentCode uint16 `dmr:"bits:24-39"`
	MSAddress    uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.5 C_BCAST PDU, Table 7.20
// BroadcastParms1 and BroadcastParms2 are not decoded further; their layout
// depends on AnnouncementType (TS 102 361-4 §7.2.19, Tables 7.69 onward).
type CBroadcastPDU struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"`
	BroadcastParms1  uint16                 `dmr:"bits:5-18"`
	Reg              bool                   `dmr:"bit:19"`
	Backoff          uint8                  `dmr:"bits:20-23"`
	SysIdentCode     uint16                 `dmr:"bits:24-39"`
	BroadcastParms2  uint32                 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.5 P_CLEAR PDU
type ClearPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`
	Reserved1       bool   `dmr:"bit:12"`
	Reserved2       uint8  `dmr:"bits:13-14"`
	GroupIndividual bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.5 P_PROTECT PDU
type ProtectPDU struct {
	Reserved        uint16 `dmr:"bits:0-11"`
	ProtectKind     uint8  `dmr:"bits:12-14"`
	GroupIndividual bool   `dmr:"bit:15"`
	TargetAddress   uint32 `dmr:"bits:16-39"`
	SourceAddress   uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.6 C_AHOY / P_AHOY PDU
type AhoyPDU struct {
	ServiceOptsMirror uint8  `dmr:"bits:0-6"`
	ServiceKindFlag   bool   `dmr:"bit:7"`
	ALS               bool   `dmr:"bit:8"`
	GroupIndividual   bool   `dmr:"bit:9"`
	AppendedBlocks    uint8  `dmr:"bits:10-11"`
	ServiceKind       uint8  `dmr:"bits:12-15"`
	TargetAddress     uint32 `dmr:"bits:16-39"`
	SourceAddress     uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 C_ACKD PDU
type AckOutboundPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`
	ReasonCode     byte   `dmr:"bits:7-14"`
	Reserved       bool   `dmr:"bit:15"`
	TargetAddress  uint32 `dmr:"bits:16-39"`
	AdditionalInfo uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 C_ACKU PDU
type AckInboundPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`
	ReasonCode     byte   `dmr:"bits:7-14"`
	Reserved       bool   `dmr:"bit:15"`
	TargetAddress  uint32 `dmr:"bits:16-39"`
	AdditionalInfo uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 P_ACKD PDU
type AckOutboundPayloadPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`
	ReasonCode     byte   `dmr:"bits:7-14"`
	Reserved       bool   `dmr:"bit:15"`
	TargetAddress  uint32 `dmr:"bits:16-39"`
	AdditionalInfo uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 P_ACKU PDU
type AckInboundPayloadPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`
	ReasonCode     byte   `dmr:"bits:7-14"`
	Reserved       bool   `dmr:"bit:15"`
	TargetAddress  uint32 `dmr:"bits:16-39"`
	AdditionalInfo uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.9 C_RAND PDU
type RandomAccessPDU struct {
	ServiceOptions uint8  `dmr:"bits:0-6"`
	ProxyFlag      bool   `dmr:"bit:7"`
	Reserved       uint8  `dmr:"bits:8-11"`
	ServiceKind    uint8  `dmr:"bits:12-15"`
	TargetAddress  uint32 `dmr:"bits:16-39"`
	SourceAddress  uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.10 C_ACKVIT PDU
type AckvitationPDU struct {
	ServiceOptsMirror uint8  `dmr:"bits:0-6"`
	ServiceKindFlag   bool   `dmr:"bit:7"`
	Reserved          uint8  `dmr:"bits:8-9"`
	UAB               uint8  `dmr:"bits:10-11"`
	ServiceKind       uint8  `dmr:"bits:12-15"`
	TargetAddress     uint32 `dmr:"bits:16-39"`
	SourceAddress     uint32 `dmr:"bits:40-63"`
}

// ETSI TS 102 361-4 §7.1.1.1.11 P_MAINT PDU
type MaintenancePDU struct {
	Reserved      uint16 `dmr:"bits:0-11"`
	MaintKind     uint8  `dmr:"bits:12-14"`
	Reserved2     bool   `dmr:"bit:15"`
	TargetAddress uint32 `dmr:"bits:16-39"`
	SourceAddress uint32 `dmr:"bits:40-63"`
}

// Opcode0x38Hint is the likely meaning of a CSBK with opcode 0x38, which is
// BS_Dwn_Act in Tier II and TD_GRANT_MI in Tier III.
type Opcode0x38Hint uint8

const (
	// Opcode0x38Unknown gives no hint.
	Opcode0x38Unknown Opcode0x38Hint = iota
	// Opcode0x38BSOutboundActivation hints at BS_Dwn_Act, which an MS sends
	// (ETSI TS 102 361-2 5.1.1.1).
	Opcode0x38BSOutboundActivation
	// Opcode0x38TalkgroupDataGrantMultiItem hints at TD_GRANT_MI, which a
	// TSCC sends (ETSI TS 102 361-4).
	Opcode0x38TalkgroupDataGrantMultiItem
)

// dmr:crc crc_ccitt
// dmr:crc_mask 0xA5A5
// dmr:input_size 96
// dmr:dispatch_if FID==0
// ETSI TS 102 361-1 - 9.1.5 CSBK PDU
//
// The FID redefines the CSBKO (ETSI TS 102 361-1 Table 9.21), so the standard
// payload layouts apply only to the SFID. Payloads of other FIDs and of
// opcodes without a layout are kept in RawPayload.
//
// Opcode 0x38 is BS_Dwn_Act in Tier II and TD_GRANT_MI in Tier III, and the
// CSBK does not say which, so its payload stays in RawPayload; see
// AsBSOutboundActivation and AsTalkgroupDataGrantMultiItem.
//
// CSBK.Encode returns an error if more than one payload field is set, or if
// CSBKOpcode and FID do not select the payload field that is set. Put the
// payload of a manufacturer FID or of opcode 0x38 in RawPayload.
type CSBK struct {
	LastBlock   bool               `dmr:"bit:0"`
	ProtectFlag bool               `dmr:"bit:1"`
	CSBKOpcode  CSBKOpcode         `dmr:"bits:2-7"`
	FID         enums.FeatureSetID `dmr:"bits:8-15"`
	FEC         fec.Result         `dmr:"-"`
	// Hint0x38 is the likely meaning of opcode 0x38. DecodeCSBK leaves it
	// Opcode0x38Unknown; layer2.Burst sets it from the SYNC pattern of the
	// burst. It is a hint only: a network relay can rewrite the SYNC.
	Hint0x38 Opcode0x38Hint `dmr:"-"`

	// Tier II CSBK PDUs (ETSI TS 102 361-1 / TS 102 361-2)
	UnitToUnitVoiceServiceRequestPDU        *UnitToUnitVoiceServiceRequestPDU        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUnitToUnitVoiceServiceRequestPDU"`
	UnitToUnitVoiceServiceAnswerResponsePDU *UnitToUnitVoiceServiceAnswerResponsePDU `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUnitToUnitVoiceServiceAnswerResponsePDU"`
	NegativeAcknowledgementPDU              *NegativeAcknowledgementPDU              `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKNegativeAcknowledgementPDU"`
	PreamblePDU                             *PreamblePDU                             `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKPreamblePDU"`
	ChannelTimingPDU                        *ChannelTimingPDU                        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKChannelTimingPDU"`

	// Tier III CSBK PDUs (ETSI TS 102 361-4)
	PrivateVoiceGrantPDU            *PrivateVoiceGrantPDU            `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKPrivateVoiceGrant"`
	TalkgroupVoiceGrantPDU          *TalkgroupVoiceGrantPDU          `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKTalkgroupVoiceGrant"`
	BroadcastTalkgroupVoiceGrantPDU *BroadcastTalkgroupVoiceGrantPDU `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKBroadcastTalkgroupVoiceGrant"`
	PrivateDataGrantPDU             *PrivateDataGrantPDU             `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKPrivateDataGrant"`
	TalkgroupDataGrantPDU           *TalkgroupDataGrantPDU           `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKTalkgroupDataGrant"`
	DuplexPrivateVoiceGrantPDU      *DuplexPrivateVoiceGrantPDU      `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKDuplexPrivateVoiceGrant"`
	DuplexPrivateDataGrantPDU       *DuplexPrivateDataGrantPDU       `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKDuplexPrivateDataGrant"`
	PrivateDataGrantMultiItemPDU    *PrivateDataGrantMultiItemPDU    `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKPrivateDataGrantMultiItem"`
	MovePDU                         *MovePDU                         `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKMove"`
	AlohaPDU                        *AlohaPDU                        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAloha"`
	CBroadcastPDU                   *CBroadcastPDU                   `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKBroadcast"`
	ClearPDU                        *ClearPDU                        `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKClear"`
	ProtectPDU                      *ProtectPDU                      `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKProtect"`
	AhoyPDU                         *AhoyPDU                         `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAhoy"`
	AckOutboundPDU                  *AckOutboundPDU                  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckOutbound"`
	AckInboundPDU                   *AckInboundPDU                   `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckInbound"`
	AckOutboundPayloadPDU           *AckOutboundPayloadPDU           `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckOutboundPayload"`
	AckInboundPayloadPDU            *AckInboundPayloadPDU            `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckInboundPayload"`
	RandomAccessPDU                 *RandomAccessPDU                 `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKRandomAccess"`
	AckvitationPDU                  *AckvitationPDU                  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckvitation"`
	MaintenancePDU                  *MaintenancePDU                  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKMaintenance"`

	RawPayload [64]bit.Bit `dmr:"bits:16-79,raw,fallback"`

	crc uint16 `dmr:"-"` //nolint:unused
}

// AsBSOutboundActivation decodes the RawPayload of a standard FID CSBK with
// opcode 0x38 as a BS_Dwn_Act PDU. ok is false for another opcode or FID.
func (csbk *CSBK) AsBSOutboundActivation() (p BSOutboundActivationPDU, ok bool) {
	if csbk.CSBKOpcode != CSBKBSOutboundActivationPDU || csbk.FID != 0 {
		return p, false
	}
	p, _ = DecodeBSOutboundActivationPDU(csbk.RawPayload)
	return p, true
}

// AsTalkgroupDataGrantMultiItem decodes the RawPayload of a standard FID
// CSBK with opcode 0x38 as a TD_GRANT_MI PDU. ok is false for another opcode
// or FID.
func (csbk *CSBK) AsTalkgroupDataGrantMultiItem() (p TalkgroupDataGrantMultiItemPDU, ok bool) {
	if csbk.CSBKOpcode != CSBKTalkgroupDataGrantMultiItem || csbk.FID != 0 {
		return p, false
	}
	p, _ = DecodeTalkgroupDataGrantMultiItemPDU(csbk.RawPayload)
	return p, true
}

// DataType returns enums.DataTypeCSBK.
func (csbk *CSBK) DataType() enums.DataType {
	return enums.DataTypeCSBK
}
