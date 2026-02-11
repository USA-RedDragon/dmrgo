package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/enums"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/dmr/fec/golay"
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
	fullLinkControl       *pdu.FullLinkControl
	csbk                  *pdu.CSBK
	dataHeader            *pdu.DataHeader
	halfRateData          *pdu.Rate12Data
	bitData               [264]bool
	deinterleavedInfoBits [196]byte
	deinterleavedInfoLen  int

	PayloadCorrectedErrors int
	PayloadUncorrectable   bool
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
	b.bitData = bytesToBits(data)

	b.SyncPattern = extractSyncPattern(b.bitData)
	b.IsData = isDataSync(b.SyncPattern)
	b.VoiceBurst, b.HasEmbeddedSignalling = classifyVoice(b.SyncPattern)

	if b.HasEmbeddedSignalling {
		b.EmbeddedSignalling, b.EmbeddedSignallingData = parseEmbedded(b.bitData)
	}

	b.HasSlotType = b.IsData
	if b.HasSlotType {
		b.SlotType = parseSlotType(b.bitData)
	}

	if !b.IsData {
		b.VoiceData = parseVoiceBits(b.bitData)
		return nil
	}

	bBits := extractDataBits(b.bitData)
	b.deinterleavedInfoLen, b.PayloadCorrectedErrors, b.PayloadUncorrectable = b.deinterleave(bBits, b.SlotType.DataType)
	var err error
	b.Data, err = b.extractData()
	return err
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

func (b *Burst) deinterleave(bits [196]byte, dataType elements.DataType) (int, int, bool) {
	switch dataType {
	case elements.DataTypeRate34:
		var t trellis34.Trellis34
		decoded, errs := t.Decode(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), errs, false
	case elements.DataTypeRate1:
		// Table B.10B: Transmit bit ordering for rate 1 coded data
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[i] = bits[i]
		}
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[96+i] = bits[100+i]
		}
		return 196, 0, false
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
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), corrected, uncorrectable
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

func (b *Burst) extractData() (elements.Data, error) {
	if !b.HasSlotType || b.SlotType.DataType == elements.DataTypeReserved {
		return nil, fmt.Errorf("burst does not have valid slot type for data extraction")
	}

	dt := b.SlotType.DataType
	infoBits := b.deinterleavedInfoBits[:b.deinterleavedInfoLen]
	switch dt {
	case elements.DataTypeCSBK:
		b.csbk = &pdu.CSBK{}
		if b.csbk.DecodeFromBits(infoBits, dt) {
			return b.csbk, nil
		}
		return nil, fmt.Errorf("failed to decode CSBK from bits")
	case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
		b.fullLinkControl = &pdu.FullLinkControl{}
		if b.fullLinkControl.DecodeFromBits(infoBits, dt) {
			return b.fullLinkControl, nil
		}
		return nil, fmt.Errorf("failed to decode full link control from bits")
	case elements.DataTypePIHeader:
		// TODO: implement PI header parsing
		return nil, nil
	case elements.DataTypeDataHeader:
		b.dataHeader = &pdu.DataHeader{}
		if b.dataHeader.DecodeFromBits(infoBits, dt) {
			return b.dataHeader, nil
		}
		return nil, fmt.Errorf("failed to decode data header from bits")
	case elements.DataTypeRate34:
		// TODO: implement rate 3/4 data parsing
		return nil, fmt.Errorf("todo: rate 3/4 data parsing not implemented")
	case elements.DataTypeRate12:
		b.halfRateData = &pdu.Rate12Data{}
		if b.halfRateData.DecodeFromBits(infoBits, dt) {
			return b.halfRateData, nil
		}
		return nil, fmt.Errorf("failed to decode rate 1/2 data from bits")
	case elements.DataTypeRate1:
		// TODO: implement rate 1 data parsing
		return nil, fmt.Errorf("todo: rate 1 data parsing not implemented")
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

// PackEmbeddedSignallingData converts the 32-bit (unpacked) embedded signalling
// data into a 4-byte packed array.
func (b *Burst) PackEmbeddedSignallingData() [4]byte {
	var data [4]byte
	for i := 0; i < 32; i++ {
		if b.EmbeddedSignallingData[i] == 1 {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}
	return data
}

// UnpackEmbeddedSignallingData populates the 32-bit (unpacked) embedded signalling
// data from a byte slice. Only best-effort unpacking is performed up to 32 bits.
func (b *Burst) UnpackEmbeddedSignallingData(data []byte) {
	// Clear existing
	b.EmbeddedSignallingData = [32]byte{}

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
	var infoBits [96]byte
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
	var bitData [264]bool

	// Data part 1: encoded[0:97] → bits[0:97]
	for i := 0; i < 98; i++ {
		bitData[i] = encoded[i] == 1
	}

	// Data part 2: encoded[98:195] → bits[166:263]
	for i := 0; i < 98; i++ {
		bitData[166+i] = encoded[98+i] == 1
	}

	// Slot Type: encode color code (0) + data type
	inputByte := colorCode&0xF<<4 | byte(dataType&0xF)
	slotTypeBits := golay.Encode(inputByte)

	for i := 0; i < 10; i++ {
		bitData[98+i] = slotTypeBits[i] == 1
	}
	for i := 0; i < 10; i++ {
		bitData[156+i] = slotTypeBits[10+i] == 1
	}

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
		bitData[108+i] = ((syncVal >> (47 - i)) & 1) == 1
	}

	// Pack bits to bytes
	var data [33]byte
	for i := 0; i < 264; i++ {
		if bitData[i] {
			data[i/8] |= 1 << (7 - (i % 8))
		}
	}

	return data
}
