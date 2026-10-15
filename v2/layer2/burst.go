package layer2

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

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
	// payload around a data SYNC pattern or around embedded signalling
	// (ETSI TS 102 361-1 clause 5.1.5.1, clause 6.4.2).
	KindData
	// KindReverseChannel is a standalone inbound RC burst (ETSI TS 102
	// 361-1 clause 6.4.1).
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
	// Kind is the kind of the burst.
	Kind Kind
	// SyncPattern is the SYNC pattern in the burst center, or
	// enums.SyncNone when the center carries embedded signalling, as in
	// voice bursts B to F and data bursts with an EMB.
	SyncPattern enums.SyncPattern
	// NearestSync is the SYNC pattern nearest to the received center field,
	// whatever the classification. DecodeFromBytes sets it; Encode ignores
	// it.
	NearestSync enums.SyncPattern
	// SyncDistance is the Hamming distance of the center field from
	// NearestSync. DecodeFromBytes sets it; Encode ignores it.
	SyncDistance int
	// VoiceBurst is the superframe position of a voice burst. DecodeFromBytes
	// sets VoiceBurstA or, unless DecodeOptions gives the position,
	// VoiceBurstUnknown; VoiceReceiver sets bursts B to F. Encode ignores it.
	VoiceBurst enums.VoiceBurstType

	// VoiceSlots holds the three 72-bit vocoder slots of a voice burst as
	// received, before any FEC. Encode writes them as they are. AMBEFrames
	// decodes AMBE+2 slots.
	VoiceSlots [VoiceSlotsPerBurst][72]bit.Bit

	// SlotType is the slot type of a data burst.
	SlotType pdu.SlotType

	// BPTCReserved holds the reserved bits R(0) to R(2) of the BPTC(196,96)
	// payload of a data burst, R(i) at index i, after FEC correction. The
	// spec sets them to zero, and Motorola Capacity Max sets R(2) to flag
	// RAS (ETSI TS 102 361-1 clause B.1.1).
	BPTCReserved [3]bit.Bit

	// EmbeddedSignalling is the EMB of a burst with HasEmbeddedSignalling.
	EmbeddedSignalling pdu.EmbeddedSignalling
	// EmbeddedSignallingData is the 32-bit embedded data field of a burst
	// with HasEmbeddedSignalling.
	EmbeddedSignallingData [32]bit.Bit

	// ReverseChannel is the RC PDU of a burst with HasReverseChannel.
	ReverseChannel pdu.ReverseChannel

	// Data is the decoded PDU of a data burst. DecodeFromBytes sets it also
	// when the PDU CRC or FEC fails, and leaves it nil only for a reserved
	// data type. The decoded PDU is a value that the Burst reuses: the next
	// decode into the Burst, or into a copy of it, overwrites it. Copy the
	// PDU to keep it. A Burst and its copies must not decode concurrently.
	Data Data
	// FEC reports the FEC and CRC results of the decode.
	FEC fec.BurstFECStats

	bitData               [264]bit.Bit
	decodedDataType       enums.DataType
	decodedEMB            bool
	deinterleavedInfoBits [196]bit.Bit
	deinterleavedInfoLen  int
	store                 decodeStore
}

// decodeStore holds the PDUs that a Burst decodes into, so that a reused
// Burst decodes without allocating. Each PDU is allocated on the first decode
// of its data type.
type decodeStore struct {
	csbk       *pdu.CSBK
	lc         *pdu.FullLinkControl
	piHeader   *pdu.PIHeader
	dataHeader *pdu.DataHeader
	rate12     *pdu.Rate12Data
	rate34     *pdu.Rate34Data
	rate1      *pdu.Rate1Data
	mbcHeader  *pdu.MBCHeader
	mbcCont    *pdu.MBCContinuation
	usbd       *pdu.UnifiedSingleBlockData
}

// storeSlot returns *p, after it allocates *p when it is nil.
func storeSlot[T any](p **T) *T {
	if *p == nil {
		*p = new(T)
	}
	return *p
}

