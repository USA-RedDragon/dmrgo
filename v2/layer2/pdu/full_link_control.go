package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// FullLinkControl is the Full Link Control (Full LC) PDU. FLCO and FID
// select the view of RawPayload: AsGroupVoice, for example, decodes it for
// FLCO enums.FLCOGroupVoiceChannelUser and the standard FID 0.
// (ETSI TS 102 361-1 clause 9.1.6)
//
//dmr:fec reed_solomon_12_9_4
//dmr:input BurstDataType
//dmr:fec_mask BurstDataType err:ErrNoFullLCMask enums.DataTypeVoiceLCHeader=0x969696 enums.DataTypeTerminatorWithLC=0x999999
//dmr:dispatch_if FID==0
//dmr:variant GroupVoice FLCGroupVoice FLCO=enums.FLCOGroupVoiceChannelUser
//dmr:variant UnitToUnit FLCUnitToUnit FLCO=enums.FLCOUnitToUnitVoiceChannelUser
//dmr:variant GPSInfo FLCGPSInfo FLCO=enums.FLCOGPSInfo
//dmr:variant TalkerAliasHeader FLCTalkerAliasHeader FLCO=enums.FLCOTalkerAliasHeader
//dmr:variant TalkerAliasBlock FLCTalkerAliasBlock FLCO=enums.FLCOTalkerAliasBlock1|enums.FLCOTalkerAliasBlock2|enums.FLCOTalkerAliasBlock3
//dmr:variant TerminatorDataLinkControl FLCTerminatorDataLinkControl FLCO=enums.FLCOTerminatorDataLinkControl
type FullLinkControl struct {
	// BurstDataType selects the RS(12,9) parity mask; Encode returns an error unless it is DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
	BurstDataType enums.DataType `dmr:"-"`
	ProtectFlag   bool           `dmr:"bit:0"`         // Protect Flag (PF)
	Reserved      bool           `dmr:"bit:1"`         // reserved
	FLCO          enums.FLCO     `dmr:"bits:2-7,enum"` // Full Link Control Opcode (FLCO)
	// FID is the Feature set ID; the As methods apply only to the standard FID 0, and RawPayload holds the payload of any FID.
	FID enums.FID `dmr:"bits:8-15,enum"`
	// FEC is the RS(12,9) result; MMDVMHost accepts only an LC with FEC.ErrorsCorrected 0 (layer2.MMDVMHostAccepts).
	FEC fec.Result `dmr:"-"`

	// RawPayload holds LC bits 16 to 71, the payload that FLCO and FID
	// select, packed most significant bit first: bit 16 is the top bit of
	// byte 0, so the bytes are LC octets 2 to 8.
	RawPayload [7]byte `dmr:"bits:16-71,raw,union"`
}

// Addresses returns the 24-bit target and source addresses of a Grp_V_Ch_Usr
// (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC. It reads RawPayload for any FID, so,
// as MMDVMHost does, it also returns best-effort addresses from a
// manufacturer FID LC with these FLCOs. ok is false for other FLCOs.
// (ETSI TS 102 361-2 Tables 7.1 and 7.2)
func (s *FullLinkControl) Addresses() (target, source uint32, ok bool) {
	if s == nil {
		return 0, 0, false
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return 0, 0, false
	}
	return uint24(s.RawPayload[1:4]), uint24(s.RawPayload[4:7]), true
}

// Call returns the call of a Grp_V_Ch_Usr (FLCO 0) or UU_V_Ch_Usr (FLCO 3)
// LC: group for FLCO 0, the 24-bit target and the source. ok is false for
// other FLCOs. It reads RawPayload for any FID, as Addresses does.
func (s *FullLinkControl) Call() (group bool, target, source uint32, ok bool) {
	target, source, ok = s.Addresses()
	return ok && s.FLCO == enums.FLCOGroupVoiceChannelUser, target, source, ok
}

// SetAddresses sets the target and source addresses of a Grp_V_Ch_Usr or
// UU_V_Ch_Usr LC of any FID in RawPayload. It returns ErrNoAddresses for
// other FLCOs and a *dmrgo.FieldRangeError when an address does not fit in
// 24 bits, and then changes nothing.
func (s *FullLinkControl) SetAddresses(target, source uint32) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	if err := checkAddresses(target, source, errFullLinkControlTargetRange, errFullLinkControlSourceRange); err != nil {
		return err
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return ErrNoAddresses
	}
	putUint24(s.RawPayload[1:4], target)
	putUint24(s.RawPayload[4:7], source)
	return nil
}

