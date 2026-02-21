package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/bptc"
	trellis34 "github.com/USA-RedDragon/dmrgo/dmr/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

// Burst represents a DMR burst.
type Burst struct {
	SyncPattern enums.SyncPattern
	VoiceBurst  enums.VoiceBurstType

	VoiceData pdu.Vocoder

	HasSlotType bool
	SlotType    pdu.SlotType

	HasEmbeddedSignalling  bool
	EmbeddedSignalling     pdu.EmbeddedSignalling
	EmbeddedSignallingData [32]bit.Bit

	IsData                bool
	Data                  elements.Data
	FEC                   fec.BurstFECStats
	fullLinkControl       *pdu.FullLinkControl
	csbk                  *pdu.CSBK
	dataHeader            *pdu.DataHeader
	halfRateData          *pdu.Rate12Data
	threeQuarterRateData  *pdu.Rate34Data
	fullRateData          *pdu.Rate1Data
	bitData               [264]bit.Bit
	deinterleavedInfoBits [196]bit.Bit
	deinterleavedInfoLen  int
}

// NewBurstFromBytes creates a new Burst from the given bytes.
func NewBurstFromBytes(data [33]byte) (*Burst, error) {
	burst := &Burst{}
	err := burst.DecodeFromBytes(data)
	return burst, err
}

// DecodeFromBytes populates the burst in place, enabling zero-allocation decoding when reusing a Burst.
func (b *Burst) DecodeFromBytes(data [33]byte) error {
	*b = Burst{}
	b.bitData = bit.UnpackBytesToBits264(data)

	b.SyncPattern = extractSyncPattern(b.bitData)
	b.IsData = isDataSync(b.SyncPattern)
	b.VoiceBurst, b.HasEmbeddedSignalling = classifyVoice(b.SyncPattern)

	if b.HasEmbeddedSignalling {
		b.EmbeddedSignalling, b.EmbeddedSignallingData = parseEmbedded(b.bitData)
		b.FEC.EMB = b.EmbeddedSignalling.FEC
	}

	b.HasSlotType = b.IsData
	if b.HasSlotType {
		b.SlotType = parseSlotType(b.bitData)
		b.FEC.SlotType = b.SlotType.FEC
	}

	if !b.IsData {
		b.VoiceData = parseVoiceBits(b.bitData)
		b.FEC.Voice = b.VoiceData.FECResult()
		return nil
	}

	bBits := extractDataBits(b.bitData)
	b.deinterleavedInfoLen, b.FEC.Payload = b.deinterleave(bBits, b.SlotType.DataType)
	var err error
	b.Data, err = b.extractData()
	return err
}

func extractSyncPattern(bitData [264]bit.Bit) enums.SyncPattern {
	syncBytes := [6]byte{}
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			syncBytes[i] |= byte(bitData[108+(i*8)+j]) << (7 - j)
		}
	}
	return enums.SyncPatternFromBytes(syncBytes)
}

func isDataSync(sync enums.SyncPattern) bool {
	return sync == enums.Tdma1Data || sync == enums.Tdma2Data || sync == enums.MsSourcedData || sync == enums.BsSourcedData
}

func classifyVoice(sync enums.SyncPattern) (enums.VoiceBurstType, bool) {
	if sync == enums.Tdma2Voice || sync == enums.Tdma1Voice || sync == enums.MsSourcedVoice || sync == enums.BsSourcedVoice {
		return enums.VoiceBurstA, false
	}
	return enums.VoiceBurstUnknown, sync == enums.EmbeddedSignallingPattern
}

func parseEmbedded(bitData [264]bit.Bit) (pdu.EmbeddedSignalling, [32]bit.Bit) {
	var embeddedBits [16]bit.Bit
	copy(embeddedBits[:8], bitData[108:116])
	copy(embeddedBits[8:], bitData[148:156])

	embedded, _ := pdu.DecodeEmbeddedSignalling(embeddedBits)
	var embeddedData [32]bit.Bit
	copy(embeddedData[:], bitData[116:148])
	return embedded, embeddedData
}