// HasEmbeddedSignalling reports whether the burst carries an EMB and an
// embedded data field: a voice or data burst without a SYNC pattern, or a
// standalone RC burst.
func (b *Burst) HasEmbeddedSignalling() bool {
	return b.Kind == KindReverseChannel || (b.Kind == KindVoice || b.Kind == KindData) && b.SyncPattern == enums.SyncNone
}

// HasReverseChannel reports whether the embedded data field carries an RC
// PDU: in a standalone RC burst, and in a voice or data burst whose EMB has
// PI set and LCSS single fragment (ETSI TS 102 361-1 clause 6.4).
func (b *Burst) HasReverseChannel() bool {
	if b.Kind == KindReverseChannel {
		return true
	}
	return b.HasEmbeddedSignalling() && b.EmbeddedSignalling.PreemptionAndPowerControlIndicator &&
		b.EmbeddedSignalling.LCSS == enums.LCSSSingleFragment
}

// NewBurstFromBytes decodes data into a new Burst, as DecodeFromBytes does.
// It returns the burst also when it returns an error.
func NewBurstFromBytes(data RawBurst) (*Burst, error) {
	burst := &Burst{}
	err := burst.DecodeFromBytes(data)
	return burst, err
}

// DecodeFromBytes decodes data into the burst in place. It is
// DecodeFromBytesWith with nil options. When it returns no error, a decode
// into a reused Burst does not allocate, except for a CSBK, Full LC, data
// header or MBC header whose dispatch field points to a payload struct.
func (b *Burst) DecodeFromBytes(data RawBurst) error {
	return b.DecodeFromBytesWith(data, nil)
}

// DecodeFromBytesWith decodes data into the burst with receive-side options;
// a nil opts means the zero DecodeOptions. An FEC or CRC failure is not an
// error, because FEC reports it. The error is ErrReservedDataType,
// ErrColorCodeMismatch, an error for invalid options, or a join of these,
// and the burst is populated also when an error is returned.
func (b *Burst) DecodeFromBytesWith(data RawBurst, opts *DecodeOptions) error {
	if opts == nil {
		opts = &DecodeOptions{}
	}
	*b = Burst{store: b.store}
	b.bitData = bit.Unpack264(data)
	b.NearestSync, b.SyncDistance = b.nearestSync(allSyncPatterns)
	err := b.decode(opts)
	ccErr := b.checkColorCode(opts.ColorCode)
	switch {
	case err == nil:
		return ccErr
	case ccErr == nil:
		return err
	}
	return errors.Join(err, ccErr)
}

func (b *Burst) decode(opts *DecodeOptions) error {
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
		return b.decodeData(opts.DataType, threshold)
	case KindReverseChannel:
		b.decodeStandaloneRC()
		return nil
	case KindUnknown:
	default:
		return fmt.Errorf("unknown burst kind %d", opts.Kind)
	}

	switch {
	case distance > threshold && b.isEmbeddedDataBurst():
		return b.decodeData(nil, threshold)
	case distance > threshold:
		b.decodeVoice(enums.VoiceBurstUnknown, enums.SyncNone)
	case isDataSync(nearest):
		return b.decodeData(nil, threshold)
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
		b.setEmbedded(parseEmbedded(b.bitData))
	}
	b.VoiceSlots = parseVoiceSlots(b.bitData)
}

// setEmbedded sets the EMB and embedded data field of a burst with
// HasEmbeddedSignalling, and the RC PDU when the field carries one.
func (b *Burst) setEmbedded(emb pdu.EmbeddedSignalling, data [32]bit.Bit) {
	b.EmbeddedSignalling, b.EmbeddedSignallingData = emb, data
	b.FEC.EMB = emb.FEC
	if b.HasReverseChannel() {
		b.ReverseChannel = DecodeEmbeddedRC(data)
		b.FEC.RC = b.ReverseChannel.FEC
	}
}

