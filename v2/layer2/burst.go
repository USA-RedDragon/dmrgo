package layer2

import (
	"fmt"
	"reflect"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Burst represents a DMR burst.
type Burst struct {
	SyncPattern enums.SyncPattern
	// SyncDistance is the Hamming distance between the received SYNC field
	// and the nearest Table 9.2 pattern. It is at most
	// enums.SyncMatchThreshold when SyncPattern is a SYNC pattern.
	SyncDistance int
	// VoiceBurst is the superframe position of a voice burst. Decoding is
	// stateless per burst, so DecodeFromBytes sets VoiceBurstA for a burst
	// with a voice SYNC pattern and VoiceBurstUnknown for every other burst,
	// bursts B to F included. VoiceReceiver sets the position of bursts B to
	// F from the superframe sequence. Encode ignores it.
	VoiceBurst enums.VoiceBurstType

	// VoiceData holds the three voice slots decoded as AMBE+2 frames. Encode
	// writes a slot as received while the DecodedBits of its frame are
	// unchanged, and codes the frame with vocoder.EncodeAMBE72 otherwise; see
	// Encode.
	VoiceData pdu.Vocoder

	HasSlotType bool
	SlotType    pdu.SlotType

	HasEmbeddedSignalling  bool
	EmbeddedSignalling     pdu.EmbeddedSignalling
	EmbeddedSignallingData [32]bit.Bit

	HasReverseChannel bool
	ReverseChannel    *pdu.ReverseChannel

	IsData bool
	// Data is the decoded PDU of a data burst, nil when its decode failed.
	// Encode codes Data when it is set, except the PR FILL that
	// DecodeFromBytes returns for an idle burst; see Encode.
	Data                  elements.Data
	FEC                   fec.BurstFECStats
	TrunkingMode          bool
	bitData               [264]bit.Bit
	decodedData           elements.Data
	decodedDataType       elements.DataType
	decodedVoice          pdu.Vocoder
	hasVoiceSlots         bool
	deinterleavedInfoBits [196]bit.Bit
	deinterleavedInfoLen  int
}

// SetTrunkingMode sets whether the burst should be decoded as Tier III trunking.
// This affects disambiguation of CSBK opcode 0x38 (BS_Dwn_Act vs TD_GRANT_MI).
func (b *Burst) SetTrunkingMode(mode bool) {
	b.TrunkingMode = mode
}

// NewBurstFromBytes creates a new Burst from the given bytes.
func NewBurstFromBytes(data [33]byte) (*Burst, error) {
	burst := &Burst{}
	err := burst.DecodeFromBytes(data)
	return burst, err
}

// DecodeFromBytes populates the burst in place, enabling zero-allocation decoding when reusing a Burst.
func (b *Burst) DecodeFromBytes(data [33]byte) error {
	trunkingMode := b.TrunkingMode
	*b = Burst{}
	b.TrunkingMode = trunkingMode
	b.bitData = bit.UnpackBytesToBits264(data)

	b.SyncPattern, b.SyncDistance = extractSyncPattern(b.bitData)
	b.IsData = isDataSync(b.SyncPattern)
	b.VoiceBurst, b.HasEmbeddedSignalling = classifyVoice(b.SyncPattern)

	if b.SyncPattern == enums.MsSourcedRcSync {
		b.decodeStandaloneRC()
		return nil
	}

	if b.HasEmbeddedSignalling {
		b.EmbeddedSignalling, b.EmbeddedSignallingData = parseEmbedded(b.bitData)
		b.FEC.EMB = b.EmbeddedSignalling.FEC

		// §6.4: Reverse Channel detection — PI=1 and LCSS=SingleFragment
		// means the 32-bit embedded data carries an RC PDU, not an LC fragment.
		if b.EmbeddedSignalling.PreemptionAndPowerControlIndicator &&
			b.EmbeddedSignalling.LCSS == enums.SingleFragmentLCorCSBK {
			rc, rcFEC := DecodeRCFromEmbeddedData(b.EmbeddedSignallingData)
			b.HasReverseChannel = true
			b.ReverseChannel = &rc
			b.FEC.RC = rcFEC
		}
	}

	b.HasSlotType = b.IsData
	if b.HasSlotType {
		b.SlotType = parseSlotType(b.bitData)
		b.FEC.SlotType = b.SlotType.FEC
	}

	if !b.IsData {
		b.VoiceData = parseVoiceBits(b.bitData)
		b.FEC.Voice = b.VoiceData.FECResult()
		b.decodedVoice = b.VoiceData
		b.hasVoiceSlots = true
		return nil
	}

	bBits := extractDataBits(b.bitData)
	b.deinterleavedInfoLen, b.FEC.Payload = b.deinterleave(bBits, b.SlotType.DataType)
	var err error
	b.Data, err = b.extractData()
	b.decodedData = b.Data
	b.decodedDataType = b.SlotType.DataType
	return err
}

// extractSyncPattern classifies the 48-bit centre of the burst against the
// Table 9.2 SYNC patterns.
func extractSyncPattern(bitData [264]bit.Bit) (enums.SyncPattern, int) {
	syncBytes := [6]byte{}
	for i := 0; i < 6; i++ {
		for j := 0; j < 8; j++ {
			syncBytes[i] |= byte(bitData[108+(i*8)+j]) << (7 - j) //nolint:gosec // max index: 108+5*8+7=155 < 264
		}
	}
	return enums.SyncPatternFromBytesWithDistance(syncBytes)
}

func isDataSync(sync enums.SyncPattern) bool {
	return sync == enums.Tdma1Data || sync == enums.Tdma2Data || sync == enums.MsSourcedData || sync == enums.BsSourcedData
}

func isVoiceSync(sync enums.SyncPattern) bool {
	return sync == enums.Tdma1Voice || sync == enums.Tdma2Voice || sync == enums.MsSourcedVoice || sync == enums.BsSourcedVoice
}

func isSyncPattern(sync enums.SyncPattern) bool {
	return isDataSync(sync) || isVoiceSync(sync) || sync == enums.MsSourcedRcSync || sync == enums.Reserved
}

func classifyVoice(sync enums.SyncPattern) (enums.VoiceBurstType, bool) {
	if isVoiceSync(sync) {
		return enums.VoiceBurstA, false
	}
	return enums.VoiceBurstUnknown, sync == enums.EmbeddedSignallingPattern || sync == enums.MsSourcedRcSync
}

// decodeStandaloneRC decodes a §6.4.1 standalone inbound RC burst (Figure
// 6.7, Table E.12): EMB(8) RC(16) SYNC(48) RC(16) EMB(8) around the burst
// centre. The RC SYNC stays in place and the burst carries no voice.
// An RC burst always carries an RC PDU, so PI does not gate RC decoding.
func (b *Burst) decodeStandaloneRC() {
	var embBits [16]bit.Bit
	copy(embBits[:8], b.bitData[RCEMBFirstStart:RCEMBFirstEnd])
	copy(embBits[8:], b.bitData[RCEMBSecondStart:RCEMBSecondEnd])
	b.EmbeddedSignalling, b.FEC.EMB = pdu.DecodeEmbeddedSignalling(embBits)

	copy(b.EmbeddedSignallingData[:16], b.bitData[RCDataFirstStart:RCDataFirstEnd])
	copy(b.EmbeddedSignallingData[16:], b.bitData[RCDataSecondStart:RCDataSecondEnd])
	rc, rcFEC := DecodeRCFromEmbeddedData(b.EmbeddedSignallingData)
	b.HasReverseChannel = true
	b.ReverseChannel = &rc
	b.FEC.RC = rcFEC
}

func (b *Burst) encodeStandaloneRC() ([264]bit.Bit, error) {
	var bitData [264]bit.Bit
	data, err := b.embeddedData()
	if err != nil {
		return bitData, err
	}
	embBits, err := pdu.EncodeEmbeddedSignalling(&b.EmbeddedSignalling)
	if err != nil {
		return bitData, fmt.Errorf("cannot encode embedded signalling: %w", err)
	}
	copy(bitData[RCEMBFirstStart:RCEMBFirstEnd], embBits[:8])
	copy(bitData[RCDataFirstStart:RCDataFirstEnd], data[:16])
	writeSync(&bitData, enums.MsSourcedRcSync)
	copy(bitData[RCDataSecondStart:RCDataSecondEnd], data[16:])
	copy(bitData[RCEMBSecondStart:RCEMBSecondEnd], embBits[8:])
	return bitData, nil
}

// embeddedData returns the 32 bits of the embedded signalling data field:
// the coded ReverseChannel when HasReverseChannel is set, and
// EmbeddedSignallingData otherwise.
func (b *Burst) embeddedData() ([32]bit.Bit, error) {
	if !b.HasReverseChannel {
		return b.EmbeddedSignallingData, nil
	}
	if b.ReverseChannel == nil {
		return [32]bit.Bit{}, fmt.Errorf("HasReverseChannel is set but ReverseChannel is nil")
	}
	rcBits, err := EncodeRCToEmbeddedData(b.ReverseChannel)
	if err != nil {
		return [32]bit.Bit{}, fmt.Errorf("cannot encode reverse channel: %w", err)
	}
	return rcBits, nil
}

func writeSync(bitData *[264]bit.Bit, sync enums.SyncPattern) {
	syncVal := int64(sync)
	for i := 0; i < 48; i++ {
		bitData[SyncStart+i] = bit.Bit((syncVal >> (47 - i)) & 1)
	}
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
	decoded, _ := pdu.DecodeVocoder(voiceBits)
	return decoded
}

func extractDataBits(bitData [264]bit.Bit) [196]bit.Bit {
	var bits [196]bit.Bit
	copy(bits[:98], bitData[:98])
	copy(bits[98:], bitData[166:264])
	return bits
}

func (b *Burst) deinterleave(bits [196]bit.Bit, dataType elements.DataType) (int, fec.Result) {
	switch dataType {
	case elements.DataTypeRate34:
		var t trellis.Trellis34
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
		return 192, fec.Result{}
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
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
	default:
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
	}
}

// isBPTCDataType reports whether bursts of dataType carry 96 information
// bits coded with BPTC(196,96).
func isBPTCDataType(dataType elements.DataType) bool {
	switch dataType {
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
		return true
	case elements.DataTypeRate34, elements.DataTypeRate1, elements.DataTypeReserved:
		return false
	default:
		return false
	}
}

// BPTCPayload returns the 96 information bits of a data burst coded with
// BPTC(196,96), packed most significant bit first, as DecodeFromBytes
// corrected them before any CRC or RS(12,9) check, and the BPTC FEC result,
// which is also FEC.Payload. ok is false for a voice burst, for the Rate 3/4,
// Rate 1 and reserved data types, which are not BPTC coded, and for a burst
// that DecodeFromBytes did not decode.
func (b *Burst) BPTCPayload() (payload [12]byte, result fec.Result, ok bool) {
	if !b.IsData || !isBPTCDataType(b.SlotType.DataType) || b.deinterleavedInfoLen != 96 {
		return payload, result, false
	}
	copy(payload[:], bit.PackBits(b.deinterleavedInfoBits[:96]))
	return payload, b.FEC.Payload, true
}

// ToString returns a string representation of the burst.
func (b *Burst) ToString() string {
	ret := fmt.Sprintf("{ SyncPattern: %s", enums.SyncPatternToName(b.SyncPattern))
	if b.HasEmbeddedSignalling {
		ret += fmt.Sprintf("EmbeddedSignalling: %v, ", b.EmbeddedSignalling.ToString())
		if b.HasReverseChannel && b.ReverseChannel != nil {
			ret += fmt.Sprintf("ReverseChannel: %v, ", b.ReverseChannel.ToString())
		}
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
	} else if b.SyncPattern != enums.MsSourcedRcSync {
		ret += fmt.Sprintf("VoiceBurst: %v, Voice Data: %v, ", enums.VoiceBurstTypeToName(b.VoiceBurst), b.VoiceData.ToString())
	}

	ret += fmt.Sprintf("IsData: %v }", b.IsData)

	return ret
}

func (b *Burst) extractData() (elements.Data, error) {
	if !b.HasSlotType {
		return nil, fmt.Errorf("burst does not have valid slot type for data extraction")
	}
	if b.SlotType.DataType.IsReserved() {
		return nil, fmt.Errorf("reserved data type %d", b.SlotType.DataType)
	}

	dt := b.SlotType.DataType
	infoBits := b.deinterleavedInfoBits[:b.deinterleavedInfoLen]
	switch dt {
	case elements.DataTypeCSBK:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeCSBK(sizedBits, b.TrunkingMode)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode CSBK from bits")
		}
		return &decoded, nil
	case elements.DataTypeVoiceLCHeader, elements.DataTypeTerminatorWithLC:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeFullLinkControl(sizedBits, dt)
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode full link control from bits")
		}
		return &decoded, nil
	case elements.DataTypePIHeader:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodePIHeader(sizedBits)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode PI header from bits")
		}
		return &decoded, nil
	case elements.DataTypeDataHeader:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeDataHeader(sizedBits)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode data header from bits: %v", decoded.ToString())
		}
		return &decoded, nil
	case elements.DataTypeRate34:
		var sizedBits [144]bit.Bit
		copy(sizedBits[:], infoBits[:144])
		rt, fecResult := pdu.DecodeRate34Data(sizedBits)
		rt.DataType = dt
		b.FEC.PDU = fecResult
		return &rt, nil
	case elements.DataTypeRate12:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		rt, fecResult := pdu.DecodeRate12Data(sizedBits)
		rt.DataType = dt
		b.FEC.PDU = fecResult
		return &rt, nil
	case elements.DataTypeRate1:
		var sizedBits [192]bit.Bit
		copy(sizedBits[:], infoBits[:192])
		rt, fecResult := pdu.DecodeRate1Data(sizedBits)
		rt.DataType = dt
		b.FEC.PDU = fecResult
		return &rt, nil
	case elements.DataTypeMBCHeader:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeMBCHeader(sizedBits)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode MBC header from bits")
		}
		return &decoded, nil
	case elements.DataTypeMBCContinuation:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeMBCContinuation(sizedBits)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		return &decoded, nil
	case elements.DataTypeIdle:
		prFill := &pdu.PRFill{DataType: dt}
		return prFill, nil
	case elements.DataTypeUnifiedSingleBlock:
		var sizedBits [96]bit.Bit
		copy(sizedBits[:], infoBits[:96])
		decoded, fecResult := pdu.DecodeUnifiedSingleBlockData(sizedBits)
		decoded.DataType = dt
		b.FEC.PDU = fecResult
		if fecResult.Uncorrectable {
			return nil, fmt.Errorf("failed to decode USBD from bits")
		}
		return &decoded, nil
	case elements.DataTypeReserved:
		return nil, fmt.Errorf("reserved data type parsing not implemented")
	default:
		return nil, fmt.Errorf("unhandled data type %v", b.SlotType.DataType)
	}
}

