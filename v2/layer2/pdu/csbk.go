package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// CSBKOpcode is the 6-bit CSBK opcode (CSBKO) of a CSBK or MBC header
// (ETSI TS 102 361-4 Table B.1). The UDT header opcodes, such as C_UDTHD,
// are UDTOpcode values: they are sent in a data header burst (UDTHeader).
type CSBKOpcode byte

// The CSBK opcodes of Tier II (ETSI TS 102 361-2 clause 7.1.2) and Tier III
// (ETSI TS 102 361-4 Table B.1).
const (
	CSBKUnitToUnitVoiceServiceRequest        CSBKOpcode = 0b00000100 // UU_V_Req
	CSBKUnitToUnitVoiceServiceAnswerResponse CSBKOpcode = 0b00000101 // UU_Ans_Rsp
	CSBKChannelTiming                        CSBKOpcode = 0b00000111 // CT_CSBK
	CSBKNegativeAcknowledgement              CSBKOpcode = 0b00100110 // NACK_Rsp
	CSBKBSOutboundActivation                 CSBKOpcode = 0b00111000 // BS_Dwn_Act, same value as CSBKTalkgroupDataGrantMultiItem
	CSBKTalkgroupDataGrantMultiItem          CSBKOpcode = 0b00111000 // TD_GRANT_MI, same value as CSBKBSOutboundActivation
	CSBKPreamble                             CSBKOpcode = 0b00111101 // Pre_CSBK
	CSBKAloha                                CSBKOpcode = 0b00011001 // C_ALOHA
	CSBKAhoy                                 CSBKOpcode = 0b00011100 // C_AHOY and P_AHOY
	CSBKAckvitation                          CSBKOpcode = 0b00011110 // C_ACKVIT
	CSBKRandomAccess                         CSBKOpcode = 0b00011111 // C_RAND
	CSBKAckOutbound                          CSBKOpcode = 0b00100000 // C_ACKD
	CSBKAckInbound                           CSBKOpcode = 0b00100001 // C_ACKU
	CSBKAckOutboundPayload                   CSBKOpcode = 0b00100010 // P_ACKD
	CSBKAckInboundPayload                    CSBKOpcode = 0b00100011 // P_ACKU
	CSBKBroadcast                            CSBKOpcode = 0b00101000 // C_BCAST
	CSBKMaintenance                          CSBKOpcode = 0b00101010 // P_MAINT
	CSBKClear                                CSBKOpcode = 0b00101110 // P_CLEAR
	CSBKProtect                              CSBKOpcode = 0b00101111 // P_PROTECT
	CSBKPrivateVoiceGrant                    CSBKOpcode = 0b00110000 // PV_GRANT
	CSBKTalkgroupVoiceGrant                  CSBKOpcode = 0b00110001 // TV_GRANT
	CSBKBroadcastTalkgroupVoiceGrant         CSBKOpcode = 0b00110010 // BTV_GRANT
	CSBKPrivateDataGrant                     CSBKOpcode = 0b00110011 // PD_GRANT
	CSBKTalkgroupDataGrant                   CSBKOpcode = 0b00110100 // TD_GRANT
	CSBKDuplexPrivateVoiceGrant              CSBKOpcode = 0b00110101 // PV_GRANT_DX
	CSBKDuplexPrivateDataGrant               CSBKOpcode = 0b00110110 // PD_GRANT_DX
	CSBKPrivateDataGrantMultiItem            CSBKOpcode = 0b00110111 // PD_GRANT_MI
	CSBKMove                                 CSBKOpcode = 0b00111001 // C_MOVE
)

