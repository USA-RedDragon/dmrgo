package pdu

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// FullLinkControl is the Full Link Control (Full LC) PDU (ETSI TS 102 361-1 clause 9.1.6).
//
//dmr:fec reed_solomon_12_9_4
//dmr:input BurstDataType
//dmr:fec_mask BurstDataType enums.DataTypeVoiceLCHeader=0x969696 enums.DataTypeTerminatorWithLC=0x999999
//dmr:dispatch_if FID==0
type FullLinkControl struct {
	// BurstDataType selects the RS(12,9) parity mask; Encode returns an error unless it is DataTypeVoiceLCHeader or DataTypeTerminatorWithLC.
	BurstDataType enums.DataType `dmr:"-"`
	ProtectFlag   bool           `dmr:"bit:0"` // Protect Flag (PF)
	Reserved      bool           `dmr:"bit:1"` // reserved
	// FLCO is the Full Link Control Opcode; Encode returns an error unless FLCO and FID select the one payload field that is set.
	FLCO enums.FLCO `dmr:"bits:2-7,enum"`
	// FID is the Feature set ID; the payload fields apply only to the standard FID 0, and RawPayload holds the payload of any other FID.
	FID enums.FeatureSetID `dmr:"bits:8-15,enum"`
	// FEC is the RS(12,9) result; MMDVMHost accepts only an LC with FEC.ErrorsCorrected 0 (layer2.MMDVMHostAccepts).
	FEC fec.Result `dmr:"-"`

	GroupVoice                *FLCGroupVoice                `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGroupVoiceChannelUser"`                                                     // Grp_V_Ch_Usr payload
	UnitToUnit                *FLCUnitToUnit                `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOUnitToUnitVoiceChannelUser"`                                                // UU_V_Ch_Usr payload
	GPSInfo                   *FLCGPSInfo                   `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGPSInfo"`                                                                   // GPS_Info payload
	TalkerAliasHeader         *FLCTalkerAliasHeader         `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasHeader"`                                                         // Talker_Alias_HDR payload
	TalkerAliasBlock          *FLCTalkerAliasBlock          `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasBlock1|enums.FLCOTalkerAliasBlock2|enums.FLCOTalkerAliasBlock3"` // Talker_Alias_BLK1 to BLK3 payload
	TerminatorDataLinkControl *FLCTerminatorDataLinkControl `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTerminatorDataLinkControl"`                                                 // terminator data link control payload

	// RawPayload holds LC bits 16 to 71 when no payload field is set, as
	// for a manufacturer FLCO, packed most significant bit first: bit 16
	// is the top bit of byte 0, so the bytes are LC octets 2 to 8.
	RawPayload [7]byte `dmr:"bits:16-71,raw,fallback"`
}

// Addresses returns the 24-bit target and source addresses of a Grp_V_Ch_Usr
// (FLCO 0) or UU_V_Ch_Usr (FLCO 3) LC. It reads RawPayload when no payload
// field is set, so, as MMDVMHost does, it also returns best-effort addresses
// from a manufacturer FID LC with these FLCOs. ok is false for other FLCOs.
// (ETSI TS 102 361-2 Tables 7.1 and 7.2)
func (s *FullLinkControl) Addresses() (target, source uint32, ok bool) {
	if s == nil {
		return 0, 0, false
	}
	if s.FLCO == enums.FLCOGroupVoiceChannelUser && s.GroupVoice != nil {
		return s.GroupVoice.GroupAddress, s.GroupVoice.SourceAddress, true
	}
	if s.FLCO == enums.FLCOUnitToUnitVoiceChannelUser && s.UnitToUnit != nil {
		return s.UnitToUnit.TargetAddress, s.UnitToUnit.SourceAddress, true
	}
	if s.FLCO != enums.FLCOGroupVoiceChannelUser && s.FLCO != enums.FLCOUnitToUnitVoiceChannelUser {
		return 0, 0, false
	}
	return uint24(s.RawPayload[1:4]), uint24(s.RawPayload[4:7]), true
}