// Encode returns the encoded bytes of the burst. IsData selects the
// payload as DecodeFromBytes sets it: a data burst carries the slot type and
// data, and every other burst carries voice. VoiceBurst has no effect. A
// burst without embedded signalling needs a Table 9.2 SYNC pattern, and the
// pattern is a data SYNC pattern exactly when IsData is set. The zero Burst
// has neither, so it does not encode. A data burst needs HasSlotType. With
// HasReverseChannel set, the embedded signalling data field carries the
// coded ReverseChannel instead of EmbeddedSignallingData. Without it the
// field carries EmbeddedSignallingData as it is: the FEC of an embedded LC
// spans bursts B to E, so a single burst does not correct it. Encode returns an
// error for a burst that breaks these rules and when a field value does not
// fit in its on-air bits. Encode does not modify the burst.
//
// A data burst carries Data coded for SlotType.DataType: BPTC(196,96) for
// the BPTC types, the Rate 3/4 trellis or Rate 1. The slot type data type
// also selects the RS(12,9) mask of a Full LC and replaces its DataType.
// Data must be the pdu type that DecodeFromBytes returns for the data type,
// or for an MBC continuation one of its CGAP, MVAP, BCAP and VNAP forms. With
// Data nil, a burst from DecodeFromBytes carries the payload that it decoded,
// after FEC correction, so a burst whose CRC failed can still be forwarded.
// The PR FILL that DecodeFromBytes returns for an idle burst also stands
// for the decoded payload. The decoded payload goes out only under the data
// type it was decoded as. A data burst built by hand needs Data. The BPTC
// R(3..0) bits and the Rate 1 pad bits are set to zero (ETSI TS 102 361-1
// B.1.1, B.2.5), so Encode writes zero there.
//
// A voice burst carries three 72-bit vocoder slots. For a burst from
// DecodeFromBytes, Encode writes each slot as received while the
// DecodedBits of its frame in VoiceData equal those that DecodeFromBytes
// decoded, so a slot of a codec other than AMBE+2, or an AMBE+2 frame with
// bit errors, goes out unchanged. Encode codes a changed frame, and every
// frame of a burst built by hand, with vocoder.EncodeAMBE72, which also
// gives a frame that the decoder corrected clean FEC bits. To regenerate
// every frame of a decoded burst, copy its VoiceData into a new Burst.
func (b *Burst) Encode() ([33]byte, error) {
	if b.SyncPattern == enums.MsSourcedRcSync {
		bitData, err := b.encodeStandaloneRC()
		if err != nil {
			return [33]byte{}, err
		}
		return bit.PackBits264(bitData), nil
	}
	if err := b.checkCentre(); err != nil {
		return [33]byte{}, err
	}
	if b.IsData && !b.HasSlotType {
		return [33]byte{}, fmt.Errorf("data burst has no slot type")
	}

	var bitData [264]bit.Bit

	if b.IsData {
		slotBits, err := pdu.EncodeSlotType(&b.SlotType)
		if err != nil {
			return [33]byte{}, fmt.Errorf("cannot encode slot type: %w", err)
		}
		copy(bitData[98:108], slotBits[:10])
		copy(bitData[156:166], slotBits[10:20])

		dataBits, err := b.encodeDataBits()
		if err != nil {
			return [33]byte{}, err
		}
		copy(bitData[:98], dataBits[:98])
		copy(bitData[166:264], dataBits[98:196])
	} else {
		voiceBits, err := b.voiceBits()
		if err != nil {
			return [33]byte{}, err
		}
		copy(bitData[:108], voiceBits[:108])
		copy(bitData[156:264], voiceBits[108:216])
	}

	// Sync or Embedded Signalling
	if b.HasEmbeddedSignalling {
		data, err := b.embeddedData()
		if err != nil {
			return [33]byte{}, err
		}
		esBits, err := pdu.EncodeEmbeddedSignalling(&b.EmbeddedSignalling)
		if err != nil {
			return [33]byte{}, fmt.Errorf("cannot encode embedded signalling: %w", err)
		}
		copy(bitData[108:116], esBits[0:8])
		copy(bitData[116:148], data[:])
		copy(bitData[148:156], esBits[8:16])
	} else {
		writeSync(&bitData, b.SyncPattern)
	}

	return bit.PackBits264(bitData), nil
}