// String returns the name of the CSBKOpcode. It allocates only for a value
// without a name.
func (opcode CSBKOpcode) String() string {
	switch opcode {
	case CSBKBSOutboundActivation:
		return "BS Outbound Activation PDU or TD_GRANT_MI PDU"
	case CSBKUnitToUnitVoiceServiceRequest:
		return "Unit-to-Unit Voice Service Request PDU"
	case CSBKUnitToUnitVoiceServiceAnswerResponse:
		return "Unit-to-Unit Voice Service Answer/Response PDU"
	case CSBKNegativeAcknowledgement:
		return "Negative Acknowledgement PDU"
	case CSBKPreamble:
		return "Preamble PDU"
	case CSBKChannelTiming:
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

// BSOutboundActivationPDU is the BS outbound activation (BS_Dwn_Act) CSBK
// payload (ETSI TS 102 361-2 clause 7.1.2.1).
//
//dmr:subpdu
type BSOutboundActivationPDU struct {
	Reserved      uint16 `dmr:"bits:0-15"`  // reserved, 0
	BSAddress     uint32 `dmr:"bits:16-39"` // 24-bit address of the BS to activate
	SourceAddress uint32 `dmr:"bits:40-63"` // 24-bit address of the sending MS
}

// UnitToUnitVoiceServiceRequestPDU is the unit to unit voice service request
// (UU_V_Req) CSBK payload (ETSI TS 102 361-2 clause 7.1.2.2).
type UnitToUnitVoiceServiceRequestPDU struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // service options of the call
	Reserved       byte                  `dmr:"bits:8-15"`         // reserved, 0
	TargetAddress  uint32                `dmr:"bits:16-39"`        // 24-bit target address
	SourceAddress  uint32                `dmr:"bits:40-63"`        // 24-bit source address
}

// UnitToUnitVoiceServiceAnswerResponsePDU is the unit to unit voice service
// answer response (UU_Ans_Rsp) CSBK payload (ETSI TS 102 361-2 clause 7.1.2.3).
type UnitToUnitVoiceServiceAnswerResponsePDU struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // service options of the call
	AnswerResponse byte                  `dmr:"bits:8-15"`         // 0x20 for proceed, 0x21 for deny
	TargetAddress  uint32                `dmr:"bits:16-39"`        // 24-bit target address
	SourceAddress  uint32                `dmr:"bits:40-63"`        // 24-bit source address
}

// NegativeAcknowledgementPDU is the negative acknowledgement response
// (NACK_Rsp) CSBK payload (ETSI TS 102 361-2 clause 7.1.2.4).
type NegativeAcknowledgementPDU struct {
	AdditionalInfo layer3.AdditionalInformationField `dmr:"bits:0-0,delegate,noptr"` // tells if SourceAddress is valid
	SourceType     layer3.SourceType                 `dmr:"bits:1-1,delegate,noptr"` // BS or MS sourced
	ServiceType    CSBKOpcode                        `dmr:"bits:2-7"`                // opcode of the refused service
	ReasonCode     byte                              `dmr:"bits:8-15"`               // reason code, 0x21 when the MS does not support the service
	SourceAddress  uint32                            `dmr:"bits:16-39"`              // 24-bit source address
	TargetAddress  uint32                            `dmr:"bits:40-63"`              // 24-bit target address
}

