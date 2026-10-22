package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
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

// String returns the name of the kind. It allocates only for a value without
// a name.
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
	return "Kind(" + text.Int(int64(k)) + ")"
}

// Burst represents a DMR burst. A decode stores the PDU of a data burst in
// the Burst, so a Burst must not be copied after its first decode; go vet
// reports a copy. The zero Burst is ready to use.
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
	// received, before any FEC, packed as RawBurst.VoiceSlots returns them.
	// Encode writes them as they are. AMBEFrames decodes AMBE+2 slots.
	VoiceSlots [VoiceSlotsPerBurst][9]byte

	// SlotType is the slot type of a data burst.
	SlotType pdu.SlotType

	// BPTCReserved holds the reserved bits R(0) to R(2) of the BPTC(196,96)
	// payload of a data burst after FEC correction, R(i) in bit i, so that
	// it reads R(2), R(1), R(0) in the order of the BPTC matrix, the first
	// most significant. Encode returns a *dmrgo.FieldRangeError for a data
	// burst when one of bits 3 to 7 is set. The spec sets R(0) to R(2) to
	// zero, and Motorola Capacity Max sets R(2), 0x4, to flag RAS (ETSI TS
	// 102 361-1 clause B.1.1).
	BPTCReserved uint8

	// EmbeddedSignalling is the EMB of a burst with HasEmbeddedSignalling.
	EmbeddedSignalling pdu.EmbeddedSignalling
	// EmbeddedSignallingData is the 32-bit embedded data field of a burst
	// with HasEmbeddedSignalling, packed as RawBurst.EmbeddedData returns it.
	EmbeddedSignallingData [4]byte

	// ReverseChannel is the RC PDU of a burst with HasReverseChannel.
	ReverseChannel pdu.ReverseChannel

	// Data is the decoded PDU of a data burst, nil only for a reserved data
	// type. It points into the Burst, which reuses the PDU: the next decode
	// overwrites it, so copy it to keep it.
	Data Data
	// FEC reports the FEC and CRC results of the decode.
	FEC fec.BurstFECStats

	raw             RawBurst
	decodedDataType enums.DataType
	decodedEMB      bool
	payload         [25]byte
	payloadBits     int
	decodedCoded    [12]byte
	hasDecodedCoded bool
	store           decodeStore
	noCopy          noCopy
}

type decodeStore struct {
	csbk       pdu.CSBK
	lc         pdu.FullLinkControl
	piHeader   pdu.PIHeader
	dataHeader pdu.DataHeader
	rate12     pdu.Rate12Data
	rate34     pdu.Rate34Data
	rate1      pdu.Rate1Data
	mbcHeader  pdu.MBCHeader
	mbcCont    pdu.MBCContinuation
	usbd       pdu.UnifiedSingleBlockData
	prFill     pdu.PRFill
}

func (b *Burst) reset(data RawBurst) {
	b.Kind = KindUnknown
	b.SyncPattern, b.NearestSync, b.SyncDistance = 0, 0, 0
	b.VoiceBurst = 0
	b.VoiceSlots = [VoiceSlotsPerBurst][9]byte{}
	b.SlotType = pdu.SlotType{}
	b.BPTCReserved = 0
	b.EmbeddedSignalling = pdu.EmbeddedSignalling{}
	b.EmbeddedSignallingData = [4]byte{}
	b.ReverseChannel = pdu.ReverseChannel{}
	b.Data = nil
	b.FEC = fec.BurstFECStats{}
	b.raw = data
	b.decodedDataType, b.decodedEMB = 0, false
	b.payload, b.payloadBits = [25]byte{}, 0
	b.decodedCoded, b.hasDecodedCoded = [12]byte{}, false
}

// HasEmbeddedSignalling reports whether the burst carries an EMB and an
// embedded data field: a voice or data burst without a SYNC pattern, or a
// standalone RC burst.
func (b *Burst) HasEmbeddedSignalling() bool {
	if b == nil {
		return false
	}
	return b.Kind == KindReverseChannel || (b.Kind == KindVoice || b.Kind == KindData) && b.SyncPattern == enums.SyncNone
}