// voiceBits returns the 216 voice bits that Encode writes, as described
// there.
func (b *Burst) voiceBits() ([216]bit.Bit, error) {
	var voice [216]bit.Bit
	copy(voice[:108], b.bitData[:108])
	copy(voice[108:], b.bitData[156:264])
	for i := range b.VoiceData.Frames {
		if b.hasVoiceSlots && b.VoiceData.Frames[i].DecodedBits == b.decodedVoice.Frames[i].DecodedBits {
			continue
		}
		slot, err := vocoder.EncodeVocoderFrame(&b.VoiceData.Frames[i])
		if err != nil {
			return voice, fmt.Errorf("cannot encode voice: VoiceData.Frames[%d]: %w", i, err)
		}
		copy(voice[i*72:(i+1)*72], slot[:])
	}
	return voice, nil
}

func (b *Burst) checkCentre() error {
	if b.HasEmbeddedSignalling {
		return nil
	}
	if !isSyncPattern(b.SyncPattern) {
		return fmt.Errorf("SYNC pattern %#x is not a Table 9.2 pattern and the burst has no embedded signalling", int64(b.SyncPattern))
	}
	if b.IsData != isDataSync(b.SyncPattern) {
		return fmt.Errorf("IsData is %t but the SYNC pattern is %s", b.IsData, enums.SyncPatternToName(b.SyncPattern))
	}
	return nil
}