// PreamblePDU is the preamble (Pre) CSBK payload (ETSI TS 102 361-2 clause
// 7.1.2.5).
type PreamblePDU struct {
	Data               bool   `dmr:"bit:0"`      // true when data follows the preambles, false when a CSBK follows
	Group              bool   `dmr:"bit:1"`      // true when TargetAddress is a talkgroup
	Reserved           uint8  `dmr:"bits:2-7"`   // reserved, 0
	CSBKBlocksToFollow byte   `dmr:"bits:8-15"`  // number of blocks after this one (CBF)
	TargetAddress      uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress      uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// ChannelTimingPDU is the channel timing (CT_CSBK) CSBK payload (ETSI TS 102
// 361-2 clause 7.1.2.6, Table 7.8).
type ChannelTimingPDU struct {
	SyncAge                 uint16 `dmr:"bits:0-10"`  // time since the last beacon in 500 ms steps
	Generation              uint8  `dmr:"bits:11-15"` // number of timing hops from the leader, 0 at the leader
	LeaderIdentifier        uint32 `dmr:"bits:16-35"` // 20-bit ID of the timing leader, 0 when unknown
	NewLeader               bool   `dmr:"bit:36"`     // true to appoint a new timing leader
	LeaderDynamicIdentifier uint8  `dmr:"bits:37-38"` // leader preference of the timing leader
	ChannelTimingOp1        bool   `dmr:"bit:39"`     // most significant bit of the channel timing opcode
	SourceIdentifier        uint32 `dmr:"bits:40-59"` // 20-bit ID of the sending MS
	Reserved                bool   `dmr:"bit:60"`     // reserved, 0
	SourceDynamicIdentifier uint8  `dmr:"bits:61-62"` // leader preference of the sending MS
	ChannelTimingOp0        bool   `dmr:"bit:63"`     // least significant bit of the channel timing opcode
}

// PrivateVoiceGrantPDU is the private voice channel grant (PV_GRANT) CSBK
// payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type PrivateVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	Reserved        bool   `dmr:"bit:13"`     // reserved, 0
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// TalkgroupVoiceGrantPDU is the talkgroup voice channel grant (TV_GRANT) CSBK
// payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type TalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	LateEntry       bool   `dmr:"bit:13"`     // true when sent after the call set-up (late entry)
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// BroadcastTalkgroupVoiceGrantPDU is the broadcast talkgroup voice channel
// grant (BTV_GRANT) CSBK payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type BroadcastTalkgroupVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	LateEntry       bool   `dmr:"bit:13"`     // true when sent after the call set-up (late entry)
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// PrivateDataGrantPDU is the private data channel grant (PD_GRANT) CSBK payload
// (ETSI TS 102 361-4 clause 7.1.1.1.1).
type PrivateDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	HiRate          bool   `dmr:"bit:13"`     // true for dual slot data, false for single slot data
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// TalkgroupDataGrantPDU is the talkgroup data channel grant (TD_GRANT) CSBK
// payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type TalkgroupDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	HiRate          bool   `dmr:"bit:13"`     // true for dual slot data, false for single slot data
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// DuplexPrivateVoiceGrantPDU is the duplex private voice channel grant
// (PV_GRANT_DX) CSBK payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type DuplexPrivateVoiceGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	Reserved        bool   `dmr:"bit:13"`     // reserved, 0
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	CallDirection   bool   `dmr:"bit:15"`     // true when TargetAddress is the calling MS, false when it is the called MS
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// DuplexPrivateDataGrantPDU is the duplex private data channel grant
// (PD_GRANT_DX) CSBK payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type DuplexPrivateDataGrantPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	HiRate          bool   `dmr:"bit:13"`     // true for dual slot data, false for single slot data
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	CallDirection   bool   `dmr:"bit:15"`     // true when TargetAddress is the calling MS, false when it is the called MS
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// PrivateDataGrantMultiItemPDU is the multi-item private data channel grant
// (PD_GRANT_MI) CSBK payload (ETSI TS 102 361-4 clause 7.1.1.1.1).
type PrivateDataGrantMultiItemPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	HiRate          bool   `dmr:"bit:13"`     // true for dual slot data, false for single slot data
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// TalkgroupDataGrantMultiItemPDU is the multi-item talkgroup data channel grant
// (TD_GRANT_MI) CSBK payload (ETSI TS 102 361-4 clause 7.1.1.1.1). DecodeCSBK
// keeps it in RawPayload (opcode 0x38): use CSBK.AsTalkgroupDataGrantMultiItem.
//
//dmr:subpdu
type TalkgroupDataGrantMultiItemPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // 12-bit channel number, 0xFFF when a CGAP block follows
	LogicalChannel  bool   `dmr:"bit:12"`     // TDMA channel: false for channel 1, true for channel 2
	HiRate          bool   `dmr:"bit:13"`     // true for dual slot data, false for single slot data
	Emergency       bool   `dmr:"bit:14"`     // true for an emergency call
	Offset          bool   `dmr:"bit:15"`     // true for offset timing, false for aligned timing
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// MovePDU is the move (C_MOVE) CSBK payload (ETSI TS 102 361-4 clause
// 7.1.1.1.3).
type MovePDU struct {
	Reserved1       uint16 `dmr:"bits:0-8"`   // reserved, 0
	Mask            uint8  `dmr:"bits:9-13"`  // address mask, 0 to 24
	Reserved2       uint8  `dmr:"bits:14-18"` // reserved, 0
	Reg             bool   `dmr:"bit:19"`     // true when MSs must register
	Backoff         uint8  `dmr:"bits:20-23"` // backoff number, 1 to 15 (0 is reserved)
	Reserved3       uint8  `dmr:"bits:24-27"` // reserved, 0
	PhysicalChannel uint16 `dmr:"bits:28-39"` // 12-bit TSCC channel number, 0xFFF when an MVAP block follows
	MSAddress       uint32 `dmr:"bits:40-63"` // 24-bit individual MS address
}

// AlohaPDU is the Aloha (C_ALOHA) CSBK payload (ETSI TS 102 361-4 clause
// 7.1.1.1.4).
type AlohaPDU struct {
	Reserved1    bool   `dmr:"bit:0"`      // reserved, 0
	TSCCAS       bool   `dmr:"bit:1"`      // true when the TSCC supports a TSCCAS
	SiteTSSync   bool   `dmr:"bit:2"`      // true when all channels of the site are timeslot synchronized
	Version      uint8  `dmr:"bits:3-5"`   // version of ETSI TS 102 361-4 that the system complies with
	Offset       bool   `dmr:"bit:6"`      // true when the TSCC uses offset timing
	ActiveConn   bool   `dmr:"bit:7"`      // true when the TS has a connection to the network
	Mask         uint8  `dmr:"bits:8-12"`  // address mask, 0 to 24
	ServiceFunc  uint8  `dmr:"bits:13-14"` // services for which random access is invited
	NRandWait    uint8  `dmr:"bits:15-18"` // code for the TDMA frames an MS waits before a random access retry
	Reg          bool   `dmr:"bit:19"`     // true when MSs must register
	Backoff      uint8  `dmr:"bits:20-23"` // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode uint16 `dmr:"bits:24-39"` // system identity code (C_SYScode) of the TSCC
	MSAddress    uint32 `dmr:"bits:40-63"` // 24-bit individual MS address
}

// BroadcastPDU is the announcement (C_BCAST) CSBK payload (ETSI TS 102 361-4
// clause 7.1.1.1.5, Table 7.20). The layout of BroadcastParms1 and
// BroadcastParms2 depends on AnnouncementType (ETSI TS 102 361-4 clause 7.2.19).
type BroadcastPDU struct {
	AnnouncementType enums.AnnouncementType `dmr:"bits:0-4,enum"` // which system parameters are broadcast
	BroadcastParms1  uint16                 `dmr:"bits:5-18"`     // 14 bits whose layout depends on AnnouncementType
	Reg              bool                   `dmr:"bit:19"`        // true when MSs must register
	Backoff          uint8                  `dmr:"bits:20-23"`    // backoff number, 1 to 15 (0 is reserved)
	SysIdentCode     uint16                 `dmr:"bits:24-39"`    // system identity code (C_SYScode) of the TSCC
	BroadcastParms2  uint32                 `dmr:"bits:40-63"`    // 24 bits whose layout depends on AnnouncementType
}

// ClearPDU is the clear (P_CLEAR) CSBK payload (ETSI TS 102 361-4 clause
// 7.1.1.3.2).
type ClearPDU struct {
	PhysicalChannel uint16 `dmr:"bits:0-11"`  // channel number to move to, 0 for the control channel
	Reserved1       bool   `dmr:"bit:12"`     // reserved, 0
	Reserved2       uint8  `dmr:"bits:13-14"` // reserved, 0
	GroupIndividual bool   `dmr:"bit:15"`     // true when TargetAddress is a talkgroup
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// ProtectPDU is the protect (P_PROTECT) CSBK payload (ETSI TS 102 361-4 clause
// 7.1.1.3.3).
type ProtectPDU struct {
	Reserved        uint16 `dmr:"bits:0-11"`  // reserved, 0
	ProtectKind     uint8  `dmr:"bits:12-14"` // 0 DIS_PTT, 1 EN_PTT, 2 ILLEGALLY_PARKED, 3 EN_PTT_ONE_MS
	GroupIndividual bool   `dmr:"bit:15"`     // true when TargetAddress is a talkgroup
	TargetAddress   uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress   uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// AhoyPDU is the ahoy (C_AHOY and P_AHOY) CSBK payload (ETSI TS 102 361-4
// clauses 7.1.1.1.6 and 7.1.1.3.4).
type AhoyPDU struct {
	ServiceOptsMirror uint8  `dmr:"bits:0-6"`   // Service_Options of the request that this answers
	ServiceKindFlag   bool   `dmr:"bit:7"`      // flag whose meaning depends on ServiceKind
	ALS               bool   `dmr:"bit:8"`      // true when the caller asked for ambient listening
	GroupIndividual   bool   `dmr:"bit:9"`      // true when TargetAddress is a talkgroup
	AppendedBlocks    uint8  `dmr:"bits:10-11"` // number of appended data blocks to send
	ServiceKind       uint8  `dmr:"bits:12-15"` // service that this PDU supports
	TargetAddress     uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress     uint32 `dmr:"bits:40-63"` // 24-bit source address, or an authentication challenge
}

// AckOutboundPDU is the outbound acknowledgement (C_ACKD) CSBK payload (ETSI TS
// 102 361-4 clause 7.1.1.1.7).
type AckOutboundPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`   // supplementary response information
	ReasonCode     byte   `dmr:"bits:7-14"`  // acknowledgement reason code
	Reserved       bool   `dmr:"bit:15"`     // reserved, 0
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target address
	AdditionalInfo uint32 `dmr:"bits:40-63"` // 24-bit additional information, usually a source address
}

// AckInboundPDU is the inbound acknowledgement (C_ACKU) CSBK payload (ETSI TS
// 102 361-4 clause 7.1.1.2.3).
type AckInboundPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`   // supplementary response information
	ReasonCode     byte   `dmr:"bits:7-14"`  // acknowledgement reason code
	Reserved       bool   `dmr:"bit:15"`     // reserved, 0
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target address
	AdditionalInfo uint32 `dmr:"bits:40-63"` // 24-bit additional information, usually a source address
}

// AckOutboundPayloadPDU is the outbound payload acknowledgement (P_ACKD) CSBK
// payload (ETSI TS 102 361-4 clause 7.1.1.3.5).
type AckOutboundPayloadPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`   // supplementary response information
	ReasonCode     byte   `dmr:"bits:7-14"`  // acknowledgement reason code
	Reserved       bool   `dmr:"bit:15"`     // reserved, 0
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target address
	AdditionalInfo uint32 `dmr:"bits:40-63"` // 24-bit additional information, usually a source address
}

// AckInboundPayloadPDU is the inbound payload acknowledgement (P_ACKU) CSBK
// payload (ETSI TS 102 361-4 clause 7.1.1.4.2).
type AckInboundPayloadPDU struct {
	ResponseInfo   uint8  `dmr:"bits:0-6"`   // supplementary response information
	ReasonCode     byte   `dmr:"bits:7-14"`  // acknowledgement reason code
	Reserved       bool   `dmr:"bit:15"`     // reserved, 0
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target address
	AdditionalInfo uint32 `dmr:"bits:40-63"` // 24-bit additional information, usually a source address
}

// RandomAccessPDU is the random access (C_RAND) CSBK payload (ETSI TS 102 361-4
// clause 7.1.1.2.1).
type RandomAccessPDU struct {
	ServiceOptions uint8  `dmr:"bits:0-6"`   // service options of the request
	ProxyFlag      bool   `dmr:"bit:7"`      // true for 21 to 44 gateway digits, false for 1 to 20
	Reserved       uint8  `dmr:"bits:8-11"`  // 4 bits whose content depends on ServiceKind
	ServiceKind    uint8  `dmr:"bits:12-15"` // service requested
	TargetAddress  uint32 `dmr:"bits:16-39"` // 24-bit target or gateway address
	SourceAddress  uint32 `dmr:"bits:40-63"` // 24-bit address of the requesting MS
}

// AckvitationPDU is the ackvitation (C_ACKVIT) CSBK payload (ETSI TS 102 361-4
// clause 7.1.1.2.2).
type AckvitationPDU struct {
	ServiceOptsMirror uint8  `dmr:"bits:0-6"`   // Service_Options_Mirror
	ServiceKindFlag   bool   `dmr:"bit:7"`      // flag whose meaning depends on ServiceKind
	Reserved          uint8  `dmr:"bits:8-9"`   // reserved, 0
	UAB               uint8  `dmr:"bits:10-11"` // Appended_Blocks, 0 to 3 for 1 to 4 appended data UDTs
	ServiceKind       uint8  `dmr:"bits:12-15"` // service that this PDU supports
	TargetAddress     uint32 `dmr:"bits:16-39"` // 24-bit target address, or an authentication challenge
	SourceAddress     uint32 `dmr:"bits:40-63"` // 24-bit individual MS address
}

// MaintenancePDU is the maintenance (P_MAINT) CSBK payload (ETSI TS 102 361-4
// clause 7.1.1.4.3).
type MaintenancePDU struct {
	Reserved      uint16 `dmr:"bits:0-11"`  // reserved, 0
	MaintKind     uint8  `dmr:"bits:12-14"` // maintenance kind, 0 for DISCON (end of payload channel use)
	Reserved2     bool   `dmr:"bit:15"`     // reserved, 0
	TargetAddress uint32 `dmr:"bits:16-39"` // 24-bit target address
	SourceAddress uint32 `dmr:"bits:40-63"` // 24-bit source address
}

// Opcode0x38Hint is the likely meaning of a CSBK with opcode 0x38, which is
// BS_Dwn_Act in Tier II and TD_GRANT_MI in Tier III.
type Opcode0x38Hint uint8

const (
	// Opcode0x38Unknown gives no hint.
	Opcode0x38Unknown Opcode0x38Hint = iota
	// Opcode0x38BSOutboundActivation hints at BS_Dwn_Act, which an MS sends
	// (ETSI TS 102 361-2 clause 5.1.1.1).
	Opcode0x38BSOutboundActivation
	// Opcode0x38TalkgroupDataGrantMultiItem hints at TD_GRANT_MI, which a
	// TSCC sends (ETSI TS 102 361-4 clause 7.1.1.1.1).
	Opcode0x38TalkgroupDataGrantMultiItem
)

// CSBK is the Control Signalling Block (CSBK) PDU (ETSI TS 102 361-1 clause 9.1.8).
//
//dmr:crc crc_ccitt
//dmr:crc_mask 0xA5A5
//dmr:input_size 96
//dmr:dispatch_if FID==0
type CSBK struct {
	LastBlock   bool               `dmr:"bit:0"`     // LB bit, true for the last block
	ProtectFlag bool               `dmr:"bit:1"`     // PF bit
	Opcode      CSBKOpcode         `dmr:"bits:2-7"`  // CSBKO, selects the payload field
	FID         enums.FeatureSetID `dmr:"bits:8-15"` // feature set ID, only the standard FID has payload fields
	FEC         fec.Result         `dmr:"-"`         // result of the CRC check
	// Hint0x38 is the likely meaning of opcode 0x38. DecodeCSBK leaves it
	// Opcode0x38Unknown. layer2.Burst sets it from the SYNC of the burst,
	// which a network relay can rewrite.
	Hint0x38 Opcode0x38Hint `dmr:"-"`

	UnitToUnitVoiceServiceRequestPDU        *UnitToUnitVoiceServiceRequestPDU        `dmr:"bits:16-79,dispatch:Opcode=CSBKUnitToUnitVoiceServiceRequest"`        // UU_V_Req payload, or nil
	UnitToUnitVoiceServiceAnswerResponsePDU *UnitToUnitVoiceServiceAnswerResponsePDU `dmr:"bits:16-79,dispatch:Opcode=CSBKUnitToUnitVoiceServiceAnswerResponse"` // UU_Ans_Rsp payload, or nil
	NegativeAcknowledgementPDU              *NegativeAcknowledgementPDU              `dmr:"bits:16-79,dispatch:Opcode=CSBKNegativeAcknowledgement"`              // NACK_Rsp payload, or nil
	PreamblePDU                             *PreamblePDU                             `dmr:"bits:16-79,dispatch:Opcode=CSBKPreamble"`                             // Pre_CSBK payload, or nil
	ChannelTimingPDU                        *ChannelTimingPDU                        `dmr:"bits:16-79,dispatch:Opcode=CSBKChannelTiming"`                        // CT_CSBK payload, or nil

	PrivateVoiceGrantPDU            *PrivateVoiceGrantPDU            `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateVoiceGrant"`            // PV_GRANT payload, or nil
	TalkgroupVoiceGrantPDU          *TalkgroupVoiceGrantPDU          `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupVoiceGrant"`          // TV_GRANT payload, or nil
	BroadcastTalkgroupVoiceGrantPDU *BroadcastTalkgroupVoiceGrantPDU `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcastTalkgroupVoiceGrant"` // BTV_GRANT payload, or nil
	PrivateDataGrantPDU             *PrivateDataGrantPDU             `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrant"`             // PD_GRANT payload, or nil
	TalkgroupDataGrantPDU           *TalkgroupDataGrantPDU           `dmr:"bits:16-79,dispatch:Opcode=CSBKTalkgroupDataGrant"`           // TD_GRANT payload, or nil
	DuplexPrivateVoiceGrantPDU      *DuplexPrivateVoiceGrantPDU      `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateVoiceGrant"`      // PV_GRANT_DX payload, or nil
	DuplexPrivateDataGrantPDU       *DuplexPrivateDataGrantPDU       `dmr:"bits:16-79,dispatch:Opcode=CSBKDuplexPrivateDataGrant"`       // PD_GRANT_DX payload, or nil
	PrivateDataGrantMultiItemPDU    *PrivateDataGrantMultiItemPDU    `dmr:"bits:16-79,dispatch:Opcode=CSBKPrivateDataGrantMultiItem"`    // PD_GRANT_MI payload, or nil
	MovePDU                         *MovePDU                         `dmr:"bits:16-79,dispatch:Opcode=CSBKMove"`                         // C_MOVE payload, or nil
	AlohaPDU                        *AlohaPDU                        `dmr:"bits:16-79,dispatch:Opcode=CSBKAloha"`                        // C_ALOHA payload, or nil
	BroadcastPDU                    *BroadcastPDU                    `dmr:"bits:16-79,dispatch:Opcode=CSBKBroadcast"`                    // C_BCAST payload, or nil
	ClearPDU                        *ClearPDU                        `dmr:"bits:16-79,dispatch:Opcode=CSBKClear"`                        // P_CLEAR payload, or nil
	ProtectPDU                      *ProtectPDU                      `dmr:"bits:16-79,dispatch:Opcode=CSBKProtect"`                      // P_PROTECT payload, or nil
	AhoyPDU                         *AhoyPDU                         `dmr:"bits:16-79,dispatch:Opcode=CSBKAhoy"`                         // C_AHOY or P_AHOY payload, or nil
	AckOutboundPDU                  *AckOutboundPDU                  `dmr:"bits:16-79,dispatch:Opcode=CSBKAckOutbound"`                  // C_ACKD payload, or nil
	AckInboundPDU                   *AckInboundPDU                   `dmr:"bits:16-79,dispatch:Opcode=CSBKAckInbound"`                   // C_ACKU payload, or nil
	AckOutboundPayloadPDU           *AckOutboundPayloadPDU           `dmr:"bits:16-79,dispatch:Opcode=CSBKAckOutboundPayload"`           // P_ACKD payload, or nil
	AckInboundPayloadPDU            *AckInboundPayloadPDU            `dmr:"bits:16-79,dispatch:Opcode=CSBKAckInboundPayload"`            // P_ACKU payload, or nil
	RandomAccessPDU                 *RandomAccessPDU                 `dmr:"bits:16-79,dispatch:Opcode=CSBKRandomAccess"`                 // C_RAND payload, or nil
	AckvitationPDU                  *AckvitationPDU                  `dmr:"bits:16-79,dispatch:Opcode=CSBKAckvitation"`                  // C_ACKVIT payload, or nil
	MaintenancePDU                  *MaintenancePDU                  `dmr:"bits:16-79,dispatch:Opcode=CSBKMaintenance"`                  // P_MAINT payload, or nil

	// RawPayload holds CSBK octets 2 to 9 (bits 16 to 79) when no payload
	// field is set, as for opcode 0x38 or a manufacturer FID.
	RawPayload [8]byte `dmr:"bits:16-79,raw,fallback"`
}

// AsBSOutboundActivation decodes the RawPayload of a standard FID CSBK with
// opcode 0x38 as a BS_Dwn_Act PDU. ok is false for another opcode or FID.
func (s *CSBK) AsBSOutboundActivation() (p BSOutboundActivationPDU, ok bool) {
	if s.Opcode != CSBKBSOutboundActivation || s.FID != 0 {
		return p, false
	}
	return decodeBSOutboundActivationPDU(s.RawPayload), true
}

// AsTalkgroupDataGrantMultiItem decodes the RawPayload of a standard FID
// CSBK with opcode 0x38 as a TD_GRANT_MI PDU. ok is false for another opcode
// or FID.
func (s *CSBK) AsTalkgroupDataGrantMultiItem() (p TalkgroupDataGrantMultiItemPDU, ok bool) {
	if s.Opcode != CSBKTalkgroupDataGrantMultiItem || s.FID != 0 {
		return p, false
	}
	return decodeTalkgroupDataGrantMultiItemPDU(s.RawPayload), true
}

// SetBSOutboundActivation makes s a standard FID BS_Dwn_Act CSBK that
// carries p: it sets Opcode, FID, Hint0x38 and RawPayload, clears the
// payload fields and keeps LastBlock and ProtectFlag. It returns the error
// of p.Encode and leaves s as it is on an error.
func (s *CSBK) SetBSOutboundActivation(p BSOutboundActivationPDU) error {
	payload, err := p.Encode()
	if err != nil {
		return err
	}
	s.setOpcode0x38(payload, Opcode0x38BSOutboundActivation)
	return nil
}

// SetTalkgroupDataGrantMultiItem makes s a standard FID TD_GRANT_MI CSBK
// that carries p, as SetBSOutboundActivation does.
func (s *CSBK) SetTalkgroupDataGrantMultiItem(p TalkgroupDataGrantMultiItemPDU) error {
	payload, err := p.Encode()
	if err != nil {
		return err
	}
	s.setOpcode0x38(payload, Opcode0x38TalkgroupDataGrantMultiItem)
	return nil
}

func (s *CSBK) setOpcode0x38(payload [8]byte, hint Opcode0x38Hint) {
	*s = CSBK{
		LastBlock:   s.LastBlock,
		ProtectFlag: s.ProtectFlag,
		Opcode:      CSBKBSOutboundActivation,
		FID:         enums.FIDStandardized,
		Hint0x38:    hint,
		RawPayload:  payload,
	}
}

// NewBSOutboundActivation returns the BS_Dwn_Act CSBK, with LastBlock set,
// that an MS sends to wake up the BS with address bs (ETSI TS 102 361-2
// clause 7.1.2.1). It returns a *dmrgo.FieldRangeError when an address does
// not fit in 24 bits.
func NewBSOutboundActivation(bs, source uint32) (CSBK, error) {
	s := CSBK{LastBlock: true}
	err := s.SetBSOutboundActivation(BSOutboundActivationPDU{BSAddress: bs, SourceAddress: source})
	return s, err
}

// DataType returns enums.DataTypeCSBK.
func (s *CSBK) DataType() enums.DataType {
	return enums.DataTypeCSBK
}
