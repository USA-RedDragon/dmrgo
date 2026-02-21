package pdu

import (
	"fmt"
	"math"

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
		var sizedBits [8]bit.Bit
		copy(sizedBits[:], infoBits[16:24])
		flc.ServiceOptions = *layer3Elements.NewServiceOptionsFromBits(sizedBits)
		for i := 24; i < 48; i++ {
			flc.TargetAddress <<= 1
			flc.TargetAddress |= int(infoBits[i])
		}

		for i := 48; i < 72; i++ {
			flc.SourceAddress <<= 1
			flc.SourceAddress |= int(infoBits[i])
		}
	case enums.FLCOGroupVoiceChannelUser:
		var sizedBits [8]bit.Bit
		copy(sizedBits[:], infoBits[16:24])
		flc.ServiceOptions = *layer3Elements.NewServiceOptionsFromBits(sizedBits)

		for i := 24; i < 48; i++ {
			flc.GroupAddress <<= 1
			flc.GroupAddress |= int(infoBits[i])
		}

		for i := 48; i < 72; i++ {
			flc.SourceAddress <<= 1
			flc.SourceAddress |= int(infoBits[i])
		}

	case enums.FLCOGPSInfo:
		var sizedBits [3]bit.Bit
		copy(sizedBits[:], infoBits[20:23])
		flc.PositionError = *layer3Elements.NewPositionErrorFromBits(sizedBits)

		flc.Longitude = float32(360 / math.Pow(2, 25))
		longInt := 0
		for i := 23; i < 48; i++ {
			longInt <<= 1
			longInt |= int(infoBits[i])
		}
		flc.Longitude *= float32(longInt)

		flc.Latitude = float32(180 / math.Pow(2, 24))
		latInt := 0
		for i := 48; i < 72; i++ {
			latInt <<= 1
			latInt |= int(infoBits[i])
		}
		flc.Latitude *= float32(latInt)
	case enums.FLCOTalkerAliasHeader:
		var sizedBits [2]bit.Bit
		copy(sizedBits[:], infoBits[16:18])
		flc.TalkerAliasDataFormat = layer3Elements.NewTalkerAliasDataFormatFromBits(sizedBits)

		taLen := 0
		for i := 18; i < 24; i++ {
			taLen <<= 1
			taLen |= int(infoBits[i])
		}
		flc.TalkerAliasDataLength = taLen

		flc.TalkerAliasDataMSB = infoBits[23] == 1

		// Header provides up to 48 bits of alias data
		if taLen > 48 {
			taLen = 48
		}
		flc.TalkerAliasDataLen = taLen
		copy(flc.TalkerAliasData[:], infoBits[24:24+taLen])
	case enums.FLCOTalkerAliasBlock1, enums.FLCOTalkerAliasBlock2, enums.FLCOTalkerAliasBlock3:
		const blockLen = 56
		flc.TalkerAliasDataLen = blockLen
		copy(flc.TalkerAliasData[:], infoBits[16:72])
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
	case enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser:
		// Byte 2: Service Options
		data[2] = flc.ServiceOptions.ToByte()

		// Bytes 3-5: Destination
		var dst int
		if flc.FLCO == enums.FLCOGroupVoiceChannelUser {
			dst = flc.GroupAddress
		} else {
			dst = flc.TargetAddress
		}
		data[3] = byte(dst >> 16)
		data[4] = byte(dst >> 8)
		data[5] = byte(dst)

		// Bytes 6-8: Source Address
		src := flc.SourceAddress
		data[6] = byte(src >> 16)
		data[7] = byte(src >> 8)
		data[8] = byte(src)

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