// dataInfoBits returns the information bits of the data payload: Data coded
// for the slot type data type or, when Data is nil, the payload that
// DecodeFromBytes decoded. The PR FILL that DecodeFromBytes returns for an
// idle burst stands for the decoded payload, which need not be Table D.2.
func (b *Burst) dataInfoBits() ([196]bit.Bit, error) {
	dt := b.SlotType.DataType
	_, prFill := b.Data.(*pdu.PRFill)
	switch {
	case b.Data != nil && (!prFill || b.Data != b.decodedData):
		return encodeData(b.Data, dt)
	case b.deinterleavedInfoLen == 0:
		return [196]bit.Bit{}, fmt.Errorf("data burst has no Data")
	case dt != b.decodedDataType:
		return [196]bit.Bit{}, fmt.Errorf("decoded payload is a %s but the slot type data type is %s; set Data",
			elements.DataTypeToName(b.decodedDataType), elements.DataTypeToName(dt))
	default:
		return b.deinterleavedInfoBits, nil
	}
}

// encodeData returns the information bits of d in a data burst of dataType.
func encodeData(d elements.Data, dataType elements.DataType) ([196]bit.Bit, error) {
	var info [196]bit.Bit
	if rv := reflect.ValueOf(d); rv.Kind() == reflect.Pointer && rv.IsNil() || !dataTypeCarries(dataType, d) {
		return info, fmt.Errorf("data type %s cannot carry Data of type %T", elements.DataTypeToName(dataType), d)
	}
	bits, err := encodePDU(d, dataType)
	if err != nil {
		return info, fmt.Errorf("cannot encode data: %w", err)
	}
	copy(info[:], bits)
	return info, nil
}