// HasReverseChannel reports whether the embedded data field carries an RC
// PDU: in a standalone RC burst, and in a voice or data burst whose EMB has
// PI set and LCSS single fragment (ETSI TS 102 361-1 clause 6.4).
func (b *Burst) HasReverseChannel() bool {
	if b == nil {
		return false
	}
	if b.Kind == KindReverseChannel {
		return true
	}
	return b.HasEmbeddedSignalling() && b.EmbeddedSignalling.PreemptionAndPowerControlIndicator &&
		b.EmbeddedSignalling.LCSS == enums.LCSSSingleFragment
}

// NewBurst returns a new Burst. A Burst holds the storage of every PDU that
// a decode fills, so no decode into it allocates. It allocates the Burst.
func NewBurst() *Burst {
	return new(Burst)
}

// NewBurstFromBytes decodes data into a new Burst, as DecodeFromBytes does.
// It returns the burst also when it returns an error. It allocates the
// Burst.
func NewBurstFromBytes(data RawBurst) (*Burst, error) {
	burst := NewBurst()
	err := burst.DecodeFromBytes(data)
	return burst, err
}

// DecodeFromBytes decodes data into the burst in place. It is
// DecodeFromBytesWith with nil options. When it returns no error, a decode
// into a reused Burst does not allocate.
func (b *Burst) DecodeFromBytes(data RawBurst) error {
	if b == nil {
		return dmrgo.ErrNilReceiver
	}
	return b.DecodeFromBytesWith(data, nil)
}

// DecodeFromBytesWith decodes data into the burst with receive-side options;
// a nil opts means the zero DecodeOptions. An FEC or CRC failure is not an
// error, because FEC reports it. The error is ErrReservedDataType,
// ErrColorCodeMismatch, an error for invalid options, or a join of these,
// and the burst is populated also when an error is returned.
func (b *Burst) DecodeFromBytesWith(data RawBurst, opts *DecodeOptions) error {
	if b == nil {
		return dmrgo.ErrNilReceiver
	}
	var o DecodeOptions
	if opts != nil {
		o = *opts
	}
	b.reset(data)
	b.NearestSync, b.SyncDistance = b.nearestSync(allSyncPatterns)
	err := b.decode(o)
	ccErr := b.checkColorCode(o.ColorCode)
	switch {
	case err == nil:
		return ccErr
	case ccErr == nil:
		return err
	}
	return joinColorCode(err)
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
		return b.decodeData(opts.DataType, threshold)
	case KindReverseChannel:
		b.decodeStandaloneRC()
		return nil
	case KindUnknown:
	default:
		return errUnknownKind
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
		return ErrVoicePosition
	}
	return nil
}

func (b *Burst) decodeVoice(pos enums.VoiceBurstType, sync enums.SyncPattern) {
	b.Kind, b.SyncPattern, b.VoiceBurst = KindVoice, sync, pos
	if sync == enums.SyncNone {
		b.setEmbedded(b.raw.EMB())
	}
	b.VoiceSlots = b.raw.VoiceSlots()
}

func (b *Burst) setEmbedded(emb pdu.EmbeddedSignalling) {
	data := b.raw.EmbeddedData()
	b.EmbeddedSignalling = emb
	b.EmbeddedSignallingData = data
	b.FEC.EMB = emb.FEC
	if b.HasReverseChannel() {
		b.ReverseChannel = DecodeEmbeddedRC(data)
		b.FEC.RC = b.ReverseChannel.FEC
	}
}