func parseSlotType(bitData [264]bit.Bit) pdu.SlotType {
	var slotBits [20]bit.Bit
	copy(slotBits[:10], bitData[98:108])
	copy(slotBits[10:], bitData[156:166])
	st, _ := pdu.DecodeSlotType(slotBits)
	return st
}

func parseVoiceBits(bitData [264]bit.Bit) pdu.Vocoder {
	var voiceBits [216]bit.Bit
	copy(voiceBits[:108], bitData[:108])
	copy(voiceBits[108:], bitData[156:264])
	return pdu.NewVocoderFromBits(voiceBits)
}

func extractDataBits(bitData [264]bit.Bit) [196]bit.Bit {
	var bits [196]bit.Bit
	copy(bits[:98], bitData[:98])
	copy(bits[98:], bitData[166:264])
	return bits
}

func (b *Burst) deinterleave(bits [196]bit.Bit, dataType elements.DataType) (int, fec.FECResult) {
	switch dataType {
	case elements.DataTypeRate34:
		var t trellis34.Trellis34
		decoded, result := t.Decode(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), result
	case elements.DataTypeRate1:
		// Table B.10B: Transmit bit ordering for rate 1 coded data
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[i] = bits[i]
		}
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[96+i] = bits[100+i]
		}
		return 196, fec.FECResult{}
	case elements.DataTypePIHeader,
		elements.DataTypeVoiceLCHeader,
		elements.DataTypeTerminatorWithLC,
		elements.DataTypeCSBK,
		elements.DataTypeMBCHeader,
		elements.DataTypeMBCContinuation,
		elements.DataTypeDataHeader,
		elements.DataTypeRate12,
		elements.DataTypeIdle,
		elements.DataTypeUnifiedSingleBlock:
		bptc19696 := bptc.BPTC19696{}
		decoded, result := bptc19696.DeinterleaveDataBits(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), result
	case elements.DataTypeReserved:
		panic(fmt.Sprintf("Unknown data type %v", dataType))
	default:
		panic(fmt.Sprintf("Unhandled data type %v", dataType))
	}
}

// ToString returns a string representation of the burst.
func (b *Burst) ToString() string {
	ret := fmt.Sprintf("{ SyncPattern: %s", enums.SyncPatternToName(b.SyncPattern))
	if b.HasEmbeddedSignalling {
		ret += fmt.Sprintf("EmbeddedSignalling: %v, ", b.EmbeddedSignalling.ToString())
	}
	if b.HasSlotType {
		ret += fmt.Sprintf("SlotType: %v, ", b.SlotType.ToString())
	}
	if b.IsData {
		agg := b.FEC.Aggregate()
		ret += fmt.Sprintf("FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t}, ", agg.BitsChecked, agg.ErrorsCorrected, agg.Uncorrectable)
		if b.Data != nil {
			ret += fmt.Sprintf("Data: %v, ", b.Data.ToString())
		}
	} else {
		ret += fmt.Sprintf("VoiceBurst: %v, Voice Data: %v, ", enums.VoiceBurstTypeToName(b.VoiceBurst), b.VoiceData.ToString())
	}

	ret += fmt.Sprintf("IsData: %v }", b.IsData)

	return ret
}