// decodeData decodes a data burst. The center is the nearest data SYNC
// pattern, unless that is more than threshold and damagedSyncDistance bits
// off and the center decodes as an EMB with the color code of the slot
// type: then the burst carries embedded signalling.
func (b *Burst) decodeData(dataType *enums.DataType, threshold int) error {
	b.Kind = KindData
	b.SlotType = parseSlotType(b.bitData)
	b.FEC.SlotType = b.SlotType.FEC
	var distance int
	b.SyncPattern, distance = b.nearestSync(dataSyncPatterns)
	if distance > threshold && distance > damagedSyncDistance {
		emb, data := parseEmbedded(b.bitData)
		if !emb.FEC.Uncorrectable && emb.ColorCode == b.SlotType.ColorCode {
			b.SyncPattern = enums.SyncNone
			b.decodedEMB = true
			b.setEmbedded(emb, data)
		}
	}
	if dataType != nil {
		b.SlotType.DataType = *dataType
	}
	b.deinterleavedInfoLen, b.FEC.Payload = b.deinterleave(extractDataBits(b.bitData), b.SlotType.DataType)
	var err error
	b.Data, b.FEC.PDU, err = decodePayload(&b.store, b.SlotType.DataType, b.deinterleavedInfoBits[:b.deinterleavedInfoLen], b.SyncPattern)
	b.decodedDataType = b.SlotType.DataType
	return err
}

// Limits of isEmbeddedDataBurst. The BPTC(196,96) decoder accepts about 9%
// of random blocks, but each with 16 or more corrections, so the BPTC limit
// keeps voice out.
const (
	embeddedDataMaxSlotTypeErrors = 2
	embeddedDataMaxBPTCErrors     = 8
)

// damagedSyncDistance is the largest Hamming distance at which the center of
// a data burst is taken as a data SYNC pattern with bit errors. The
// MS-sourced data SYNC pattern is 2 bits from an EMB codeword, but an EMB
// with random embedded data comes this close to a data SYNC pattern in only
// about 0.14% of bursts.
const damagedSyncDistance = 12

// isEmbeddedDataBurst reports whether a burst without a SYNC pattern is a
// data burst that carries an EMB and embedded data in place of the SYNC
// (ETSI TS 102 361-1 clause 6.4.2).
func (b *Burst) isEmbeddedDataBurst() bool {
	if _, d := b.nearestSync(dataSyncPatterns); d <= damagedSyncDistance {
		return false
	}
	st := parseSlotType(b.bitData)
	if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected > embeddedDataMaxSlotTypeErrors || !st.DataType.IsBPTC() {
		return false
	}
	if emb, _ := parseEmbedded(b.bitData); emb.FEC.Uncorrectable || emb.ColorCode != st.ColorCode {
		return false
	}
	info, res := bptc.Decode196(extractDataBits(b.bitData))
	if res.Uncorrectable || res.ErrorsCorrected > embeddedDataMaxBPTCErrors {
		return false
	}
	p := bit.Pack96(info)
	switch st.DataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		_, lcRes, err := pdu.DecodeFullLCCodeword(p, st.DataType)
		return err == nil && !lcRes.Uncorrectable
	case enums.DataTypePIHeader, enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeDataHeader,
		enums.DataTypeUnifiedSingleBlock:
		return pdu.BlockCRCOK(p, st.DataType)
	case enums.DataTypeMBCContinuation, enums.DataTypeRate12, enums.DataTypeIdle:
		return true
	case enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeReserved:
	}
	return false
}