// dataTypeCarries reports whether a data burst of dataType carries d.
func dataTypeCarries(dataType elements.DataType, d elements.Data) bool {
	switch d.(type) {
	case *pdu.FullLinkControl:
		return dataType == elements.DataTypeVoiceLCHeader || dataType == elements.DataTypeTerminatorWithLC
	case *pdu.CSBK:
		return dataType == elements.DataTypeCSBK
	case *pdu.PIHeader:
		return dataType == elements.DataTypePIHeader
	case *pdu.DataHeader:
		return dataType == elements.DataTypeDataHeader
	case *pdu.MBCHeader:
		return dataType == elements.DataTypeMBCHeader
	case *pdu.MBCContinuation, *pdu.CGAPContinuation, *pdu.MVAPContinuation, *pdu.BCAPContinuation, *pdu.VNAPContinuation:
		return dataType == elements.DataTypeMBCContinuation
	case *pdu.UnifiedSingleBlockData:
		return dataType == elements.DataTypeUnifiedSingleBlock
	case *pdu.Rate12Data:
		return dataType == elements.DataTypeRate12
	case *pdu.Rate34Data:
		return dataType == elements.DataTypeRate34
	case *pdu.Rate1Data:
		return dataType == elements.DataTypeRate1
	case *pdu.PRFill:
		return dataType == elements.DataTypeIdle
	default:
		return false
	}
}