func (b *Burst) extractData() (elements.Data, error) {
	if !b.HasSlotType || b.SlotType.DataType == elements.DataTypeReserved {
		return nil, fmt.Errorf("burst does not have valid slot type for data extraction")
	}

	dt := b.SlotType.DataType
	infoBits := b.deinterleavedInfoBits[:b.deinterleavedInfoLen]
	switch dt {
	case elements.DataTypeCSBK:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeCSBK(sizedBits)
		decoded.DataType = dt
		b.csbk = &decoded
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode CSBK from bits")
		}
		return b.csbk, nil
	case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeFullLinkControl(sizedBits)
		decoded.DataType = dt
		b.fullLinkControl = &decoded
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode full link control from bits")
		}
		return b.fullLinkControl, nil
	case elements.DataTypePIHeader:
		// TODO: implement PI header parsing
		return nil, nil
	case elements.DataTypeDataHeader:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeDataHeader(sizedBits)
		decoded.DataType = dt
		b.dataHeader = &decoded
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode data header from bits: %v", b.dataHeader.ToString())
		}
		return b.dataHeader, nil
	case elements.DataTypeRate34:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		rt, _ := pdu.DecodeRate34Data(sizedBits)
		rt.DataType = dt
		b.threeQuarterRateData = &rt
		return b.threeQuarterRateData, nil
	case elements.DataTypeRate12:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		rt, _ := pdu.DecodeRate12Data(sizedBits)
		rt.DataType = dt
		b.halfRateData = &rt
		return b.halfRateData, nil
	case elements.DataTypeRate1:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		rt, _ := pdu.DecodeRate1Data(sizedBits)
		rt.DataType = dt
		b.fullRateData = &rt
		return b.fullRateData, nil
	case elements.DataTypeMBCHeader, elements.DataTypeMBCContinuation:
		// TODO: implement MBC parsing
		return nil, fmt.Errorf("todo: MBC parsing not implemented")
	case elements.DataTypeIdle:
		return nil, nil
	case elements.DataTypeUnifiedSingleBlock:
		// TODO: implement unified single block parsing
		return nil, fmt.Errorf("todo: unified single block parsing not implemented")
	case elements.DataTypeReserved:
		return nil, fmt.Errorf("todo: reserved data type parsing not implemented")
	default:
		return nil, fmt.Errorf("todo: unhandled data type parsing not implemented")
	}
}

// Encode returns the encoded bytes of the burst.
func (b *Burst) Encode() [33]byte {
	var bitData [264]bit.Bit

	if b.IsData {
		// Encode data payload
		dataBits := b.encodeDataBits()
		copy(bitData[:98], dataBits[:98])
		copy(bitData[166:264], dataBits[98:196])

		// Encode slot type
		if b.HasSlotType {
			slotBits := pdu.EncodeSlotType(&b.SlotType)
			copy(bitData[98:108], slotBits[:10])
			copy(bitData[156:166], slotBits[10:20])
		}
	} else if b.VoiceBurst != enums.VoiceBurstUnknown || b.HasEmbeddedSignalling {
		// Voice Data
		voiceBits := b.VoiceData.Encode()
		copy(bitData[:108], voiceBits[:108])
		copy(bitData[156:264], voiceBits[108:216])
	}

	// Sync or Embedded Signalling
	if b.HasEmbeddedSignalling {
		esBits := pdu.EncodeEmbeddedSignalling(&b.EmbeddedSignalling)
		copy(bitData[108:116], esBits[0:8])
		copy(bitData[116:148], b.EmbeddedSignallingData[:])
		copy(bitData[148:156], esBits[8:16])
	} else {
		// Encode Sync Pattern
		syncVal := int64(b.SyncPattern)
		for i := 0; i < 48; i++ {
			bitData[108+i] = bit.Bit((syncVal >> (47 - i)) & 1)
		}
	}

	return bit.PackBits264(bitData)
}

func (b *Burst) encodeDataBits() [196]bit.Bit {
	switch b.SlotType.DataType {
	case elements.DataTypeRate34:
		var t trellis34.Trellis34
		var data [144]bit.Bit
		copy(data[:], b.deinterleavedInfoBits[:144])
		return t.Encode(data)
	case elements.DataTypeRate1:
		var bits [196]bit.Bit
		for i := 0; i < 96; i++ {
			bits[i] = b.deinterleavedInfoBits[i]
		}
		// bits[96..99] are reserved (zero)
		for i := 0; i < 96; i++ {
			bits[100+i] = b.deinterleavedInfoBits[96+i]
		}
		return bits
	default:
		// BPTC(196,96) types
		var infoBits [96]bit.Bit
		copy(infoBits[:], b.deinterleavedInfoBits[:96])
		return bptc.Encode(infoBits)
	}
}

