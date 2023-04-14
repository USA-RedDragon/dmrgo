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

	HasEmbeddedSignalling bool
	EmbeddedSignalling    pdu.EmbeddedSignalling

	IsData                bool
	Data                  elements.Data
	bitData               [264]bool
	deinterleavedInfoBits []byte
}

// NewBurstFromBytes creates a new Burst from the given bytes.
func NewBurstFromBytes(data [33]byte) *Burst {
	burst := &Burst{}

	burst.bitData = [264]bool{}
	for i := 0; i < 264; i++ {
		burst.bitData[i] = (data[i/8] & (1 << (7 - (i % 8)))) != 0
	}

	// set syncOrEmbeddedSignalling from bits 108-156 in data
	syncOrEmbeddedSignalling := [6]byte{}
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			if burst.bitData[108+(i*8)+j] {
				syncOrEmbeddedSignalling[i] |= 1 << (7 - j)
			}
		}
	}

	burst.SyncPattern = enums.SyncPatternFromBytes(syncOrEmbeddedSignalling)

	burst.IsData = burst.SyncPattern == enums.Tdma1Data || burst.SyncPattern == enums.Tdma2Data || burst.SyncPattern == enums.MsSourcedData || burst.SyncPattern == enums.BsSourcedData

	isVoiceSuperFrameStart := false
	burst.VoiceBurst = enums.VoiceBurstUnknown

	if burst.SyncPattern == enums.Tdma2Voice || burst.SyncPattern == enums.Tdma1Voice || burst.SyncPattern == enums.MsSourcedVoice || burst.SyncPattern == enums.BsSourcedVoice {
		isVoiceSuperFrameStart = true
		burst.VoiceBurst = enums.VoiceBurstA
	}

	burst.HasEmbeddedSignalling = burst.SyncPattern == enums.EmbeddedSignallingPattern && !isVoiceSuperFrameStart
	if burst.HasEmbeddedSignalling {
		embeddedSignallingBits := [16]byte{}
		for i := 0; i < 8; i++ {
			if burst.bitData[108+i] {
				embeddedSignallingBits[i] = 1
			} else {
				embeddedSignallingBits[i] = 0
			}
		}
		for i := 0; i < 8; i++ {
			if burst.bitData[148+i] {
				embeddedSignallingBits[8+i] = 1
			} else {
				embeddedSignallingBits[8+i] = 0
			}
		}
		burst.EmbeddedSignalling = pdu.NewEmbeddedSignallingFromBits(embeddedSignallingBits)
	}

	burst.HasSlotType = burst.IsData
	if burst.HasSlotType {
		slotTypeBits := [20]byte{}
		for i := 0; i < 10; i++ {
			if burst.bitData[98+i] {
				slotTypeBits[i] = 1
			} else {
				slotTypeBits[i] = 0
			}
		}
		for i := 0; i < 10; i++ {
			if burst.bitData[156+i] {
				slotTypeBits[10+i] = 1
			} else {
				slotTypeBits[10+i] = 0
			}
		}
		burst.SlotType = pdu.NewSlotTypeFromBits(slotTypeBits)
	}

	if !burst.IsData {
		var voiceBits [216]byte
		for i := 0; i < 108; i++ {
			if burst.bitData[i] {
				voiceBits[i] = 1
			} else {
				voiceBits[i] = 0
			}
		}
		for i := 0; i < 108; i++ {
			if burst.bitData[156+i] {
				voiceBits[108+i] = 1
			} else {
				voiceBits[108+i] = 0
			}
		}
		burst.VoiceData = pdu.NewVocoderFromBits(voiceBits)
	} else {
		var bits [196]byte
		for i := 0; i < 98; i++ {
			if burst.bitData[i] {
				bits[i] = 1
			} else {
				bits[i] = 0
			}
		}
		for i := 0; i < 98; i++ {
			if burst.bitData[166+i] {
				bits[98+i] = 1
			} else {
				bits[98+i] = 0
			}
		}
		burst.deinterleavedInfoBits = burst.deinterleave(bits, burst.SlotType.DataType)
		burst.Data = burst.extractData()
	}

	//     # variables not standardized in ETSI, used for various DMR protocols processing
	//     self.timeslot: int = 1
	//     self.sequence_no: int = 0
	//     self.stream_no: bytes = bytes(4)
	//     self.transmission_type: TransmissionTypes = TransmissionTypes.Idle

	return burst
}

func (b *Burst) deinterleave(bits [196]byte, dataType elements.DataType) []byte {
	switch dataType {
	case elements.DataTypeRate34:
		t := trellis34.New()
		decoded := t.Decode(bits)
		return decoded[:]
	case elements.DataTypeRate1:
		var deinterleaved []byte = make([]byte, 196)

		// Table B.10B: Transmit bit ordering for rate 1 coded data
		for i := 0; i < 96; i++ {
			deinterleaved[i] = bits[i]
		}
		for i := 0; i < 96; i++ {
			deinterleaved[96+i] = bits[100+i]
		}
		return deinterleaved
	case elements.DataTypeReserved:
		panic(fmt.Sprintf("Unknown data type %v", dataType))
	default:
		// here expected are: rate 1/2, PI header, voice headeader/terminator, csbk, data header, idle message,
		// response header/data blocks, mbc header/continuation/last block, udt header/continuation/last block
		// unified single block data and more
		// See section B.0 table B.1, FEC and CRC summary, ETSI TS 102 361-1 V2.5.1 (2017-10)
		bptc19696 := bptc.BPTC19696{}
		decoded := bptc19696.DeinterleaveDataBits(bits)
		return decoded[:]
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
	if b.SlotType.DataType == elements.DataTypeCSBK {
		fmt.Println("CSBK")
		// return pdu.NewCSBKFromBits(b.deinterleavedInfoBits)
	} else if b.SlotType.DataType == elements.DataTypeVoiceLCHeader {
		return pdu.NewFullLinkControlFromBits(b.deinterleavedInfoBits, b.SlotType.DataType)
	} else if b.SlotType.DataType == elements.DataTypePIHeader {
		fmt.Println("PI")
		// return pdu.NewPIHeaderFromBits(b.deinterleavedInfoBits)
	} else if b.SlotType.DataType == elements.DataTypeTerminatorWithLC {
		return pdu.NewFullLinkControlFromBits(b.deinterleavedInfoBits, b.SlotType.DataType)
	} else if b.SlotType.DataType == elements.DataTypeDataHeader {
		fmt.Println("Data Header")
		// return pdu.NewDataHeaderFromBits(b.deinterleavedInfoBits)
	} else if b.SlotType.DataType == elements.DataTypeRate34 {
		fmt.Println("Data Rate 3/4")
		// return pdu.NewRate34DataFromBits(b.deinterleavedInfoBits)
	} else if b.SlotType.DataType == elements.DataTypeRate12 {
		fmt.Println("Data Rate 1/2")
		// return pdu.NewRate12DataFromBits(b.deinterleavedInfoBits)
	} else if b.SlotType.DataType == elements.DataTypeRate1 {
		fmt.Println("Data Rate 1")
		// return pdu.NewRate1DataFromBits(b.deinterleavedInfoBits)
	}

	return nil
}
