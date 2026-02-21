package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	layer2Elements "github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	layer3Elements "github.com/USA-RedDragon/dmrgo/dmr/layer3/elements"
)

// dmr:fec reed_solomon_12_9_4
// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.6  Full Link Control (FULL LC) PDU
type FullLinkControl struct {
	DataType     layer2Elements.DataType `dmr:"-"`
	ProtectFlag  bool                    `dmr:"bit:0"`
	FLCO         enums.FLCO              `dmr:"bits:2-7,enum,err,from:enums.FLCOFromInt"`
	FeatureSetID enums.FeatureSetID      `dmr:"bits:8-15,enum,err,from:enums.FeatureSetIDFromInt"`
	FEC          fec.FECResult           `dmr:"-"`

	GroupVoice        *FLCGroupVoice        `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGroupVoiceChannelUser"`
	UnitToUnit        *FLCUnitToUnit        `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOUnitToUnitVoiceChannelUser"`
	GPSInfo           *FLCGPSInfo           `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOGPSInfo"`
	TalkerAliasHeader *FLCTalkerAliasHeader `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasHeader"`
	TalkerAliasBlock  *FLCTalkerAliasBlock  `dmr:"bits:16-71,dispatch:FLCO=enums.FLCOTalkerAliasBlock1|enums.FLCOTalkerAliasBlock2|enums.FLCOTalkerAliasBlock3"`
}

// FLC FLCO payload variants — these are the 56-bit payloads (infoBits[16:72])
// decoded per-FLCO type. Each struct uses 0-based bit offsets within the 56-bit window.

// ETSI TS 102 361-2 V2.4.1 (2017-10) - Table 7.1: Grp_V_Ch_Usr PDU content
type FLCGroupVoice struct {
	ServiceOptions layer3Elements.ServiceOptions `dmr:"bits:0-7,delegate"`
	GroupAddress   int                           `dmr:"bits:8-31"`
	SourceAddress  int                           `dmr:"bits:32-55"`
}

// ETSI TS 102 361-2 V2.4.1 (2017-10) - Table 7.2: UU_V_Ch_Usr PDU content
type FLCUnitToUnit struct {
	ServiceOptions layer3Elements.ServiceOptions `dmr:"bits:0-7,delegate"`
	TargetAddress  int                           `dmr:"bits:8-31"`
	SourceAddress  int                           `dmr:"bits:32-55"`
}

// ETSI TS 102 361-2 V2.4.1 (2017-10) - Table 7.3: GPS Info PDU content
type FLCGPSInfo struct {
	PositionError layer3Elements.PositionError `dmr:"bits:4-6,delegate"`
	Longitude     float32                      `dmr:"bits:7-31,type:longitude"`
	Latitude      float32                      `dmr:"bits:32-55,type:latitude"`
}

// ETSI TS 102 361-2 V2.4.1 (2017-10) - Table 7.4: Talker Alias Header Info PDU content
type FLCTalkerAliasHeader struct {
	TalkerAliasDataFormat layer3Elements.TalkerAliasDataFormat `dmr:"bits:0-1,delegate,noptr"`
	TalkerAliasDataLength int                                  `dmr:"bits:2-7"`
	TalkerAliasDataMSB    bool                                 `dmr:"bit:7"`
	TalkerAliasData       [48]bit.Bit                          `dmr:"bits:8-55,raw"`
}

// ETSI TS 102 361-2 V2.4.1 (2017-10) - Table 7.5: Talker Alias Blk PDU content
type FLCTalkerAliasBlock struct {
	TalkerAliasData [56]bit.Bit `dmr:"bits:0-55,raw"`
}

func (flc FullLinkControl) GetDataType() layer2Elements.DataType {
	return flc.DataType
}

func (flc FullLinkControl) ToString() string {
	ret := "FullLinkControl{ "
	ret += fmt.Sprintf("dataType: %s, ProtectFlag: %t, FLCO: %s, FeaturesetID: %s, ", layer2Elements.DataTypeToName(flc.DataType), flc.ProtectFlag, enums.FLCOToName(flc.FLCO), enums.FeatureSetIDToName(flc.FeatureSetID))

	if flc.GroupVoice != nil {
		ret += fmt.Sprintf("ServiceOptions: %s, GroupAddress: %d, SourceAddress: %d, ",
			flc.GroupVoice.ServiceOptions.ToString(),
			flc.GroupVoice.GroupAddress,
			flc.GroupVoice.SourceAddress)
	}

	if flc.UnitToUnit != nil {
		ret += fmt.Sprintf("ServiceOptions: %s, TargetAddress: %d, SourceAddress: %d, ",
			flc.UnitToUnit.ServiceOptions.ToString(),
			flc.UnitToUnit.TargetAddress,
			flc.UnitToUnit.SourceAddress)
	}

	if flc.GPSInfo != nil {
		ret += fmt.Sprintf("PositionError: %s, Longitude: %f, Latitude: %f, ",
			flc.GPSInfo.PositionError.ToString(),
			flc.GPSInfo.Longitude,
			flc.GPSInfo.Latitude)
	}

	if flc.TalkerAliasHeader != nil {
		ret += fmt.Sprintf("TalkerAliasDataFormat: %s, TalkerAliasDataLength: %d, TalkerAliasDataMSB: %t, ",
			layer3Elements.TalkerAliasDataFormatToName(flc.TalkerAliasHeader.TalkerAliasDataFormat),
			flc.TalkerAliasHeader.TalkerAliasDataLength,
			flc.TalkerAliasHeader.TalkerAliasDataMSB)
	}

	ret += fmt.Sprintf("FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }", flc.FEC.BitsChecked, flc.FEC.ErrorsCorrected, flc.FEC.Uncorrectable)

	return ret
}
