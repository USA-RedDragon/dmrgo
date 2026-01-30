package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
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
	EmbeddedSignallingData [32]byte

	IsData                bool
	Data                  elements.Data
	bitData               [264]bool
	deinterleavedInfoBits []byte

	PayloadCorrectedErrors int
	PayloadUncorrectable   bool
}

// NewBurstFromBytes creates a new Burst from the given bytes.
func NewBurstFromBytes(data [33]byte) *Burst {
	burst := &Burst{}
	burst.bitData = bytesToBits(data)

	burst.SyncPattern = extractSyncPattern(burst.bitData)
	burst.IsData = isDataSync(burst.SyncPattern)
	burst.VoiceBurst, burst.HasEmbeddedSignalling = classifyVoice(burst.SyncPattern)

	if burst.HasEmbeddedSignalling {
		burst.EmbeddedSignalling, burst.EmbeddedSignallingData = parseEmbedded(burst.bitData)
	}

	burst.HasSlotType = burst.IsData
	if burst.HasSlotType {
		burst.SlotType = parseSlotType(burst.bitData)
	}

	if !burst.IsData {
		burst.VoiceData = parseVoiceBits(burst.bitData)
		return burst
	}

	bBits := extractDataBits(burst.bitData)
	burst.deinterleavedInfoBits, burst.PayloadCorrectedErrors, burst.PayloadUncorrectable = burst.deinterleave(bBits, burst.SlotType.DataType)
	burst.Data = burst.extractData()

	return burst
}

func bytesToBits(data [33]byte) [264]bool {
	var bits [264]bool
	for i := 0; i < 264; i++ {
		bits[i] = (data[i/8] & (1 << (7 - (i % 8)))) != 0
	}
	return bits
}

