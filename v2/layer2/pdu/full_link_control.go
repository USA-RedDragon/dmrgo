package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// ETSI TS 102 361-1 - 9.1.6  Full Link Control (FULL LC) PDU
//
// The FID redefines the FLCO (ETSI TS 102 361-1 Table 9.21), so the standard
// payload layouts apply only to the SFID. Payloads of other FIDs and of
// unknown FLCOs are kept in RawPayload. FullLinkControl.Encode returns an
// error if more than one payload field is set, or if FLCO and FeatureSetID
// do not select the payload field that is set. Put the payload of a
// manufacturer FID in RawPayload.
//
// BurstDataType, the data type of the burst that carries the LC, selects the
// Data Type CRC Mask XORed into the RS(12,9) parity (ETSI TS 102 361-1
// B.3.12, Table B.21). Pass it to DecodeFullLinkControl and set it before
// FullLinkControl.Encode. FullLinkControl.Encode returns an error unless
// BurstDataType is
// DataTypeVoiceLCHeader or DataTypeTerminatorWithLC, the only data types
// that carry an RS-protected Full LC. DecodeFullLinkControl applies no mask
// for other data types.
//
// FEC is the RS(12,9) result and ErrorsCorrected counts the corrected
// errors. MMDVMHost accepts a Full LC only when the RS(12,9) parity matches
// without correction; a caller that must match it rejects an LC with
// FEC.ErrorsCorrected > 0 (see layer2.MMDVMHostAccepts).
//
//dmr:fec reed_solomon_12_9_4
//dmr:input BurstDataType
//dmr:fec_mask BurstDataType enums.DataTypeVoiceLCHeader=0x969696 enums.DataTypeTerminatorWithLC=0x999999
//dmr:dispatch_if FeatureSetID==0
type FullLinkControl struct {
	BurstDataType enums.DataType     `dmr:"-"`
	ProtectFlag   bool               `dmr:"bit:0"`
	Reserved      bool               `dmr:"bit:1"`
	FLCO          enums.FLCO         `dmr:"bits:2-7,enum"`
	FeatureSetID  enums.FeatureSetID `dmr:"bits:8-15,enum"`
	FEC           fec.Result         `dmr:"-"`

	GroupVoice                *FLCGroupVoice                `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGroupVoiceChannelUser"`
	UnitToUnit                *FLCUnitToUnit                `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOUnitToUnitVoiceChannelUser"`
	GPSInfo                   *FLCGPSInfo                   `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGPSInfo"`
	TalkerAliasHeader         *FLCTalkerAliasHeader         `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasHeader"`
	TalkerAliasBlock          *FLCTalkerAliasBlock          `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasBlock1|enums.FLCOTalkerAliasBlock2|enums.FLCOTalkerAliasBlock3"`
	TerminatorDataLinkControl *FLCTerminatorDataLinkControl `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTerminatorDataLinkControl"`

	RawPayload [56]bit.Bit `dmr:"bits:16-71,raw,fallback"`
}

// Addresses returns the 24-bit target and source addresses (LC bits 24-47
// and 48-71) of a Grp_V_Ch_Usr (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC (ETSI TS
// 102 361-2 Tables 7.1 and 7.2). For a group call the target is the group
// address. It ignores the FID and reads RawPayload when no variant was
// decoded, so it also returns addresses from a manufacturer FID LC (such as
// Motorola, 0x10) with these FLCOs, as MMDVMHost does. A manufacturer FID
// may redefine the payload, so such addresses are a best effort. ok is false
// for other FLCOs.
func (s *FullLinkControl) Addresses() (target, source uint32, ok bool) {
	if s.FLCO == enums.FLCOGroupVoiceChannelUser && s.GroupVoice != nil {
		return s.GroupVoice.GroupAddress, s.GroupVoice.SourceAddress, true
	}
	if s.FLCO == enums.FLCOUnitToUnitVoiceChannelUser && s.UnitToUnit != nil {
		return s.UnitToUnit.TargetAddress, s.UnitToUnit.SourceAddress, true
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return 0, 0, false
	}
	return uint32(bit.Uint(s.RawPayload[8:32])), uint32(bit.Uint(s.RawPayload[32:56])), true //nolint:gosec // 24-bit fields
}

