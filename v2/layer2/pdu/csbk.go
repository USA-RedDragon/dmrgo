package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	layer3Elements "github.com/USA-RedDragon/dmrgo/v2/layer3/elements"
)

type CSBKOpcode byte

const (
	// Tier II opcodes (ETSI TS 102 361-1 / TS 102 361-2)
	CSBKUnitToUnitVoiceServiceRequestPDU        CSBKOpcode = 0b00000100
	CSBKUnitToUnitVoiceServiceAnswerResponsePDU CSBKOpcode = 0b00000101
	CSBKChannelTimingPDU                        CSBKOpcode = 0b00000111
	CSBKNegativeAcknowledgementPDU              CSBKOpcode = 0b00100110
	CSBKBSOutboundActivationPDU                 CSBKOpcode = 0b00111000
	CSBKPreamblePDU                             CSBKOpcode = 0b00111101

	// Tier III opcodes (ETSI TS 102 361-4 — Table B.1)
	CSBKAloha                        CSBKOpcode = 0b00011001
	CSBKUDTOutboundHeader            CSBKOpcode = 0b00011010
	CSBKUDTInboundHeader             CSBKOpcode = 0b00011011
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
	// Note: CSBKTalkgroupDataGrantMultiItem shares opcode 0x38 with CSBKBSOutboundActivationPDU.
	// Disambiguation is via the TrunkingMode flag on the CSBK struct.
	CSBKMove CSBKOpcode = 0b00111001
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
	case CSBKAloha:
		return "C_ALOHA PDU"
	case CSBKUDTOutboundHeader:
		return "C_UDTHD PDU"
	case CSBKUDTInboundHeader:
		return "C_UDTHU PDU"
	case CSBKAhoy:
		return "C_AHOY PDU"
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
	Reserved      uint16      `dmr:"bits:0-15"`
	BSAddress     [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-1 - 9.3.2 UU_V_Req PDU
type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	Reserved       byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-1 - 9.3.3 UU_Ans_Rsp PDU
type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions byte        `dmr:"bits:0-7"`
	AnswerResponse byte        `dmr:"bits:8-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-1 - 9.3.5 NACK_Rsp PDU
type NegativeAcknowledgementPDU struct {
	AdditionalInfo layer3Elements.AdditionalInformationField `dmr:"bits:0-0,delegate,noptr"`
	SourceType     layer3Elements.SourceType                 `dmr:"bits:1-1,delegate,noptr"`
	ServiceType    CSBKOpcode                                `dmr:"bits:2-7"`
	ReasonCode     byte                                      `dmr:"bits:8-15"`
	SourceAddress  [24]bit.Bit                               `dmr:"bits:16-39,raw"`
	TargetAddress  [24]bit.Bit                               `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-1 - 9.3.7 Pre PDU
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

// ETSI TS 102 361-1 - 9.3.8 Ch_Timing (Channel Timing) PDU
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

// ── Tier III CSBK sub-PDU structs (ETSI TS 102 361-4 §7.1.1) ──

// ETSI TS 102 361-4 §7.1.1.1.1 PV_GRANT PDU
type PrivateVoiceGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	Reserved        bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TV_GRANT PDU
type TalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	LateEntry       bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 BTV_GRANT PDU
type BroadcastTalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	LateEntry       bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT PDU
type PrivateDataGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	HiRate          bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TD_GRANT PDU
type TalkgroupDataGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	HiRate          bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PV_GRANT_DX PDU
type DuplexPrivateVoiceGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	Reserved        bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	CallDirection   bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT_DX PDU
type DuplexPrivateDataGrantPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	HiRate          bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	CallDirection   bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 PD_GRANT_MI PDU
type PrivateDataGrantMultiItemPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	HiRate          bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.1 TD_GRANT_MI PDU
// NOTE: Shares opcode 0x38 with BSOutboundActivationPDU; disambiguated via TrunkingMode.
type TalkgroupDataGrantMultiItemPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	LogicalChannel  bool        `dmr:"bit:12"`
	HiRate          bool        `dmr:"bit:13"`
	Emergency       bool        `dmr:"bit:14"`
	Offset          bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.2 C_MOVE PDU
type MovePDU struct {
	Reserved1       [9]bit.Bit  `dmr:"bits:0-8,raw"`
	Mask            uint8       `dmr:"bits:9-13"`
	Reserved2       [5]bit.Bit  `dmr:"bits:14-18,raw"`
	Reg             bool        `dmr:"bit:19"`
	Backoff         uint8       `dmr:"bits:20-23"`
	Reserved3       [4]bit.Bit  `dmr:"bits:24-27,raw"`
	PhysicalChannel uint16      `dmr:"bits:28-39"`
	MSAddress       [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.3 C_ALOHA PDU
type AlohaPDU struct {
	Reserved1    bool        `dmr:"bit:0"`
	TSCCAS       bool        `dmr:"bit:1"`
	SiteTSSync   bool        `dmr:"bit:2"`
	Version      uint8       `dmr:"bits:3-5"`
	Offset       bool        `dmr:"bit:6"`
	ActiveConn   bool        `dmr:"bit:7"`
	Mask         uint8       `dmr:"bits:8-12"`
	ServiceFunc  uint8       `dmr:"bits:13-14"`
	NRandWait    uint8       `dmr:"bits:15-18"`
	Reg          bool        `dmr:"bit:19"`
	Backoff      uint8       `dmr:"bits:20-23"`
	SysIdentCode uint16      `dmr:"bits:24-39"`
	MSAddress    [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.4 C_BCAST PDU (two-level dispatch on AnnouncementType)
type CBroadcastPDU struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"`
	BroadcastParms1  [14]bit.Bit            `dmr:"bits:5-18,raw"`
	Reg              bool                   `dmr:"bit:19"`
	Backoff          uint8                  `dmr:"bits:20-23"`
	SysIdentCode     uint16                 `dmr:"bits:24-39"`
	BroadcastParms2  [24]bit.Bit            `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.5 P_CLEAR PDU
type ClearPDU struct {
	PhysicalChannel uint16      `dmr:"bits:0-11"`
	Reserved1       bool        `dmr:"bit:12"`
	Reserved2       uint8       `dmr:"bits:13-14"`
	GroupIndividual bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.5 P_PROTECT PDU
type ProtectPDU struct {
	Reserved        [12]bit.Bit `dmr:"bits:0-11,raw"`
	ProtectKind     uint8       `dmr:"bits:12-14"`
	GroupIndividual bool        `dmr:"bit:15"`
	TargetAddress   [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress   [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.6 C_AHOY / P_AHOY PDU
type AhoyPDU struct {
	ServiceOptsMirror uint8       `dmr:"bits:0-6"`
	ServiceKindFlag   bool        `dmr:"bit:7"`
	ALS               bool        `dmr:"bit:8"`
	GroupIndividual   bool        `dmr:"bit:9"`
	AppendedBlocks    uint8       `dmr:"bits:10-11"`
	ServiceKind       uint8       `dmr:"bits:12-15"`
	TargetAddress     [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress     [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 C_ACKD PDU
type AckOutboundPDU struct {
	ResponseInfo   uint8       `dmr:"bits:0-6"`
	ReasonCode     byte        `dmr:"bits:7-14"`
	Reserved       bool        `dmr:"bit:15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	AdditionalInfo [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 C_ACKU PDU
type AckInboundPDU struct {
	ResponseInfo   uint8       `dmr:"bits:0-6"`
	ReasonCode     byte        `dmr:"bits:7-14"`
	Reserved       bool        `dmr:"bit:15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	AdditionalInfo [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 P_ACKD PDU
type AckOutboundPayloadPDU struct {
	ResponseInfo   uint8       `dmr:"bits:0-6"`
	ReasonCode     byte        `dmr:"bits:7-14"`
	Reserved       bool        `dmr:"bit:15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	AdditionalInfo [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.7 P_ACKU PDU
type AckInboundPayloadPDU struct {
	ResponseInfo   uint8       `dmr:"bits:0-6"`
	ReasonCode     byte        `dmr:"bits:7-14"`
	Reserved       bool        `dmr:"bit:15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	AdditionalInfo [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.8 C_UDTHD PDU
type UDTOutboundHeaderPDU struct {
	GroupIndividual  bool        `dmr:"bit:0"`
	A                bool        `dmr:"bit:1"`
	Emergency        bool        `dmr:"bit:2"`
	UDTOptionFlag    bool        `dmr:"bit:3"`
	DataPacketFormat uint8       `dmr:"bits:4-7"`
	SAP              uint8       `dmr:"bits:8-11"`
	UDTFormat        uint8       `dmr:"bits:12-15"`
	TargetAddress    [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress    [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.8 C_UDTHU PDU
type UDTInboundHeaderPDU struct {
	GroupIndividual  bool        `dmr:"bit:0"`
	A                bool        `dmr:"bit:1"`
	Emergency        bool        `dmr:"bit:2"`
	UDTOptionFlag    bool        `dmr:"bit:3"`
	DataPacketFormat uint8       `dmr:"bits:4-7"`
	SAP              uint8       `dmr:"bits:8-11"`
	UDTFormat        uint8       `dmr:"bits:12-15"`
	TargetAddress    [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress    [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.9 C_RAND PDU
type RandomAccessPDU struct {
	ServiceOptions uint8       `dmr:"bits:0-6"`
	ProxyFlag      bool        `dmr:"bit:7"`
	Reserved       uint8       `dmr:"bits:8-11"`
	ServiceKind    uint8       `dmr:"bits:12-15"`
	TargetAddress  [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress  [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.10 C_ACKVIT PDU
type AckvitationPDU struct {
	ServiceOptsMirror uint8       `dmr:"bits:0-6"`
	ServiceKindFlag   bool        `dmr:"bit:7"`
	Reserved          uint8       `dmr:"bits:8-9"`
	UAB               uint8       `dmr:"bits:10-11"`
	ServiceKind       uint8       `dmr:"bits:12-15"`
	TargetAddress     [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress     [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// ETSI TS 102 361-4 §7.1.1.1.11 P_MAINT PDU
type MaintenancePDU struct {
	Reserved      [12]bit.Bit `dmr:"bits:0-11,raw"`
	MaintKind     uint8       `dmr:"bits:12-14"`
	Reserved2     bool        `dmr:"bit:15"`
	TargetAddress [24]bit.Bit `dmr:"bits:16-39,raw"`
	SourceAddress [24]bit.Bit `dmr:"bits:40-63,raw"`
}

// SetTrunkingMode sets the trunking mode flag, affecting opcode 0x38 dispatch.
func (csbk *CSBK) SetTrunkingMode(mode bool) {
	csbk.TrunkingMode = mode
}

// dmr:crc crc_ccitt
// dmr:crc_mask 0xA5A5
// dmr:input_size 96
// ETSI TS 102 361-1 - 9.1.5 CSBK PDU
type CSBK struct {
	DataType elements.DataType `dmr:"-"`

	LastBlock    bool          `dmr:"bit:0"`
	ProtectFlag  bool          `dmr:"bit:1"`
	CSBKOpcode   CSBKOpcode    `dmr:"bits:2-7"`
	FID          byte          `dmr:"bits:8-15"`
	FEC          fec.FECResult `dmr:"-"`
	TrunkingMode bool          `dmr:"-"`

	// Tier II CSBK PDUs (ETSI TS 102 361-1 / TS 102 361-2)
	BSOutboundActivationPDU                 *BSOutboundActivationPDU                 `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKBSOutboundActivationPDU,when:TrunkingMode==false"`
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
	TalkgroupDataGrantMultiItemPDU  *TalkgroupDataGrantMultiItemPDU  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKBSOutboundActivationPDU,when:TrunkingMode==true"`
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
	UDTOutboundHeaderPDU            *UDTOutboundHeaderPDU            `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUDTOutboundHeader"`
	UDTInboundHeaderPDU             *UDTInboundHeaderPDU             `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKUDTInboundHeader"`
	RandomAccessPDU                 *RandomAccessPDU                 `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKRandomAccess"`
	AckvitationPDU                  *AckvitationPDU                  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKAckvitation"`
	MaintenancePDU                  *MaintenancePDU                  `dmr:"bits:16-79,dispatch:CSBKOpcode=CSBKMaintenance"`

	crc uint16 `dmr:"-"` //nolint:unused
}

func (csbk *CSBK) GetDataType() elements.DataType {
	return csbk.DataType
}