// opcode0x38Hint returns the likely meaning of CSBK opcode 0x38 in a burst
// with SYNC pattern sync: an MS sends BS_Dwn_Act and a TSCC sends TD_GRANT_MI.
// Only a BS sends a data burst with an EMB in place of the SYNC (ETSI TS 102
// 361-1 clause 6.4.2).
func opcode0x38Hint(sync enums.SyncPattern) pdu.Opcode0x38Hint {
	switch sync {
	case enums.SyncMSSourcedData:
		return pdu.Opcode0x38BSOutboundActivation
	case enums.SyncBSSourcedData, enums.SyncNone:
		return pdu.Opcode0x38TalkgroupDataGrantMultiItem
	case enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncMSSourcedRC, enums.SyncTDMA1Voice,
		enums.SyncTDMA1Data, enums.SyncTDMA2Voice, enums.SyncTDMA2Data, enums.SyncReserved:
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

// decodeStandaloneRC decodes a standalone inbound RC burst. An RC burst
// always carries an RC PDU, so PI does not gate RC decoding (ETSI TS 102
// 361-1 clause 6.4.1, Table E.12).
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

// AMBEFrames decodes the three voice slots as AMBE+2 frames with
// vocoder.DecodeAMBE72 and returns them with the combined FEC result. The
// result is meaningful only for a burst that carries AMBE+2 voice.
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
		// Rate 1 transmit bit order (ETSI TS 102 361-1 Table B.10B).
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
		decoded, reserved, result := bptc.Decode196WithReserved(bits)
		copy(b.deinterleavedInfoBits[:], decoded[:])
		b.BPTCReserved = reserved
		return len(decoded), result
	case enums.DataTypeReserved:
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
	default:
		copy(b.deinterleavedInfoBits[:], bits[:])
		return len(bits), fec.Result{}
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
// and SYNC pattern sync into st. The fec.Result is zero for the data blocks,
// whose only code is the payload code.
func decodePayload(st *decodeStore, dt enums.DataType, infoBits []bit.Bit, sync enums.SyncPattern) (Data, fec.Result, error) {
	if dt.IsReserved() {
		return nil, fec.Result{}, reservedDataTypeError{dt}
	}
	var bits96 [96]bit.Bit
	copy(bits96[:], infoBits)
	switch dt {
	case enums.DataTypeCSBK:
		d := storeSlot(&st.csbk)
		*d = pdu.DecodeCSBK(bits96)
		d.Hint0x38 = opcode0x38Hint(sync)
		return d, d.FEC, nil
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		d := storeSlot(&st.lc)
		*d = pdu.DecodeFullLinkControl(bits96, dt)
		return d, d.FEC, nil
	case enums.DataTypePIHeader:
		d := storeSlot(&st.piHeader)
		*d = pdu.DecodePIHeader(bits96)
		return d, d.FEC, nil
	case enums.DataTypeDataHeader:
		d := storeSlot(&st.dataHeader)
		*d = pdu.DecodeDataHeader(bits96)
		return d, d.FEC, nil
	case enums.DataTypeRate34:
		var sizedBits [144]bit.Bit
		copy(sizedBits[:], infoBits)
		d := storeSlot(&st.rate34)
		*d = pdu.DecodeRate34Data(sizedBits)
		return d, fec.Result{}, nil
	case enums.DataTypeRate12:
		d := storeSlot(&st.rate12)
		*d = pdu.DecodeRate12Data(bits96)
		return d, fec.Result{}, nil
	case enums.DataTypeRate1:
		var sizedBits [192]bit.Bit
		copy(sizedBits[:], infoBits)
		d := storeSlot(&st.rate1)
		*d = pdu.DecodeRate1Data(sizedBits)
		return d, fec.Result{}, nil
	case enums.DataTypeMBCHeader:
		d := storeSlot(&st.mbcHeader)
		*d = pdu.DecodeMBCHeader(bits96)
		return d, d.FEC, nil
	case enums.DataTypeMBCContinuation:
		d := storeSlot(&st.mbcCont)
		*d = pdu.DecodeMBCContinuation(bits96)
		return d, fec.Result{}, nil
	case enums.DataTypeIdle:
		return &pdu.PRFill{}, fec.Result{}, nil
	case enums.DataTypeUnifiedSingleBlock:
		d := storeSlot(&st.usbd)
		*d = pdu.DecodeUnifiedSingleBlockData(bits96)
		return d, d.FEC, nil
	case enums.DataTypeReserved:
	}
	return nil, fec.Result{}, reservedDataTypeError{dt}
}

// Encode returns the burst in transmit bit order, as the package doc
// describes. It returns an error for KindUnknown, a SyncPattern that does not
// fit the kind, a missing Data, a Data whose data type is not
// SlotType.DataType, or a field value that does not fit its bits.
func (b *Burst) Encode() (RawBurst, error) {
	if b.Kind == KindReverseChannel {
		bitData, err := b.encodeStandaloneRC()
		if err != nil {
			return RawBurst{}, err
		}
		return bit.Pack264(bitData), nil
	}
	if err := b.checkCentre(); err != nil {
		return RawBurst{}, err
	}

	var bitData [264]bit.Bit

	if b.Kind == KindData {
		slotBits, err := b.SlotType.Encode()
		if err != nil {
			return RawBurst{}, fmt.Errorf("cannot encode slot type: %w", err)
		}
		copy(bitData[98:108], slotBits[:10])
		copy(bitData[156:166], slotBits[10:20])

		dataBits, err := b.encodeDataBits()
		if err != nil {
			return RawBurst{}, err
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
			return RawBurst{}, err
		}
		esBits, err := b.EmbeddedSignalling.Encode()
		if err != nil {
			return RawBurst{}, fmt.Errorf("cannot encode embedded signalling: %w", err)
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
		if b.SyncPattern == enums.SyncNone && !b.decodedEMB {
			return errors.New("a data burst needs a data SYNC pattern; only a decoded data burst with an EMB has SyncNone")
		}
		if b.SyncPattern == enums.SyncNone && b.EmbeddedSignalling.ColorCode != b.SlotType.ColorCode {
			return fmt.Errorf("a data burst with an EMB needs the color code of its slot type: EMB %d, slot type %d",
				b.EmbeddedSignalling.ColorCode, b.SlotType.ColorCode)
		}
		if b.SyncPattern == enums.SyncNone || isDataSync(b.SyncPattern) {
			return nil
		}
	case KindUnknown, KindReverseChannel:
		return fmt.Errorf("burst kind %s does not encode", b.Kind)
	default:
		return fmt.Errorf("unknown burst kind %d", b.Kind)
	}
	return fmt.Errorf("a %s burst cannot carry SYNC pattern %s", b.Kind, b.SyncPattern)
}

// dataInfoBits returns Data coded for the slot type data type or, when Data
// is nil or codes to the same bits, the payload that DecodeFromBytes decoded.
func (b *Burst) dataInfoBits() ([196]bit.Bit, error) {
	dt := b.SlotType.DataType
	decoded := b.deinterleavedInfoLen > 0 && dt == b.decodedDataType
	switch {
	case b.Data != nil:
		info, err := encodeData(b.Data, dt)
		if err != nil || !decoded {
			return info, err
		}
		var st decodeStore
		orig, _, decodeErr := decodePayload(&st, dt, b.deinterleavedInfoBits[:b.deinterleavedInfoLen], b.SyncPattern)
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
	case dataType.IsBPTC():
		return 96
	default:
		return 0
	}
}

// encodeDataBits codes the information bits of a data burst with
// BPTC(196,96), the Rate 3/4 trellis or Rate 1 as the slot type data type
// selects. R(3) and the Rate 1 pad bits are zero.
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
		// Rate 1 transmit bit order (ETSI TS 102 361-1 Table B.10B).
		var bits [196]bit.Bit
		copy(bits[:96], info[:96])
		copy(bits[100:], info[96:192])
		return bits, nil
	case dt.IsBPTC():
		var infoBits [96]bit.Bit
		copy(infoBits[:], info[:96])
		return bptc.Encode196WithReserved(infoBits, b.BPTCReserved), nil
	default:
		return info, nil
	}
}