// encodePDU returns the information bits of d, a type that dataTypeCarries
// accepts. The data type of a Full LC selects its RS(12,9) mask, so
// dataType replaces lc.DataType.
func encodePDU(d elements.Data, dataType elements.DataType) ([]bit.Bit, error) {
	switch v := d.(type) {
	case *pdu.FullLinkControl:
		lc := *v
		lc.DataType = dataType
		return bits96(pdu.EncodeFullLinkControl(&lc))
	case *pdu.CSBK:
		return bits96(pdu.EncodeCSBK(v))
	case *pdu.PIHeader:
		return bits96(pdu.EncodePIHeader(v))
	case *pdu.DataHeader:
		return bits96(pdu.EncodeDataHeader(v))
	case *pdu.MBCHeader:
		return bits96(pdu.EncodeMBCHeader(v))
	case *pdu.MBCContinuation:
		return bits96(pdu.EncodeMBCContinuation(v))
	case *pdu.CGAPContinuation:
		return bits96(pdu.EncodeCGAPContinuation(v))
	case *pdu.MVAPContinuation:
		return bits96(pdu.EncodeMVAPContinuation(v))
	case *pdu.BCAPContinuation:
		return bits96(pdu.EncodeBCAPContinuation(v))
	case *pdu.VNAPContinuation:
		return bits96(pdu.EncodeVNAPContinuation(v))
	case *pdu.UnifiedSingleBlockData:
		return bits96(pdu.EncodeUnifiedSingleBlockData(v))
	case *pdu.Rate12Data:
		return bits96(pdu.EncodeRate12Data(v))
	case *pdu.Rate34Data:
		bits, err := pdu.EncodeRate34Data(v)
		return bits[:], err
	case *pdu.Rate1Data:
		bits, err := pdu.EncodeRate1Data(v)
		return bits[:], err
	case *pdu.PRFill:
		bits := pdu.PRFillBits()
		return bits[:], nil
	default:
		return nil, fmt.Errorf("unsupported Data type %T", d)
	}
}

