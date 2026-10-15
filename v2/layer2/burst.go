// Package layer2 decodes and encodes DMR bursts (ETSI TS 102 361-1 clause
// 6 and 9): the Burst model, the byte-level RawBurst, the embedded LC, the
// CACH and Short LC, the Reverse Channel, and the voice superframe
// transmitter and receiver. The PDUs that the bursts carry are in package
// pdu.
//
// Errors follow the rule of package fec: decoding never fails because FEC
// or a CRC cannot correct or check the bits. Burst.FEC and the FEC field of
// each PDU report that, and the decoded fields are still set, so a caller
// can inspect or forward them. An error means that the input cannot be
// decoded, such as ErrReservedDataType, or breaks a caller-given rule, such
// as ErrColorCodeMismatch. A caller that wants an error for an
// uncorrectable burst uses b.FEC.Aggregate().Err(). The errors of this
// package that report an uncorrectable result, such as
// ErrEmbeddedLCUncorrectable, wrap fec.ErrUncorrectable.
package layer2

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ErrReservedDataType is returned, wrapped, by DecodeFromBytes for a data
// burst whose slot type carries a reserved data type (Table 9.22). The burst
// has no Data.
var ErrReservedDataType = errors.New("reserved data type")

// Kind is the kind of a burst.
type Kind uint8

const (
	// KindUnknown is a burst of unknown kind. In DecodeOptions it lets the
	// SYNC field classify the burst; Encode rejects it.
	KindUnknown Kind = iota
	// KindVoice is a voice burst: three vocoder slots around a voice SYNC
	// pattern (burst A) or embedded signalling (bursts B to F).
	KindVoice
	// KindData is a data or control burst: a slot type and an information
	// payload around a data SYNC pattern.
	KindData
	// KindReverseChannel is a standalone inbound RC burst (§6.4.1).
	KindReverseChannel
)