func (b *Burst) decodeData(dataType *enums.DataType, threshold int) error {
	b.Kind = KindData
	b.SlotType = b.raw.SlotType()
	b.FEC.SlotType = b.SlotType.FEC
	var distance int
	b.SyncPattern, distance = b.nearestSync(dataSyncPatterns)
	if distance > threshold && distance > damagedSyncDistance {
		emb := b.raw.EMB()
		if !emb.FEC.Uncorrectable && emb.ColorCode == b.SlotType.ColorCode {
			b.SyncPattern = enums.SyncNone
			b.decodedEMB = true
			b.setEmbedded(emb)
		}
	}
	if dataType != nil {
		b.SlotType.DataType = *dataType
	}
	b.payloadBits, b.FEC.Payload = b.deinterleave(b.raw.Info(), b.SlotType.DataType)
	var err error
	b.Data, b.FEC.PDU, err = decodePayload(&b.store, b.SlotType.DataType, &b.payload, b.SyncPattern)
	b.decodedDataType = b.SlotType.DataType
	var encErr error
	switch d := b.Data.(type) {
	case *pdu.CSBK:
		b.decodedCoded, encErr = d.Encode()
	case *pdu.FullLinkControl:
		b.decodedCoded, encErr = d.Encode()
	case *pdu.DataHeader:
		b.decodedCoded, encErr = d.Encode()
	case *pdu.MBCHeader:
		b.decodedCoded, encErr = d.Encode()
	default:
		return err
	}
	b.hasDecodedCoded = encErr == nil
	return err
}

const (
	embeddedDataMaxSlotTypeErrors = 2
	embeddedDataMaxBPTCErrors     = 8
)

const damagedSyncDistance = 12