// SetAddresses sets the target and source addresses of a Grp_V_Ch_Usr or
// UU_V_Ch_Usr LC: in GroupVoice or UnitToUnit for the standard FID, and in
// RawPayload for any other FID. It returns ErrNoAddresses for other FLCOs
// and for the standard FID when the payload field of the FLCO is nil
// (NewVoiceLC and a decode set it), and a *dmrgo.FieldRangeError when an
// address does not fit in 24 bits, and then changes nothing.
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
	if s.FID != enums.FIDStandardized {
		putUint24(s.RawPayload[1:4], target)
		putUint24(s.RawPayload[4:7], source)
		return nil
	}
	if s.FLCO == enums.FLCOGroupVoiceChannelUser {
		if s.GroupVoice == nil {
			return ErrNoAddresses
		}
		s.GroupVoice.GroupAddress, s.GroupVoice.SourceAddress = target, source
		return nil
	}
	if s.UnitToUnit == nil {
		return ErrNoAddresses
	}
	s.UnitToUnit.TargetAddress, s.UnitToUnit.SourceAddress = target, source
	return nil
}

// NewVoiceLC returns the DataTypeVoiceLCHeader standard FID Full LC of a
// voice call: Grp_V_Ch_Usr when group is true, else UU_V_Ch_Usr, with Service
// Options so. MMDVMHost sends the zero value and Motorola IPSC
// layer3.MotorolaServiceOptions. It returns a *dmrgo.FieldRangeError when an
// address does not fit in 24 bits. It allocates the payload struct of the LC.
// (ETSI TS 102 361-2 Tables 7.1 and 7.2)
func NewVoiceLC(group bool, target, source uint32, so layer3.ServiceOptions) (FullLinkControl, error) {
	if err := checkAddresses(target, source, errFullLinkControlTargetRange, errFullLinkControlSourceRange); err != nil {
		return FullLinkControl{}, err
	}
	s := FullLinkControl{BurstDataType: enums.DataTypeVoiceLCHeader, FLCO: enums.FLCOUnitToUnitVoiceChannelUser}
	if group {
		s.FLCO = enums.FLCOGroupVoiceChannelUser
		s.GroupVoice = &FLCGroupVoice{ServiceOptions: so, GroupAddress: target, SourceAddress: source}
	} else {
		s.UnitToUnit = &FLCUnitToUnit{ServiceOptions: so, TargetAddress: target, SourceAddress: source}
	}
	return s, nil
}

// ServiceOptions returns the Service Options of a Grp_V_Ch_Usr (FLCO 0) or
// UU_V_Ch_Usr (FLCO 3) LC. Like Addresses, it reads RawPayload when no
// payload field is set, so it also reads a manufacturer FID LC with these
// FLCOs. ok is false for other FLCOs.
func (s *FullLinkControl) ServiceOptions() (so layer3.ServiceOptions, ok bool) {
	if s == nil {
		return layer3.ServiceOptions{}, false
	}
	switch {
	case s.FLCO == enums.FLCOGroupVoiceChannelUser && s.GroupVoice != nil:
		return s.GroupVoice.ServiceOptions, true
	case s.FLCO == enums.FLCOUnitToUnitVoiceChannelUser && s.UnitToUnit != nil:
		return s.UnitToUnit.ServiceOptions, true
	case s.FLCO == enums.FLCOGroupVoiceChannelUser, s.FLCO == enums.FLCOUnitToUnitVoiceChannelUser:
		return layer3.DecodeServiceOptions([1]byte{s.RawPayload[0]}), true
	}
	return so, false
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
	TalkerAliasDataLength int                          `dmr:"bits:2-6"`                // alias length in Code Units
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
	GroupOrIndividual  bool   `dmr:"bit:48"`     // true when LLIDDestination is a group
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
