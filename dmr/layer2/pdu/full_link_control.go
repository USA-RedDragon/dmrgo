package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	reedSolomon "github.com/USA-RedDragon/dmrgo/dmr/fec/reed_solomon"
	layer2Elements "github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	layer3Elements "github.com/USA-RedDragon/dmrgo/dmr/layer3/elements"
)

// ETSI TS 102 361-1 V2.5.1 (2017-10) - 9.1.6  Full Link Control (FULL LC) PDU
// ETSI TS 102 361-2 V2.4.1 (2017-10) - 7.1.1  Full Link Control PDUs
type FullLinkControl struct {
	dataType layer2Elements.DataType

	ProtectFlag  bool // Sometimes called private call flag
	FLCO         enums.FLCO
	FeatureSetID enums.FeatureSetID

	FEC fec.FECResult

	// Table 7.1: Grp_V_Ch_Usr PDU content
	ServiceOptions layer3Elements.ServiceOptions
	GroupAddress   int
	SourceAddress  int
	// Table 7.2: UU_V_Ch_Usr PDU content
	TargetAddress int
	// Table 7.3: GPS Info PDU content
	PositionError layer3Elements.PositionError
	Longitude     float32
	Latitude      float32
	// Table 7.4: Talker Alias header Info PDU content
	TalkerAliasDataFormat layer3Elements.TalkerAliasDataFormat
	TalkerAliasDataLength int
	TalkerAliasDataMSB    bool
	// without msb talker alias header data are 48 bits (6 bytes)
	TalkerAliasDataLen int
	TalkerAliasData    [72]bit.Bit
	// Table 7.5: Talker Alias block Info PDU content
	// talker alias blocks 1,2,3 use "talker_alias_data" field, since data are 56bits (7bytes)
}

// FLC FLCO payload variants — these are the 56-bit payloads (infoBits[16:72])
// decoded per-FLCO type. Each struct uses 0-based bit offsets within the 56-bit window.

// FLCGroupVoice is the payload of FLCO Group Voice Channel User (Table 7.1).
type FLCGroupVoice struct {
	ServiceOptions layer3Elements.ServiceOptions `dmr:"bits:0-7,delegate"`
	GroupAddress   int                           `dmr:"bits:8-31"`
	SourceAddress  int                           `dmr:"bits:32-55"`
}

// FLCUnitToUnit is the payload of FLCO Unit to Unit Voice Channel User (Table 7.2).
type FLCUnitToUnit struct {
	ServiceOptions layer3Elements.ServiceOptions `dmr:"bits:0-7,delegate"`
	TargetAddress  int                           `dmr:"bits:8-31"`
	SourceAddress  int                           `dmr:"bits:32-55"`
}

// FLCGPSInfo is the payload of FLCO GPS Info (Table 7.3).
type FLCGPSInfo struct {
	PositionError layer3Elements.PositionError `dmr:"bits:4-6,delegate"`
	Longitude     float32                      `dmr:"bits:7-31,type:longitude"`
	Latitude      float32                      `dmr:"bits:32-55,type:latitude"`
}

// FLCTalkerAliasHeader is the payload of FLCO Talker Alias Header (Table 7.4).
type FLCTalkerAliasHeader struct {
	TalkerAliasDataFormat layer3Elements.TalkerAliasDataFormat `dmr:"bits:0-1,delegate,noptr"`
	TalkerAliasDataLength int                                  `dmr:"bits:2-7"`
	TalkerAliasDataMSB    bool                                 `dmr:"bit:7"`
	TalkerAliasData       [48]bit.Bit                          `dmr:"bits:8-55,raw"`
}

// FLCTalkerAliasBlock is the payload of FLCO Talker Alias Block 1/2/3 (Table 7.5).
type FLCTalkerAliasBlock struct {
	TalkerAliasData [56]bit.Bit `dmr:"bits:0-55,raw"`
}

func (flc FullLinkControl) GetDataType() layer2Elements.DataType {
	return flc.dataType
}