// ETSI TS 102 361-1 clause 6.4.2
func (b *Burst) isEmbeddedDataBurst() bool {
	if _, d := b.nearestSync(dataSyncPatterns); d <= damagedSyncDistance {
		return false
	}
	st := b.raw.SlotType()
	if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected > embeddedDataMaxSlotTypeErrors || !st.DataType.IsBPTC() {
		return false
	}
	if emb := b.raw.EMB(); emb.FEC.Uncorrectable || emb.ColorCode != st.ColorCode {
		return false
	}
	p, _, res := bptc.Decode196(b.raw.Info())
	if res.Uncorrectable || res.ErrorsCorrected > embeddedDataMaxBPTCErrors {
		return false
	}
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

// ETSI TS 102 361-1 clause 6.4.2
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

// ETSI TS 102 361-1 clause 6.4.1, Table E.12
func (b *Burst) decodeStandaloneRC() {
	b.Kind, b.SyncPattern = KindReverseChannel, enums.SyncMSSourcedRC
	b.EmbeddedSignalling = b.raw.StandaloneRCEMB()
	b.FEC.EMB = b.EmbeddedSignalling.FEC

	data := b.raw.StandaloneRCData()
	b.EmbeddedSignallingData = data
	b.ReverseChannel = DecodeEmbeddedRC(data)
	b.FEC.RC = b.ReverseChannel.FEC
}

func (b *Burst) encodeStandaloneRC() (RawBurst, error) {
	var r RawBurst
	data, err := b.embeddedData()
	if err != nil {
		return r, err
	}
	emb, err := b.embCodeword()
	if err != nil {
		return r, err
	}
	r.setField(rcembFirstStart, 8, rcembSecondStart, 8, uint64(emb))
	r.setWord32(data, rcDataFirstStart, rcDataSecondStart)
	r.setSync(enums.SyncMSSourcedRC)
	return r, nil
}

func (b *Burst) embCodeword() (uint16, error) {
	emb, err := b.EmbeddedSignalling.Encode()
	if err != nil {
		return 0, err
	}
	return uint16(emb[0])<<8 | uint16(emb[1]), nil
}

func (b *Burst) embeddedData() ([4]byte, error) {
	if !b.HasReverseChannel() {
		return b.EmbeddedSignallingData, nil
	}
	d, err := EncodeEmbeddedRC(&b.ReverseChannel)
	if err != nil {
		return d, err
	}
	return d, nil
}

// AMBEFrames decodes the three voice slots as AMBE+2 frames with
// vocoder.DecodeAMBE72 and returns them with the combined FEC result. The
// result is meaningful only for a burst that carries AMBE+2 voice.
func (b *Burst) AMBEFrames() ([VoiceSlotsPerBurst]vocoder.AMBEFrame, fec.Result) {
	if b == nil {
		return [VoiceSlotsPerBurst]vocoder.AMBEFrame{}, fec.Result{}
	}
	var frames [VoiceSlotsPerBurst]vocoder.AMBEFrame
	var result fec.Result
	for i, slot := range b.VoiceSlots {
		f, r := vocoder.DecodeAMBE72(slot)
		frames[i] = f
		result = result.Combine(r)
	}
	return frames, result
}

// SetAMBEFrames sets the three voice slots to the AMBE+2 coding of frames,
// as vocoder.EncodeAMBE72 returns it. It returns the errors of
// EncodeAMBE72 and then changes nothing.
func (b *Burst) SetAMBEFrames(frames [VoiceSlotsPerBurst]vocoder.AMBEFrame) error {
	if b == nil {
		return dmrgo.ErrNilReceiver
	}
	var s [VoiceSlotsPerBurst][9]byte
	for i := range frames {
		var err error
		if s[i], err = vocoder.EncodeAMBE72(frames[i]); err != nil {
			return err
		}
	}
	b.VoiceSlots = s
	return nil
}

func (b *Burst) deinterleave(info [25]byte, dataType enums.DataType) (int, fec.Result) {
	switch dataType {
	case enums.DataTypeRate34:
		decoded, result := trellis.Decode(info)
		copy(b.payload[:], decoded[:])
		return 144, result
	case enums.DataTypeRate1:
		copy(b.payload[:12], info[:12])
		bits.Copy(b.payload[:], 96, info[:], 100, 96)
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
		decoded, reserved, result := bptc.Decode196(info)
		copy(b.payload[:], decoded[:])
		b.BPTCReserved = reserved
		return 96, result
	case enums.DataTypeReserved:
	}
	b.payload = info
	return 196, fec.Result{}
}

// ETSI TS 102 361-1 Table B.10B
func rate1Info(data *[25]byte) [25]byte {
	var info [25]byte
	copy(info[:12], data[:12])
	bits.Copy(info[:], 100, data[:], 96, 96)
	return info
}

// String returns a string representation of the burst. It allocates the
// string.
func (b *Burst) String() string {
	if b == nil {
		return "<nil>"
	}
	ret := "{ Kind: " + b.Kind.String() + ", SyncPattern: " + b.SyncPattern.String() + ", "
	if b.HasEmbeddedSignalling() {
		ret += "EmbeddedSignalling: " + b.EmbeddedSignalling.String() + ", "
		if b.HasReverseChannel() {
			ret += "ReverseChannel: " + b.ReverseChannel.String() + ", "
		}
	}
	switch b.Kind {
	case KindData:
		agg := b.FEC.Aggregate()
		ret += "SlotType: " + b.SlotType.String() +
			", FEC: {BitsChecked: " + text.Int(int64(agg.BitsChecked)) +
			", ErrorsCorrected: " + text.Int(int64(agg.ErrorsCorrected)) +
			", Uncorrectable: " + text.Bool(agg.Uncorrectable) + "}"
		if b.Data != nil {
			ret += ", Data: " + b.Data.String()
		}
	case KindVoice:
		ret += "VoiceBurst: " + b.VoiceBurst.String()
	case KindUnknown, KindReverseChannel:
		ret += "NearestSync: " + b.NearestSync.String() + ", SyncDistance: " + text.Int(int64(b.SyncDistance))
	}
	return ret + " }"
}

func decodePayload(st *decodeStore, dt enums.DataType, payload *[25]byte, sync enums.SyncPattern) (Data, fec.Result, error) {
	if dt.IsReserved() {
		return nil, fec.Result{}, reservedDataTypeError{dt}
	}
	p96 := [12]byte(payload[:12])
	switch dt {
	case enums.DataTypeCSBK:
		d := &st.csbk
		*d = pdu.DecodeCSBK(p96)
		d.Hint0x38 = opcode0x38Hint(sync)
		return d, d.FEC, nil
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		d := &st.lc
		*d = pdu.DecodeFullLinkControl(p96, dt)
		return d, d.FEC, nil
	case enums.DataTypePIHeader:
		d := &st.piHeader
		*d = pdu.DecodePIHeader(p96)
		return d, d.FEC, nil
	case enums.DataTypeDataHeader:
		d := &st.dataHeader
		*d = pdu.DecodeDataHeader(p96)
		return d, d.FEC, nil
	case enums.DataTypeRate34:
		d := &st.rate34
		*d = pdu.DecodeRate34Data([18]byte(payload[:18]))
		return d, fec.Result{}, nil
	case enums.DataTypeRate12:
		d := &st.rate12
		*d = pdu.DecodeRate12Data(p96)
		return d, fec.Result{}, nil
	case enums.DataTypeRate1:
		d := &st.rate1
		*d = pdu.DecodeRate1Data([24]byte(payload[:24]))
		return d, fec.Result{}, nil
	case enums.DataTypeMBCHeader:
		d := &st.mbcHeader
		*d = pdu.DecodeMBCHeader(p96)
		return d, d.FEC, nil
	case enums.DataTypeMBCContinuation:
		d := &st.mbcCont
		*d = pdu.DecodeMBCContinuation(p96)
		return d, fec.Result{}, nil
	case enums.DataTypeIdle:
		return &st.prFill, fec.Result{}, nil
	case enums.DataTypeUnifiedSingleBlock:
		d := &st.usbd
		*d = pdu.DecodeUnifiedSingleBlockData(p96)
		return d, d.FEC, nil
	case enums.DataTypeReserved:
	}
	return nil, fec.Result{}, reservedDataTypeError{dt}
}

// Encode returns the burst in transmit bit order, as the package doc
// describes. It returns an error for KindUnknown, a SyncPattern that does
// not fit the kind, a missing Data, or a Data whose data type is not
// SlotType.DataType, and a *dmrgo.FieldRangeError for a field value that does
// not fit its bits or a padding bit that is set. A data burst takes only
// the Data types that BuildBurst takes.
func (b *Burst) Encode() (RawBurst, error) {
	if b == nil {
		return RawBurst{}, dmrgo.ErrNilReceiver
	}
	if b.Kind == KindReverseChannel {
		return b.encodeStandaloneRC()
	}
	if err := b.checkCentre(); err != nil {
		return RawBurst{}, err
	}

	var r RawBurst

	if b.Kind == KindData {
		if b.BPTCReserved>>3 != 0 {
			return RawBurst{}, errBurstBPTCReservedRange
		}
		st, err := b.SlotType.Encode()
		if err != nil {
			return RawBurst{}, err
		}
		r.setField(slotTypeFirstStart, 10, slotTypeSecondStart, 10, uint64(st[0])<<12|uint64(st[1])<<4|uint64(st[2])>>4)

		info, err := b.encodeDataBits()
		if err != nil {
			return RawBurst{}, err
		}
		r.setInfo(&info)
	} else {
		r.SetVoiceSlots(b.VoiceSlots)
	}

	if b.HasEmbeddedSignalling() {
		data, err := b.embeddedData()
		if err != nil {
			return RawBurst{}, err
		}
		emb, err := b.embCodeword()
		if err != nil {
			return RawBurst{}, err
		}
		r.setField(embFirstStart, 8, embSecondStart, 8, uint64(emb))
		r.SetEmbeddedData(data)
	} else {
		r.setSync(b.SyncPattern)
	}

	return r, nil
}

func (b *Burst) checkCentre() error {
	switch b.Kind {
	case KindVoice:
		if b.SyncPattern == enums.SyncNone || b.SyncPattern == enums.SyncReserved || isVoiceSync(b.SyncPattern) {
			return nil
		}
	case KindData:
		if b.SyncPattern == enums.SyncNone && !b.decodedEMB {
			return errDataBurstNoSync
		}
		if b.SyncPattern == enums.SyncNone && b.EmbeddedSignalling.ColorCode != b.SlotType.ColorCode {
			return errEMBColorCode
		}
		if b.SyncPattern == enums.SyncNone || isDataSync(b.SyncPattern) {
			return nil
		}
	case KindUnknown, KindReverseChannel:
		return errKindNoEncode
	default:
		return errUnknownKind
	}
	return errKindSync
}

func (b *Burst) dataInfoBits() ([25]byte, error) {
	dt := b.SlotType.DataType
	decoded := b.payloadBits > 0 && dt == b.decodedDataType
	switch {
	case b.Data != nil:
		_, info, err := encodeData(b.Data, dt, false)
		if err != nil {
			return info, err
		}
		if decoded && info != b.payload && b.decodedCodesTo([12]byte(info[:12])) {
			return b.payload, nil
		}
		return info, nil
	case b.payloadBits == 0:
		return [25]byte{}, errNoData
	case !decoded:
		return [25]byte{}, errPayloadType
	default:
		return b.payload, nil
	}
}

func (b *Burst) decodedCodesTo(want [12]byte) bool {
	switch b.decodedDataType {
	case enums.DataTypeCSBK, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeDataHeader,
		enums.DataTypeMBCHeader:
		return b.hasDecodedCoded && want == b.decodedCoded
	case enums.DataTypePIHeader, enums.DataTypeMBCContinuation, enums.DataTypeUnifiedSingleBlock, enums.DataTypeIdle:
	default:
		return false
	}
	payload := [12]byte(b.payload[:12])
	var coded [12]byte
	var err error
	switch b.decodedDataType {
	case enums.DataTypePIHeader:
		d := pdu.DecodePIHeader(payload)
		coded, err = d.Encode()
	case enums.DataTypeMBCContinuation:
		d := pdu.DecodeMBCContinuation(payload)
		coded, err = d.Encode()
	case enums.DataTypeUnifiedSingleBlock:
		d := pdu.DecodeUnifiedSingleBlockData(payload)
		coded, err = d.Encode()
	default:
		coded = pdu.PRFillBits()
	}
	return err == nil && want == coded
}

func encodeData(d Data, dataType enums.DataType, anyType bool) (enums.DataType, [25]byte, error) {
	var info [25]byte
	dt, ok, err := pduInfo(d, &info)
	switch {
	case !ok:
		return 0, info, err
	case !anyType && dt != dataType:
		return dt, [25]byte{}, errDataPDUType
	case err != nil:
		return dt, [25]byte{}, err
	}
	return dt, info, nil
}

func pduInfo(d Data, info *[25]byte) (enums.DataType, bool, error) {
	switch p := d.(type) {
	case nil:
		return 0, false, errNilData
	case *pdu.Rate34Data:
		if p != nil {
			var c [18]byte
			dt := p.DataType()
			c, err := p.Encode()
			copy(info[:], c[:])
			return dt, true, err
		}
	case *pdu.Rate1Data:
		if p != nil {
			var c [24]byte
			dt := p.DataType()
			c, err := p.Encode()
			copy(info[:], c[:])
			return dt, true, err
		}
	default:
		return bptcInfo(d, info)
	}
	return 0, false, errNilPointerData
}

func bptcInfo(d Data, info *[25]byte) (enums.DataType, bool, error) {
	var dt enums.DataType
	var c [12]byte
	var err error
	switch p := d.(type) {
	case *pdu.FullLinkControl:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.CSBK:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.PIHeader:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.DataHeader:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.MBCHeader:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.MBCContinuation:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.CGAPContinuation:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.MVAPContinuation:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.BCAPContinuation:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.VNAPContinuation:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.UnifiedSingleBlockData:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.Rate12Data:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	case *pdu.PRFill:
		if p == nil {
			return 0, false, errNilPointerData
		}
		dt = p.DataType()
		c, err = p.Encode()
	default:
		return 0, false, errNotPDU
	}
	copy(info[:], c[:])
	return dt, true, err
}

func (b *Burst) encodeDataBits() ([25]byte, error) {
	info, err := b.dataInfoBits()
	if err != nil {
		return [25]byte{}, err
	}
	dt := b.SlotType.DataType
	switch {
	case dt == enums.DataTypeRate34:
		return trellis.Encode([18]byte(info[:18])), nil
	case dt == enums.DataTypeRate1:
		return rate1Info(&info), nil
	case dt.IsBPTC():
		return bptc.Encode196([12]byte(info[:12]), b.BPTCReserved), nil
	default:
		return info, nil
	}
}