// String returns the name of the kind.
func (k Kind) String() string {
	switch k {
	case KindUnknown:
		return "Unknown"
	case KindVoice:
		return "Voice"
	case KindData:
		return "Data"
	case KindReverseChannel:
		return "Reverse Channel"
	}
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Burst represents a DMR burst.
type Burst struct {
	Kind Kind
	// SyncPattern is the Table 9.2 pattern in the burst centre, or
	// enums.SyncNone when the centre carries embedded signalling, as in
	// voice bursts B to F. A standalone RC burst has enums.SyncMSSourcedRC.
	SyncPattern enums.SyncPattern
	// NearestSync is the Table 9.2 pattern nearest to the received centre
	// field and SyncDistance its Hamming distance, whatever the
	// classification. DecodeFromBytes sets them; Encode ignores them.
	NearestSync  enums.SyncPattern
	SyncDistance int
	// VoiceBurst is the superframe position of a voice burst. Decoding is
	// stateless per burst, so DecodeFromBytes sets VoiceBurstA for a burst
	// with a voice SYNC pattern and VoiceBurstUnknown for every other burst,
	// bursts B to F included, unless DecodeOptions gives the position.
	// VoiceReceiver sets the position of bursts B to F from the superframe
	// sequence. Encode ignores it.
	VoiceBurst enums.VoiceBurstType

	// VoiceSlots holds the three 72-bit vocoder slots of a voice burst as
	// received, before any FEC: slot i is voice bits 72i to 72i+71, where
	// voice bits 0 to 107 are burst bits 0 to 107 and voice bits 108 to 215
	// are burst bits 156 to 263. Encode writes them as they are. The slots
	// are codec specific; AMBEFrames decodes AMBE+2 slots.
	VoiceSlots [VoiceSlotsPerBurst][72]bit.Bit

	// SlotType is the slot type of a data burst.
	SlotType pdu.SlotType

	// EmbeddedSignalling and EmbeddedSignallingData are the EMB and the
	// 32-bit embedded data field of a burst with HasEmbeddedSignalling.
	EmbeddedSignalling     pdu.EmbeddedSignalling
	EmbeddedSignallingData [32]bit.Bit

	// ReverseChannel is the RC PDU of a burst with HasReverseChannel.
	ReverseChannel pdu.ReverseChannel

	// Data is the decoded PDU of a data burst. DecodeFromBytes sets it also
	// when the PDU CRC or FEC fails, which FEC reports, and leaves it nil
	// only for a reserved data type. See Encode.
	Data Data
	FEC  fec.BurstFECStats

	bitData               [264]bit.Bit
	decodedDataType       enums.DataType
	deinterleavedInfoBits [196]bit.Bit
	deinterleavedInfoLen  int
}

// HasEmbeddedSignalling reports whether the burst carries an EMB and an
// embedded data field: a voice burst without a SYNC pattern, or a standalone
// RC burst.
func (b *Burst) HasEmbeddedSignalling() bool {
	return b.Kind == KindReverseChannel || b.Kind == KindVoice && b.SyncPattern == enums.SyncNone
}

// HasReverseChannel reports whether the embedded data field carries an RC
// PDU: in a standalone RC burst, and in a voice burst whose EMB has PI set
// and LCSS single fragment (§6.4).
func (b *Burst) HasReverseChannel() bool {
	if b.Kind == KindReverseChannel {
		return true
	}
	return b.HasEmbeddedSignalling() && b.EmbeddedSignalling.PreemptionAndPowerControlIndicator &&
		b.EmbeddedSignalling.LCSS == enums.LCSSSingleFragment
}

// NewBurstFromBytes decodes data into a new Burst, as DecodeFromBytes does.
// It returns the burst also with an error, populated as DecodeFromBytes
// leaves it.
func NewBurstFromBytes(data [33]byte) (*Burst, error) {
	burst := &Burst{}
	err := burst.DecodeFromBytes(data)
	return burst, err
}

// DecodeFromBytes populates the burst in place, enabling zero-allocation
// decoding when reusing a Burst. It is DecodeFromBytesWith with the zero
// DecodeOptions.
func (b *Burst) DecodeFromBytes(data [33]byte) error {
	return b.DecodeFromBytesWith(data, DecodeOptions{})
}

// DecodeFromBytesWith decodes a burst with receive-side options. A FEC or
// CRC failure is not an error: FEC reports it and Data is still set. The
// error is ErrReservedDataType for a reserved data type, ErrColorCodeMismatch
// for a color code other than DecodeOptions.ColorCode, or an error for
// invalid options. The burst is populated even when an error is returned; a
// color code mismatch is joined with any decode error.
func (b *Burst) DecodeFromBytesWith(data [33]byte, opts DecodeOptions) error {
	*b = Burst{}
	b.bitData = bit.Unpack264(data)
	b.NearestSync, b.SyncDistance = b.nearestSync(allSyncPatterns)
	err := b.decode(opts)
	return errors.Join(err, b.checkColorCode(opts.ColorCode))
}

func (b *Burst) decode(opts DecodeOptions) error {
	threshold := opts.SyncThreshold
	switch {
	case threshold == 0:
		threshold = DefaultSyncThreshold
	case threshold < 0:
		threshold = 0
	}
	nearest, distance := b.NearestSync, b.SyncDistance

	switch opts.Kind {
	case KindVoice:
		return b.decodeVoiceKind(opts.VoiceBurst, nearest, distance, threshold)
	case KindData:
		return b.decodeData(opts.DataType)
	case KindReverseChannel:
		b.decodeStandaloneRC()
		return nil
	case KindUnknown:
	default:
		return fmt.Errorf("unknown burst kind %d", opts.Kind)
	}

	switch {
	case distance > threshold:
		b.decodeVoice(enums.VoiceBurstUnknown, enums.SyncNone)
	case isDataSync(nearest):
		return b.decodeData(nil)
	case isVoiceSync(nearest):
		b.decodeVoice(enums.VoiceBurstA, nearest)
	case nearest == enums.SyncMSSourcedRC:
		b.decodeStandaloneRC()
	default:
		b.decodeVoice(enums.VoiceBurstUnknown, nearest)
	}
	return nil
}

func (b *Burst) decodeVoiceKind(pos enums.VoiceBurstType, nearest enums.SyncPattern, distance, threshold int) error {
	switch pos {
	case enums.VoiceBurstA:
		p, _ := b.nearestSync(voiceSyncPatterns)
		b.decodeVoice(pos, p)
	case enums.VoiceBurstB, enums.VoiceBurstC, enums.VoiceBurstD, enums.VoiceBurstE, enums.VoiceBurstF:
		b.decodeVoice(pos, enums.SyncNone)
	case enums.VoiceBurstUnknown:
		if distance <= threshold && isVoiceSync(nearest) {
			b.decodeVoice(enums.VoiceBurstA, nearest)
		} else {
			b.decodeVoice(pos, enums.SyncNone)
		}
	default:
		return fmt.Errorf("voice burst position %d out of range", pos)
	}
	return nil
}

func (b *Burst) decodeVoice(pos enums.VoiceBurstType, sync enums.SyncPattern) {
	b.Kind, b.SyncPattern, b.VoiceBurst = KindVoice, sync, pos
	if sync == enums.SyncNone {
		b.EmbeddedSignalling, b.EmbeddedSignallingData = parseEmbedded(b.bitData)
		b.FEC.EMB = b.EmbeddedSignalling.FEC
		if b.HasReverseChannel() {
			b.ReverseChannel = DecodeEmbeddedRC(b.EmbeddedSignallingData)
			b.FEC.RC = b.ReverseChannel.FEC
		}
	}
	b.VoiceSlots = parseVoiceSlots(b.bitData)
}

func (b *Burst) decodeData(dataType *enums.DataType) error {
	b.Kind = KindData
	b.SyncPattern, _ = b.nearestSync(dataSyncPatterns)
	b.SlotType = parseSlotType(b.bitData)
	b.FEC.SlotType = b.SlotType.FEC
	if dataType != nil {
		b.SlotType.DataType = *dataType
	}
	b.deinterleavedInfoLen, b.FEC.Payload = b.deinterleave(extractDataBits(b.bitData), b.SlotType.DataType)
	var err error
	b.Data, b.FEC.PDU, err = decodePayload(b.SlotType.DataType, b.deinterleavedInfoBits[:b.deinterleavedInfoLen], b.SyncPattern)
	b.decodedDataType = b.SlotType.DataType
	return err
}

// opcode0x38Hint returns the likely meaning of CSBK opcode 0x38 in a burst
// with SYNC pattern sync: an MS sends BS_Dwn_Act and a TSCC sends TD_GRANT_MI.
func opcode0x38Hint(sync enums.SyncPattern) pdu.Opcode0x38Hint {
	switch sync {
	case enums.SyncMSSourcedData:
		return pdu.Opcode0x38BSOutboundActivation
	case enums.SyncBSSourcedData:
		return pdu.Opcode0x38TalkgroupDataGrantMultiItem
	case enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncMSSourcedRC, enums.SyncTDMA1Voice,
		enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data, enums.SyncReserved, enums.SyncNone:
	}
	return pdu.Opcode0x38Unknown
}

func isDataSync(sync enums.SyncPattern) bool {
	return sync == enums.SyncTDMA1Data || sync == enums.SyncTDMA2Data || sync == enums.SyncMSSourcedData || sync == enums.SyncBSSourcedData
}

func isVoiceSync(sync enums.SyncPattern) bool {
	return sync == enums.SyncTDMA1Voice || sync == enums.SyncTDMA2Voice || sync == enums.SyncMSSourcedVoice || sync == enums.SyncBSSourcedVoice
}

func isSyncPattern(sync enums.SyncPattern) bool {
	return isDataSync(sync) || isVoiceSync(sync) || sync == enums.SyncMSSourcedRC || sync == enums.SyncReserved
}

// decodeStandaloneRC decodes a §6.4.1 standalone inbound RC burst (Figure
// 6.7, Table E.12): EMB(8) RC(16) SYNC(48) RC(16) EMB(8) around the burst
// centre. The RC SYNC stays in place and the burst carries no voice.
// An RC burst always carries an RC PDU, so PI does not gate RC decoding.
func (b *Burst) decodeStandaloneRC() {
	b.Kind, b.SyncPattern = KindReverseChannel, enums.SyncMSSourcedRC
	var embBits [16]bit.Bit
	copy(embBits[:8], b.bitData[rcembFirstStart:rcembFirstEnd])
	copy(embBits[8:], b.bitData[rcembSecondStart:rcembSecondEnd])
	b.EmbeddedSignalling = pdu.DecodeEmbeddedSignalling(embBits)
	b.FEC.EMB = b.EmbeddedSignalling.FEC

	copy(b.EmbeddedSignallingData[:16], b.bitData[rcDataFirstStart:rcDataFirstEnd])
	copy(b.EmbeddedSignallingData[16:], b.bitData[rcDataSecondStart:rcDataSecondEnd])
	b.ReverseChannel = DecodeEmbeddedRC(b.EmbeddedSignallingData)
	b.FEC.RC = b.ReverseChannel.FEC
}

func (b *Burst) encodeStandaloneRC() ([264]bit.Bit, error) {
	var bitData [264]bit.Bit
	data, err := b.embeddedData()
	if err != nil {
		return bitData, err
	}
	embBits, err := b.EmbeddedSignalling.Encode()
	if err != nil {
		return bitData, fmt.Errorf("cannot encode embedded signalling: %w", err)
	}
	copy(bitData[rcembFirstStart:rcembFirstEnd], embBits[:8])
	copy(bitData[rcDataFirstStart:rcDataFirstEnd], data[:16])
	writeSync(&bitData, enums.SyncMSSourcedRC)
	copy(bitData[rcDataSecondStart:rcDataSecondEnd], data[16:])
	copy(bitData[rcembSecondStart:rcembSecondEnd], embBits[8:])
	return bitData, nil
}

// embeddedData returns the 32 bits of the embedded signalling data field:
// the coded ReverseChannel when HasReverseChannel reports true, and
// EmbeddedSignallingData otherwise.
func (b *Burst) embeddedData() ([32]bit.Bit, error) {
	if !b.HasReverseChannel() {
		return b.EmbeddedSignallingData, nil
	}
	rcBits, err := EncodeEmbeddedRC(&b.ReverseChannel)
	if err != nil {
		return [32]bit.Bit{}, fmt.Errorf("cannot encode reverse channel: %w", err)
	}
	return rcBits, nil
}

func writeSync(bitData *[264]bit.Bit, sync enums.SyncPattern) {
	for i := 0; i < 48; i++ {
		bitData[syncStart+i] = bit.Bit((sync >> (47 - i)) & 1)
	}
}

func parseEmbedded(bitData [264]bit.Bit) (pdu.EmbeddedSignalling, [32]bit.Bit) {
	var embeddedBits [16]bit.Bit
	copy(embeddedBits[:8], bitData[108:116])
	copy(embeddedBits[8:], bitData[148:156])

	embedded := pdu.DecodeEmbeddedSignalling(embeddedBits)
	var embeddedData [32]bit.Bit
	copy(embeddedData[:], bitData[116:148])
	return embedded, embeddedData
}

func parseSlotType(bitData [264]bit.Bit) pdu.SlotType {
	var slotBits [20]bit.Bit
	copy(slotBits[:10], bitData[98:108])
	copy(slotBits[10:], bitData[156:166])
	return pdu.DecodeSlotType(slotBits)
}

func parseVoiceSlots(bitData [264]bit.Bit) [VoiceSlotsPerBurst][72]bit.Bit {
	var voice [216]bit.Bit
	copy(voice[:108], bitData[:108])
	copy(voice[108:], bitData[156:264])
	var slots [VoiceSlotsPerBurst][72]bit.Bit
	for i := range slots {
		copy(slots[i][:], voice[i*72:(i+1)*72])
	}
	return slots
}

// AMBEFrames decodes the three voice slots as AMBE+2 frames and returns them
// with the combined FEC result of the frame FEC (vocoder.DecodeAMBE72). The
// result is meaningful only for a burst that carries AMBE+2 voice; the
// slots of another codec decode to noise.
func (b *Burst) AMBEFrames() ([VoiceSlotsPerBurst]vocoder.AMBEFrame, fec.Result) {
	var frames [VoiceSlotsPerBurst]vocoder.AMBEFrame
	var result fec.Result
	for i, slot := range b.VoiceSlots {
		f, r := vocoder.DecodeAMBE72(slot)
		frames[i] = f
		result = result.Combine(r)
	}
	return frames, result
}

// SetAMBEFrames sets the three voice slots to the AMBE+2 coding of frames
// (vocoder.EncodeAMBE72).
func (b *Burst) SetAMBEFrames(frames [VoiceSlotsPerBurst]vocoder.AMBEFrame) {
	for i, f := range frames {
		b.VoiceSlots[i] = vocoder.EncodeAMBE72(f)
	}
}

func extractDataBits(bitData [264]bit.Bit) [196]bit.Bit {
	var bits [196]bit.Bit
	copy(bits[:98], bitData[:98])
	copy(bits[98:], bitData[166:264])
	return bits
}

func (b *Burst) deinterleave(bits [196]bit.Bit, dataType enums.DataType) (int, fec.Result) {
	switch dataType {
	case enums.DataTypeRate34:
		decoded, result := trellis.Decode(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), result
	case enums.DataTypeRate1:
		// Table B.10B: Transmit bit ordering for rate 1 coded data
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[i] = bits[i]
		}
		for i := 0; i < 96; i++ {
			b.deinterleavedInfoBits[96+i] = bits[100+i]
		}
		return 192, fec.Result{}
	case enums.DataTypePIHeader,
		enums.DataTypeVoiceLCHeader,
		enums.DataTypeTerminatorWithLC,
		enums.DataTypeCSBK,
		enums.DataTypeMBCHeader,
		enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader,
		enums.DataTypeRate12,
		enums.DataTypeIdle,
		enums.DataTypeUnifiedSingleBlock:
		decoded, result := bptc.Decode196(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		return len(decoded), result
	case enums.DataTypeReserved:
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
	default:
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
	}
}

// isBPTCDataType reports whether bursts of dataType carry 96 information
// bits coded with BPTC(196,96).
func isBPTCDataType(dataType enums.DataType) bool {
	switch dataType {
	case enums.DataTypePIHeader,
		enums.DataTypeVoiceLCHeader,
		enums.DataTypeTerminatorWithLC,
		enums.DataTypeCSBK,
		enums.DataTypeMBCHeader,
		enums.DataTypeMBCContinuation,
		enums.DataTypeDataHeader,
		enums.DataTypeRate12,
		enums.DataTypeIdle,
		enums.DataTypeUnifiedSingleBlock:
		return true
	case enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeReserved:
		return false
	default:
		return false
	}
}

// String returns a string representation of the burst.
func (b *Burst) String() string {
	ret := fmt.Sprintf("{ Kind: %s, SyncPattern: %s, ", b.Kind, b.SyncPattern)
	if b.HasEmbeddedSignalling() {
		ret += fmt.Sprintf("EmbeddedSignalling: %s, ", b.EmbeddedSignalling.String())
		if b.HasReverseChannel() {
			ret += fmt.Sprintf("ReverseChannel: %s, ", b.ReverseChannel.String())
		}
	}
	switch b.Kind {
	case KindData:
		agg := b.FEC.Aggregate()
		ret += fmt.Sprintf("SlotType: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t}",
			b.SlotType.String(), agg.BitsChecked, agg.ErrorsCorrected, agg.Uncorrectable)
		if b.Data != nil {
			ret += fmt.Sprintf(", Data: %s", b.Data.String())
		}
	case KindVoice:
		ret += fmt.Sprintf("VoiceBurst: %s", b.VoiceBurst.String())
	case KindUnknown, KindReverseChannel:
		ret += fmt.Sprintf("NearestSync: %s, SyncDistance: %d", b.NearestSync, b.SyncDistance)
	}
	return ret + " }"
}

// decodePayload decodes the information bits of a data burst of data type dt
// and SYNC pattern sync. The fec.Result is the FEC field of a PDU that has
// one, and zero for the data blocks, whose only code is the payload code.
func decodePayload(dt enums.DataType, infoBits []bit.Bit, sync enums.SyncPattern) (Data, fec.Result, error) {
	if dt.IsReserved() {
		return nil, fec.Result{}, fmt.Errorf("%w %d", ErrReservedDataType, dt)
	}
	var bits96 [96]bit.Bit
	copy(bits96[:], infoBits)
	switch dt {
	case enums.DataTypeCSBK:
		decoded := pdu.DecodeCSBK(bits96)
		decoded.Hint0x38 = opcode0x38Hint(sync)
		return &decoded, decoded.FEC, nil
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		decoded := pdu.DecodeFullLinkControl(bits96, dt)
		return &decoded, decoded.FEC, nil
	case enums.DataTypePIHeader:
		decoded := pdu.DecodePIHeader(bits96)
		return &decoded, decoded.FEC, nil
	case enums.DataTypeDataHeader:
		decoded := pdu.DecodeDataHeader(bits96)
		return &decoded, decoded.FEC, nil
	case enums.DataTypeRate34:
		var sizedBits [144]bit.Bit
		copy(sizedBits[:], infoBits)
		rt := pdu.DecodeRate34Data(sizedBits)
		return &rt, fec.Result{}, nil
	case enums.DataTypeRate12:
		rt := pdu.DecodeRate12Data(bits96)
		return &rt, fec.Result{}, nil
	case enums.DataTypeRate1:
		var sizedBits [192]bit.Bit
		copy(sizedBits[:], infoBits)
		rt := pdu.DecodeRate1Data(sizedBits)
		return &rt, fec.Result{}, nil
	case enums.DataTypeMBCHeader:
		decoded := pdu.DecodeMBCHeader(bits96)
		return &decoded, decoded.FEC, nil
	case enums.DataTypeMBCContinuation:
		decoded := pdu.DecodeMBCContinuation(bits96)
		return &decoded, fec.Result{}, nil
	case enums.DataTypeIdle:
		return &pdu.PRFill{}, fec.Result{}, nil
	case enums.DataTypeUnifiedSingleBlock:
		decoded := pdu.DecodeUnifiedSingleBlockData(bits96)
		return &decoded, decoded.FEC, nil
	case enums.DataTypeReserved:
	}
	return nil, fec.Result{}, fmt.Errorf("%w %d", ErrReservedDataType, dt)
}

// Encode returns the encoded bytes of the burst. Kind selects the payload.
// A voice burst carries a voice SYNC pattern, enums.SyncReserved, or, with
// SyncPattern enums.SyncNone, embedded signalling. A data burst carries a
// data SYNC pattern and the slot type. A standalone RC burst carries the RC
// SYNC pattern. VoiceBurst has no effect. When HasReverseChannel reports
// true, the embedded signalling data field carries the coded ReverseChannel
// instead of EmbeddedSignallingData. Otherwise the field carries
// EmbeddedSignallingData as it is: the FEC of an embedded LC spans bursts B
// to E, so a single burst does not correct it. Encode returns an error for a
// burst that breaks these rules and when a field value does not fit in its
// on-air bits. Encode does not modify the burst.
//
// A data burst carries the EncodeBits of Data coded for SlotType.DataType:
// BPTC(196,96) for the BPTC types, the Rate 3/4 trellis or Rate 1.
// Data.DataType must be SlotType.DataType, so the BurstDataType of a Full
// LC, which selects its RS(12,9) mask, must match the slot type. For an MBC
// continuation, Data can be one of its CGAP, MVAP, BCAP and VNAP forms. A
// burst from DecodeFromBytes whose Data codes to the same bits as the
// decoded payload, or whose Data is nil, carries the payload as it was
// decoded, after FEC correction, so a burst whose CRC failed can still be
// forwarded. The decoded payload goes out only under the data type it was
// decoded as. A data burst built by hand needs Data. The BPTC
// R(3..0) bits and the Rate 1 pad bits are set to zero (ETSI TS 102 361-1
// B.1.1, B.2.5), so Encode writes zero there.
//
// A voice burst carries VoiceSlots as they are, so a slot of any codec, or
// an AMBE+2 frame with bit errors, goes out unchanged. SetAMBEFrames codes
// AMBE+2 frames into the slots.
func (b *Burst) Encode() ([33]byte, error) {
	if b.Kind == KindReverseChannel {
		bitData, err := b.encodeStandaloneRC()
		if err != nil {
			return [33]byte{}, err
		}
		return bit.Pack264(bitData), nil
	}
	if err := b.checkCentre(); err != nil {
		return [33]byte{}, err
	}

	var bitData [264]bit.Bit

	if b.Kind == KindData {
		slotBits, err := b.SlotType.Encode()
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
		voiceBits := b.voiceBits()
		copy(bitData[:108], voiceBits[:108])
		copy(bitData[156:264], voiceBits[108:216])
	}

	if b.HasEmbeddedSignalling() {
		data, err := b.embeddedData()
		if err != nil {
			return [33]byte{}, err
		}
		esBits, err := b.EmbeddedSignalling.Encode()
		if err != nil {
			return [33]byte{}, fmt.Errorf("cannot encode embedded signalling: %w", err)
		}
		copy(bitData[108:116], esBits[0:8])
		copy(bitData[116:148], data[:])
		copy(bitData[148:156], esBits[8:16])
	} else {
		writeSync(&bitData, b.SyncPattern)
	}

	return bit.Pack264(bitData), nil
}

// voiceBits returns the 216 voice bits of VoiceSlots.
func (b *Burst) voiceBits() [216]bit.Bit {
	var voice [216]bit.Bit
	for i, slot := range b.VoiceSlots {
		copy(voice[i*72:(i+1)*72], slot[:])
	}
	return voice
}

func (b *Burst) checkCentre() error {
	switch b.Kind {
	case KindVoice:
		if b.SyncPattern == enums.SyncNone || b.SyncPattern == enums.SyncReserved || isVoiceSync(b.SyncPattern) {
			return nil
		}
	case KindData:
		if isDataSync(b.SyncPattern) {
			return nil
		}
	case KindUnknown, KindReverseChannel:
		return fmt.Errorf("burst kind %s does not encode", b.Kind)
	default:
		return fmt.Errorf("unknown burst kind %d", b.Kind)
	}
	return fmt.Errorf("a %s burst cannot carry SYNC pattern %s", b.Kind, b.SyncPattern)
}

// dataInfoBits returns the information bits of the data payload: Data coded
// for the slot type data type or, when Data is nil, the payload that
// DecodeFromBytes decoded. When Data codes to the same bits as the decoded
// payload does, Data is unchanged from the decode, and the payload goes out
// as received, so a burst whose CRC failed keeps its bits.
func (b *Burst) dataInfoBits() ([196]bit.Bit, error) {
	dt := b.SlotType.DataType
	decoded := b.deinterleavedInfoLen > 0 && dt == b.decodedDataType
	switch {
	case b.Data != nil:
		info, err := encodeData(b.Data, dt)
		if err != nil || !decoded {
			return info, err
		}
		orig, _, decodeErr := decodePayload(dt, b.deinterleavedInfoBits[:b.deinterleavedInfoLen], b.SyncPattern)
		if decodeErr == nil {
			if origInfo, err := encodeData(orig, dt); err == nil && origInfo == info {
				return b.deinterleavedInfoBits, nil
			}
		}
		return info, nil
	case b.deinterleavedInfoLen == 0:
		return [196]bit.Bit{}, fmt.Errorf("data burst has no Data")
	case !decoded:
		return [196]bit.Bit{}, fmt.Errorf("decoded payload is a %s but the slot type data type is %s; set Data",
			b.decodedDataType.String(), dt.String())
	default:
		return b.deinterleavedInfoBits, nil
	}
}

// encodeData returns the information bits of d in a data burst of dataType.
func encodeData(d Data, dataType enums.DataType) ([196]bit.Bit, error) {
	var info [196]bit.Bit
	if rv := reflect.ValueOf(d); rv.Kind() == reflect.Pointer && rv.IsNil() {
		return info, fmt.Errorf("data type %s cannot carry a nil %T", dataType.String(), d)
	}
	if d.DataType() != dataType {
		return info, fmt.Errorf("data type %s cannot carry Data of type %T with data type %s",
			dataType.String(), d, d.DataType().String())
	}
	bits, err := d.EncodeBits()
	if err != nil {
		return info, fmt.Errorf("cannot encode data: %w", err)
	}
	if want := infoBitsLen(dataType); len(bits) != want {
		return info, fmt.Errorf("data type %s carries %d information bits, but %T encodes %d",
			dataType.String(), want, d, len(bits))
	}
	copy(info[:], bits)
	return info, nil
}

// infoBitsLen returns the number of information bits that a data burst of
// dataType carries.
func infoBitsLen(dataType enums.DataType) int {
	switch {
	case dataType == enums.DataTypeRate34:
		return 144
	case dataType == enums.DataTypeRate1:
		return 192
	case isBPTCDataType(dataType):
		return 96
	default:
		return 0
	}
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
	case dt == enums.DataTypeRate34:
		var data [144]bit.Bit
		copy(data[:], info[:144])
		return trellis.Encode(data), nil
	case dt == enums.DataTypeRate1:
		// Table B.10B: Transmit bit ordering for rate 1 coded data
		var bits [196]bit.Bit
		copy(bits[:96], info[:96])
		copy(bits[100:], info[96:192])
		return bits, nil
	case isBPTCDataType(dt):
		var infoBits [96]bit.Bit
		copy(infoBits[:], info[:96])
		return bptc.Encode196(infoBits), nil
	default:
		return info, nil
	}
}