func bits96(bits [96]bit.Bit, err error) ([]bit.Bit, error) {
	return bits[:], err
}

// encodeDataBits codes the information bits of a data burst for the
// channel: BPTC(196,96), Rate 3/4 trellis or Rate 1 as the slot type data
// type selects. The BPTC R(3..0) bits and the Rate 1 pad bits are zero.
func (b *Burst) encodeDataBits() ([196]bit.Bit, error) {
	info, err := b.dataInfoBits()
	if err != nil {
		return [196]bit.Bit{}, err
	}
	dt := b.SlotType.DataType
	switch {
	case dt == elements.DataTypeRate34:
		var t trellis.Trellis34
		var data [144]bit.Bit
		copy(data[:], info[:144])
		return t.Encode(data), nil
	case dt == elements.DataTypeRate1:
		// Table B.10B: Transmit bit ordering for rate 1 coded data
		var bits [196]bit.Bit
		copy(bits[:96], info[:96])
		copy(bits[100:], info[96:192])
		return bits, nil
	case isBPTCDataType(dt):
		var infoBits [96]bit.Bit
		copy(infoBits[:], info[:96])
		return bptc.Encode(infoBits), nil
	default:
		return info, nil
	}
}

// PackEmbeddedSignallingData converts the 32-bit (unpacked) embedded signalling
// data into a 4-byte packed array.
func (b *Burst) PackEmbeddedSignallingData() [4]byte {
	var data [4]byte
	for i := 0; i < 32; i++ {
		data[i/8] |= byte(b.EmbeddedSignallingData[i]) << (7 - (i % 8)) //nolint:gosec // i/8 bounded: i<32 so i/8<=3 <4
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

// BuildLCDataBurst builds a 33-byte LC data burst (e.g. for Voice Header or
// Terminator) with the BS-sourced data SYNC pattern. VoiceTransmitter builds
// these bursts for the other SYNC sources.
// lcBytes is the 12-byte on-air codeword with the Data Type CRC Mask already
// applied, as returned by pdu.EncodeFullLinkControl with DataType set to dataType.
// It returns an error if colorCode or dataType does not fit in the slot type.
func BuildLCDataBurst(lcBytes [12]byte, dataType elements.DataType, colorCode uint8) ([33]byte, error) {
	return buildLCDataBurst(lcBytes, dataType, colorCode, enums.BsSourcedData)
}

func buildLCDataBurst(lcBytes [12]byte, dataType elements.DataType, colorCode uint8, sync enums.SyncPattern) ([33]byte, error) {
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
	slotTypeBits, err := pdu.EncodeSlotType(&pdu.SlotType{
		ColorCode: int(colorCode),
		DataType:  dataType,
	})
	if err != nil {
		return [33]byte{}, fmt.Errorf("cannot encode slot type: %w", err)
	}

	copy(bitData[98:108], slotTypeBits[:10])
	copy(bitData[156:166], slotTypeBits[10:20])

	writeSync(&bitData, sync)

	return bit.PackBits264(bitData), nil
}