func (flc FullLinkControl) ToString() string {
	ret := "FullLinkControl{ "
	ret += fmt.Sprintf("dataType: %s, ProtectFlag: %t, FLCO: %s, FeaturesetID: %s, ", layer2Elements.DataTypeToName(flc.dataType), flc.ProtectFlag, enums.FLCOToName(flc.FLCO), enums.FeatureSetIDToName(flc.FeatureSetID))

	if flc.FLCO == enums.FLCOUnitToUnitVoiceChannelUser || flc.FLCO == enums.FLCOGroupVoiceChannelUser {
		ret += fmt.Sprintf("ServiceOptions: %s, SourceAddress: %d, ", flc.ServiceOptions.ToString(), flc.SourceAddress)
	}

	if flc.FLCO == enums.FLCOGroupVoiceChannelUser {
		ret += fmt.Sprintf("GroupAddress: %d, ", flc.GroupAddress)
	}

	if flc.FLCO == enums.FLCOUnitToUnitVoiceChannelUser {
		ret += fmt.Sprintf("TargetAddress: %d, ", flc.TargetAddress)
	}

	if flc.FLCO == enums.FLCOGPSInfo {
		ret += fmt.Sprintf("PositionError: %s, Longitude: %f, Latitude: %f, ", flc.PositionError.ToString(), flc.Longitude, flc.Latitude)
	}

	if flc.FLCO == enums.FLCOTalkerAliasHeader || flc.FLCO == enums.FLCOTalkerAliasBlock1 || flc.FLCO == enums.FLCOTalkerAliasBlock2 || flc.FLCO == enums.FLCOTalkerAliasBlock3 {
		ret += fmt.Sprintf("TalkerAliasDataFormat: %s, TalkerAliasDataLength: %d, TalkerAliasDataMSB: %t, ", layer3Elements.TalkerAliasDataFormatToName(flc.TalkerAliasDataFormat), flc.TalkerAliasDataLength, flc.TalkerAliasDataMSB)
	}

	ret += fmt.Sprintf("FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }", flc.FEC.BitsChecked, flc.FEC.ErrorsCorrected, flc.FEC.Uncorrectable)

	return ret
}

func (flc *FullLinkControl) DecodeFromBits(infoBits []bit.Bit, dataType layer2Elements.DataType) bool {
	if len(infoBits) != 96 && len(infoBits) != 77 {
		fmt.Println("FullLinkControl: invalid infoBits length: ", len(infoBits))
		return false
	}

	if dataType != layer2Elements.DataTypeTerminatorWithLC && dataType != layer2Elements.DataTypeVoiceLCHeader {
		fmt.Println("FullLinkControl: invalid dataType: ", dataType)
		return false
	}

	var flco int
	for i := 2; i < 8; i++ {
		flco <<= 1
		flco |= int(infoBits[i])
	}

	FLCO, err := enums.FLCOFromInt(flco)
	if err != nil {
		fmt.Println("FullLinkControl: invalid FLCO: ", flco)
		return false
	}

	var fsid int
	for i := 8; i < 16; i++ {
		fsid <<= 1
		fsid |= int(infoBits[i])
	}
	FSID, err := enums.FeatureSetIDFromInt(fsid)
	if err != nil {
		fmt.Println("FullLinkControl: invalid FeatureSetID: ", fsid)
		return false
	}

	var infoBytes [12]byte
	for i := 0; i < 96; i += 8 {
		var b byte
		for j := 0; j < 8; j++ {
			b <<= 1
			b |= byte(infoBits[i+j])
		}
		infoBytes[i/8] = b
	}

	syndrome := &reedSolomon.ReedSolomon1294{}
	if err := reedSolomon.ReedSolomon1294CalcSyndrome(infoBytes[:], syndrome); err != nil {
		fmt.Println("FullLinkControl: error calculating syndrome: ", err)
		return false
	}
	var rsCorrected int
	if !reedSolomon.ReedSolomon1294CheckSyndrome(syndrome) {
		fmt.Println("FullLinkControl: syndrome check failed")
		corrected, err := reedSolomon.ReedSolomon1294Correct(infoBytes[:], syndrome)
		if err != nil {
			fmt.Println("FullLinkControl: error correcting syndrome: ", err)
			return false
		}
		rsCorrected = corrected
	}

	// reset fields
	*flc = FullLinkControl{}
	flc.dataType = dataType
	flc.FLCO = FLCO
	flc.ProtectFlag = infoBits[0] == 1
	flc.FeatureSetID = FSID
	flc.FEC = fec.FECResult{BitsChecked: 96, ErrorsCorrected: rsCorrected}

	switch FLCO {
	case enums.FLCOUnitToUnitVoiceChannelUser:
		var payloadBits [56]bit.Bit
		copy(payloadBits[:], infoBits[16:72])
		uu, _ := DecodeFLCUnitToUnit(payloadBits)
		flc.ServiceOptions = uu.ServiceOptions
		flc.TargetAddress = uu.TargetAddress
		flc.SourceAddress = uu.SourceAddress

	case enums.FLCOGroupVoiceChannelUser:
		var payloadBits [56]bit.Bit
		copy(payloadBits[:], infoBits[16:72])
		gv, _ := DecodeFLCGroupVoice(payloadBits)
		flc.ServiceOptions = gv.ServiceOptions
		flc.GroupAddress = gv.GroupAddress
		flc.SourceAddress = gv.SourceAddress

	case enums.FLCOGPSInfo:
		var payloadBits [56]bit.Bit
		copy(payloadBits[:], infoBits[16:72])
		gps, _ := DecodeFLCGPSInfo(payloadBits)
		flc.PositionError = gps.PositionError
		flc.Longitude = gps.Longitude
		flc.Latitude = gps.Latitude

	case enums.FLCOTalkerAliasHeader:
		var payloadBits [56]bit.Bit
		copy(payloadBits[:], infoBits[16:72])
		tah, _ := DecodeFLCTalkerAliasHeader(payloadBits)
		flc.TalkerAliasDataFormat = tah.TalkerAliasDataFormat
		flc.TalkerAliasDataLength = tah.TalkerAliasDataLength
		flc.TalkerAliasDataMSB = tah.TalkerAliasDataMSB

		// Dynamic clipping: only copy up to TalkerAliasDataLength bits (max 48)
		taLen := tah.TalkerAliasDataLength
		if taLen > 48 {
			taLen = 48
		}
		flc.TalkerAliasDataLen = taLen
		copy(flc.TalkerAliasData[:], tah.TalkerAliasData[:taLen])

	case enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3:
		var payloadBits [56]bit.Bit
		copy(payloadBits[:], infoBits[16:72])
		tab, _ := DecodeFLCTalkerAliasBlock(payloadBits)
		const blockLen = 56
		flc.TalkerAliasDataLen = blockLen
		copy(flc.TalkerAliasData[:], tab.TalkerAliasData[:])
	case enums.FLCOTerminatorDataLinkControl:
		// TODO: implement TLDC handling
		return false
	default:
		return false
	}

	return true
}