// NewVoiceLC returns the DataTypeVoiceLCHeader standard FID Full LC of a
// voice call: Grp_V_Ch_Usr when group is true, else UU_V_Ch_Usr, with Service
// Options so. MMDVMHost sends the zero value and Motorola IPSC
// layer3.MotorolaServiceOptions. It returns a *dmrgo.FieldRangeError when an
// address does not fit in 24 bits. (ETSI TS 102 361-2 Tables 7.1 and 7.2)
func NewVoiceLC(group bool, target, source uint32, so layer3.ServiceOptions) (FullLinkControl, error) {
	if err := checkAddresses(target, source, errFullLinkControlTargetRange, errFullLinkControlSourceRange); err != nil {
		return FullLinkControl{}, err
	}
	s := FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader}
	var err error
	if group {
		err = s.SetGroupVoice(FLCGroupVoice{ServiceOptions: so, GroupAddress: target, SourceAddress: source})
	} else {
		err = s.SetUnitToUnit(FLCUnitToUnit{ServiceOptions: so, TargetAddress: target, SourceAddress: source})
	}
	if err != nil {
		return FullLinkControl{}, err
	}
	return s, nil
}

// ServiceOptions returns the Service Options of a Grp_V_Ch_Usr (FLCO 0) or
// UU_V_Ch_Usr (FLCO 3) LC. Like Addresses, it reads RawPayload for any FID,
// so it also reads a manufacturer FID LC with these FLCOs. ok is false for
// other FLCOs.
func (s *FullLinkControl) ServiceOptions() (so layer3.ServiceOptions, ok bool) {
	if s == nil {
		return layer3.ServiceOptions{}, false
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return so, false
	}
	return layer3.DecodeServiceOptions([1]byte{s.RawPayload[0]}), true
}

// FLCGroupVoice is the group voice channel user (Grp_V_Ch_Usr) Full LC payload
// (ETSI TS 102 361-2 clause 7.1.1.1, Table 7.1).
type FLCGroupVoice struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // Service Options
	GroupAddress   uint32                `dmr:"bits:8-31"`         // 24-bit group address
	SourceAddress  uint32                `dmr:"bits:32-55"`        // 24-bit source address
}

// FLCUnitToUnit is the unit to unit voice channel user (UU_V_Ch_Usr) Full LC
// payload (ETSI TS 102 361-2 clause 7.1.1.2, Table 7.2).
type FLCUnitToUnit struct {
	ServiceOptions layer3.ServiceOptions `dmr:"bits:0-7,delegate"` // Service Options
	TargetAddress  uint32                `dmr:"bits:8-31"`         // 24-bit target address
	SourceAddress  uint32                `dmr:"bits:32-55"`        // 24-bit source address
}

// FLCGPSInfo is the GPS information (GPS_Info) Full LC payload (ETSI TS 102
// 361-2 clause 7.1.1.3, Table 7.3).
type FLCGPSInfo struct {
	Reserved      uint8                `dmr:"bits:0-3"`                 // reserved
	PositionError layer3.PositionError `dmr:"bits:4-6,delegate,noptr"`  // Position Error
	Longitude     float64              `dmr:"bits:7-31,type:longitude"` // degrees, -180 to 180
	Latitude      float64              `dmr:"bits:32-55,type:latitude"` // degrees, -90 to 90
}

// FLCTalkerAliasHeader is the talker alias header information
// (Talker_Alias_HDR) Full LC payload (ETSI TS 102 361-2 clause 7.1.1.4, Table 7.4).
type FLCTalkerAliasHeader struct {
	TalkerAliasDataFormat layer3.TalkerAliasDataFormat `dmr:"bits:0-1,delegate,noptr"` // Talker alias data format
	TalkerAliasDataLength uint8                        `dmr:"bits:2-6"`                // alias length in Code Units
	// TalkerAliasData is the 49-bit Talker alias data IE. The 7-bit format
	// uses all 49 bits; the other formats leave the most significant bit
	// unused.
	TalkerAliasData uint64 `dmr:"bits:7-55"`
}

// FLCTalkerAliasBlock is a talker alias block (Talker_Alias_BLK) Full LC
// payload (ETSI TS 102 361-2 clause 7.1.1.5, Table 7.5).
type FLCTalkerAliasBlock struct {
	TalkerAliasData [7]byte `dmr:"bits:0-55,packed"` // 56 bits of Talker alias data
}

// FLCTerminatorDataLinkControl is the terminator data link control Full LC
// payload (ETSI TS 102 361-3 clause 7.1.1.1, Table 7.1).
type FLCTerminatorDataLinkControl struct {
	LLIDDestination    uint32 `dmr:"bits:0-23"`  // destination LLID
	LLIDSource         uint32 `dmr:"bits:24-47"` // source LLID
	Group              bool   `dmr:"bit:48"`     // true when LLIDDestination is a group
	ResponseRequested  bool   `dmr:"bit:49"`     // Response Requested (A)
	FullMessageFlag    bool   `dmr:"bit:50"`     // Full Message Flag (F)
	Reserved           bool   `dmr:"bit:51"`     // reserved
	ReSynchronizeFlag  bool   `dmr:"bit:52"`     // Re-Synchronize Flag (S)
	SendSequenceNumber uint8  `dmr:"bits:53-55"` // send sequence number N(S)
}

// DataType returns BurstDataType.
func (s *FullLinkControl) DataType() enums.DataType {
	if s == nil {
		return 0
	}
	return s.BurstDataType
}