// SetAddresses sets the target and source addresses of a Grp_V_Ch_Usr
// (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC. With the standard FID it sets
// GroupVoice or UnitToUnit, creating the payload when it is nil. With any
// other FID it writes RawPayload bits 8-31 and 32-55, the layout Addresses
// reads. It returns false, and changes nothing, for other FLCOs or when an
// address does not fit in 24 bits.
func (s *FullLinkControl) SetAddresses(target, source uint32) bool {
	if target>>24 != 0 || source>>24 != 0 {
		return false
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return false
	}
	if s.FeatureSetID != enums.FIDStandardized {
		bit.PutUint(s.RawPayload[8:32], uint64(target))
		bit.PutUint(s.RawPayload[32:56], uint64(source))
		return true
	}
	if s.FLCO == enums.FLCOGroupVoiceChannelUser {
		if s.GroupVoice == nil {
			s.GroupVoice = &FLCGroupVoice{}
		}
		s.GroupVoice.GroupAddress, s.GroupVoice.SourceAddress = target, source
		return true
	}
	if s.UnitToUnit == nil {
		s.UnitToUnit = &FLCUnitToUnit{}
	}
	s.UnitToUnit.TargetAddress, s.UnitToUnit.SourceAddress = target, source
	return true
}

// FLC FLCO payload variants — these are the 56-bit payloads (infoBits[16:72])
// decoded per-FLCO type. Each struct uses 0-based bit offsets within the 56-bit window.

// ETSI TS 102 361-2 - Table 7.1: Grp_V_Ch_Usr PDU content
type FLCGroupVoice struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"`
	GroupAddress   uint32                `dmr:"bits:8-31"`
	SourceAddress  uint32                `dmr:"bits:32-55"`
}

// ETSI TS 102 361-2 - Table 7.2: UU_V_Ch_Usr PDU content
type FLCUnitToUnit struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"`
	TargetAddress  uint32                `dmr:"bits:8-31"`
	SourceAddress  uint32                `dmr:"bits:32-55"`
}

// ETSI TS 102 361-2 - Table 7.3: GPS Info PDU content
type FLCGPSInfo struct {
	Reserved      uint8                `dmr:"bits:0-3"`
	PositionError layer3.PositionError `dmr:"bits:4-6,delegate,noptr"`
	Longitude     float64              `dmr:"bits:7-31,type:longitude"`
	Latitude      float64              `dmr:"bits:32-55,type:latitude"`
}

// ETSI TS 102 361-2 - Table 7.4: Talker Alias Header Info PDU content
type FLCTalkerAliasHeader struct {
	TalkerAliasDataFormat layer3.TalkerAliasDataFormat `dmr:"bits:0-1,delegate,noptr"`
	TalkerAliasDataLength int                          `dmr:"bits:2-6"`
	// TalkerAliasData is the 49-bit Talker alias data IE. The 7-bit format
	// uses all 49 bits; the other formats leave the most significant bit
	// unused.
	TalkerAliasData uint64 `dmr:"bits:7-55"`
}

// ETSI TS 102 361-2 - Table 7.5: Talker Alias Blk PDU content
type FLCTalkerAliasBlock struct {
	TalkerAliasData [7]byte `dmr:"bits:0-55,packed"`
}

// ETSI TS 102 361-3 - Table 7.1: Terminator Data Link Control PDU content
type FLCTerminatorDataLinkControl struct {
	LLIDDestination    uint32 `dmr:"bits:0-23"`
	LLIDSource         uint32 `dmr:"bits:24-47"`
	GroupOrIndividual  bool   `dmr:"bit:48"`
	ResponseRequested  bool   `dmr:"bit:49"`
	FullMessageFlag    bool   `dmr:"bit:50"`
	Reserved           bool   `dmr:"bit:51"`
	ReSynchronizeFlag  bool   `dmr:"bit:52"`
	SendSequenceNumber uint8  `dmr:"bits:53-55"`
}

// DataType returns BurstDataType.
func (s *FullLinkControl) DataType() enums.DataType {
	return s.BurstDataType
}