func extractSyncPattern(bitData [264]bool) enums.SyncPattern {
	syncBytes := [6]byte{}
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			if bitData[108+(i*8)+j] {
				syncBytes[i] |= 1 << (7 - j)
			}
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

func parseEmbedded(bitData [264]bool) (pdu.EmbeddedSignalling, [32]byte) {
	embeddedBits := [16]byte{}
	for i := 0; i < 8; i++ {
		if bitData[108+i] {
			embeddedBits[i] = 1
		}
	}
	for i := 0; i < 8; i++ {
		if bitData[148+i] {
			embeddedBits[8+i] = 1
		}
	}

	embedded := pdu.NewEmbeddedSignallingFromBits(embeddedBits)
	var embeddedData [32]byte
	for i := 0; i < 32; i++ {
		if bitData[116+i] {
			embeddedData[i] = 1
		}
	}
	return embedded, embeddedData
}

func parseSlotType(bitData [264]bool) pdu.SlotType {
	slotBits := [20]byte{}
	for i := 0; i < 10; i++ {
		if bitData[98+i] {
			slotBits[i] = 1
		}
	}
	for i := 0; i < 10; i++ {
		if bitData[156+i] {
			slotBits[10+i] = 1
		}
	}
	return pdu.NewSlotTypeFromBits(slotBits)
}

func parseVoiceBits(bitData [264]bool) pdu.Vocoder {
	var voiceBits [216]byte
	for i := 0; i < 108; i++ {
		if bitData[i] {
			voiceBits[i] = 1
		}
	}
	for i := 0; i < 108; i++ {
		if bitData[156+i] {
			voiceBits[108+i] = 1
		}
	}
	return pdu.NewVocoderFromBits(voiceBits)
}

func extractDataBits(bitData [264]bool) [196]byte {
	var bits [196]byte
	for i := 0; i < 98; i++ {
		if bitData[i] {
			bits[i] = 1
		}
	}
	for i := 0; i < 98; i++ {
		if bitData[166+i] {
			bits[98+i] = 1
		}
	}
	return bits
}

func (b *Burst) deinterleave(bits [196]byte, dataType elements.DataType) ([]byte, int, bool) {
	switch dataType {
	case elements.DataTypeRate34:
		t := trellis34.New()
		decoded, errs := t.Decode(bits)
		return decoded[:], errs, false
	case elements.DataTypeRate1:
		var deinterleaved = make([]byte, 196)

		// Table B.10B: Transmit bit ordering for rate 1 coded data
		for i := 0; i < 96; i++ {
			deinterleaved[i] = bits[i]
		}
		for i := 0; i < 96; i++ {
			deinterleaved[96+i] = bits[100+i]
		}
		return deinterleaved, 0, false
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
		decoded, corrected, uncorrectable := bptc19696.DeinterleaveDataBits(bits)
		return decoded[:], corrected, uncorrectable
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
		ret += fmt.Sprintf("PayloadCorrected: %d, PayloadUncorrectable: %t, ", b.PayloadCorrectedErrors, b.PayloadUncorrectable)
		if b.Data != nil {
			ret += fmt.Sprintf("Data: %v, ", b.Data.ToString())
		}
	} else {
		ret += fmt.Sprintf("VoiceBurst: %v, Voice Data: %v, ", enums.VoiceBurstTypeToName(b.VoiceBurst), b.VoiceData.ToString())
	}

	ret += fmt.Sprintf("IsData: %v }", b.IsData)

	return ret
}

func (b *Burst) extractData() elements.Data {
	if !b.HasSlotType || b.SlotType.DataType == elements.DataTypeReserved {
		return nil
	}

	dt := b.SlotType.DataType
	switch dt {
	case elements.DataTypeCSBK:
		// TODO: implement CSBK parsing
		return nil
	case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
		return pdu.NewFullLinkControlFromBits(b.deinterleavedInfoBits, dt)
	case elements.DataTypePIHeader:
		// TODO: implement PI header parsing
		return nil
	case elements.DataTypeDataHeader:
		// TODO: implement data header parsing
		return nil
	case elements.DataTypeRate34:
		// TODO: implement rate 3/4 data parsing
		return nil
	case elements.DataTypeRate12:
		// TODO: implement rate 1/2 data parsing
		return nil
	case elements.DataTypeRate1:
		// TODO: implement rate 1 data parsing
		return nil
	case elements.DataTypeMBCHeader, elements.DataTypeMBCContinuation:
		// TODO: implement MBC parsing
		return nil
	case elements.DataTypeIdle:
		return nil
	case elements.DataTypeUnifiedSingleBlock:
		// TODO: implement unified single block parsing
		return nil
	case elements.DataTypeReserved:
		return nil
	default:
		return nil
	}
}

// Encode returns the encoded bytes of the burst.
func (b *Burst) Encode() [33]byte {
	var bitData [264]bool

	// Voice Data
	if b.VoiceBurst != enums.VoiceBurstUnknown || b.HasEmbeddedSignalling {
		voiceBits := b.VoiceData.Encode()
		for i := 0; i < 108; i++ {
			bitData[i] = voiceBits[i] == 1
		}
		for i := 0; i < 108; i++ {
			bitData[156+i] = voiceBits[108+i] == 1
		}
	}

	// Sync or Embedded Signalling
	if b.HasEmbeddedSignalling {
		esBits := b.EmbeddedSignalling.Encode()
		for i := 0; i < 8; i++ {
			bitData[108+i] = esBits[i] == 1
		}
		for i := 0; i < 32; i++ {
			bitData[116+i] = b.EmbeddedSignallingData[i] == 1
		}
		for i := 0; i < 8; i++ {
			bitData[148+i] = esBits[8+i] == 1
		}
	} else {
		// Encode Sync Pattern
		syncVal := int64(b.SyncPattern)
		for i := 0; i < 48; i++ {
			bitData[108+i] = ((syncVal >> (47 - i)) & 1) == 1
		}
	}

	// Helper to pack bits to bytes
	var data [33]byte
	for i := 0; i < 264; i++ {
		if bitData[i] {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}