// PackEmbeddedSignallingData converts the 32-bit (unpacked) embedded signalling
// data into a 4-byte packed array.
func (b *Burst) PackEmbeddedSignallingData() [4]byte {
	var data [4]byte
	for i := 0; i < 32; i++ {
		data[i/8] |= byte(b.EmbeddedSignallingData[i]) << (7 - (i % 8))
	}
	return data
}

// UnpackEmbeddedSignallingData populates the 32-bit (unpacked) embedded signalling
// data from a byte slice. Only best-effort unpacking is performed up to 32 bits.
func (b *Burst) UnpackEmbeddedSignallingData(data []byte) {
	// Clear existing
	b.EmbeddedSignallingData = [32]bit.Bit{}

	if len(data) == 0 {
		return
	}

	for i := 0; i < 32 && i < len(data)*8; i++ {
		byteIdx := i / 8
		bitIdx := 7 - (i % 8)
		if byteIdx < len(data) && (data[byteIdx]>>bitIdx)&1 == 1 {
			b.EmbeddedSignallingData[i] = 1
		}
	}
}

// BuildLCDataBurst builds a 33-byte LC data burst (e.g. for Voice Header or Terminator).
func BuildLCDataBurst(lcBytes [12]byte, dataType elements.DataType, colorCode uint8) [33]byte {
	// Convert LC bytes to 96 info bits
	var infoBits [96]bit.Bit
	for i := 0; i < 12; i++ {
		for j := 0; j < 8; j++ {
			if (lcBytes[i]>>(7-j))&1 == 1 {
				infoBits[i*8+j] = 1
			}
		}
	}

	// BPTC(196,96) encode
	encoded := bptc.Encode(infoBits)

	// Build the 264-bit burst:
	// bits[0:97]   — first 98 data bits
	// bits[98:107]  — slot type (first 10 bits)
	// bits[108:155] — SYNC pattern (48 bits)
	// bits[156:165] — slot type (last 10 bits)
	// bits[166:263] — last 98 data bits
	var bitData [264]bit.Bit

	// Data part 1: encoded[0:97] → bits[0:97]
	copy(bitData[:98], encoded[:98])

	// Data part 2: encoded[98:195] → bits[166:263]
	copy(bitData[166:264], encoded[98:196])

	// Slot Type: encode color code + data type with Golay FEC
	slotTypeBits := pdu.EncodeSlotType(&pdu.SlotType{
		ColorCode: int(colorCode & 0xF),
		DataType:  dataType,
	})

	copy(bitData[98:108], slotTypeBits[:10])
	copy(bitData[156:166], slotTypeBits[10:20])

	// SYNC pattern: use BS-sourced data or MS-sourced data
	var syncPattern enums.SyncPattern
	switch dataType {
	case elements.DataTypeVoiceLCHeader:
		syncPattern = enums.BsSourcedData
	case elements.DataTypeTerminatorWithLC:
		syncPattern = enums.BsSourcedData
	case elements.DataTypePIHeader,
		elements.DataTypeCSBK,
		elements.DataTypeMBCHeader,
		elements.DataTypeMBCContinuation,
		elements.DataTypeDataHeader,
		elements.DataTypeRate12,
		elements.DataTypeRate34,
		elements.DataTypeIdle,
		elements.DataTypeRate1,
		elements.DataTypeUnifiedSingleBlock,
		elements.DataTypeReserved:
		syncPattern = enums.BsSourcedData
	}

	syncVal := int64(syncPattern)
	for i := 0; i < 48; i++ {
		bitData[108+i] = bit.Bit((syncVal >> (47 - i)) & 1)
	}

	return bit.PackBits264(bitData)
}