// Encode serializes the FullLinkControl PDU into 12 bytes (9 data + 3 CRC).
func (flc *FullLinkControl) Encode() ([]byte, error) {
	data := make([]byte, 9)

	// Byte 0: PF(bit 7) + R(bit 6) + FLCO(bits 5-0)
	if flc.ProtectFlag {
		data[0] |= 0x80
	}
	// R is assumed 0
	data[0] |= byte(flc.FLCO) & 0x3F

	// Byte 1: FID
	data[1] = byte(flc.FeatureSetID)

	switch flc.FLCO {
	case enums.FLCOGroupVoiceChannelUser:
		gv := FLCGroupVoice{
			ServiceOptions: flc.ServiceOptions,
			GroupAddress:   flc.GroupAddress,
			SourceAddress:  flc.SourceAddress,
		}
		payloadBits := EncodeFLCGroupVoice(&gv)
		copy(data[2:9], bit.PackBits(payloadBits[:]))

	case enums.FLCOUnitToUnitVoiceChannelUser:
		uu := FLCUnitToUnit{
			ServiceOptions: flc.ServiceOptions,
			TargetAddress:  flc.TargetAddress,
			SourceAddress:  flc.SourceAddress,
		}
		payloadBits := EncodeFLCUnitToUnit(&uu)
		copy(data[2:9], bit.PackBits(payloadBits[:]))

	case enums.FLCOTalkerAliasHeader,
		enums.FLCOTalkerAliasBlock1,
		enums.FLCOTalkerAliasBlock2,
		enums.FLCOTalkerAliasBlock3,
		enums.FLCOGPSInfo,
		enums.FLCOTerminatorDataLinkControl:
		return nil, fmt.Errorf("FullLinkControl Encode: unsupported FLCO %s", enums.FLCOToName(flc.FLCO))
	}

	// Calculate CRC (Reed-Solomon 12,9)
	encoded, err := reedSolomon.Encode(data)
	if err != nil {
		return nil, err
	}
	// encoded is 12 bytes (9 data + 3 parity)
	return encoded, nil
}
