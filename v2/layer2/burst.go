package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
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
	return fmt.Sprintf("Kind(%d)", uint8(k))
}

// Burst represents a DMR burst. A decode stores the PDU of a data burst, and
// the payload struct that it points to, in the Burst, so a Burst must not be
// copied after its first decode; go vet reports a copy.
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
	// payload of a data burst after FEC correction, R(i) in bit i, so that
	// it reads R(2), R(1), R(0) in the order of the BPTC matrix, the first
	// most significant. Encode ignores bits 3 to 7. The spec sets them to
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
	// type. It points into the Burst, which reuses the PDU and its payload
	// struct: the next decode overwrites them, so copy them to keep them.
	Data Data
	// FEC reports the FEC and CRC results of the decode.
	FEC fec.BurstFECStats

	raw             RawBurst
	decodedDataType enums.DataType
	decodedEMB      bool
	// payload holds the payloadBits information bits that the payload code
	// of a decoded data burst returned, packed.
	payload     [25]byte
	payloadBits int
	// decodedCoded is the coding of the decoded CSBK, Full LC, data header or
	// MBC header, for Encode, which must not decode these again: their
	// payload structs would allocate.
	decodedCoded    [12]byte
	hasDecodedCoded bool
	store           decodeStore
	noCopy          noCopy
}

// decodeStore holds the PDUs that a Burst decodes into, and the payload
// structs that their dispatch fields point to, so that a Burst decodes
// without allocating.
type decodeStore struct {
	csbk              pdu.CSBK
	csbkPayloads      pdu.CSBKPayloads
	lc                pdu.FullLinkControl
	lcPayloads        pdu.FullLinkControlPayloads
	dataHeaderPayload pdu.DataHeaderPayloads
	mbcHeaderPayloads pdu.MBCHeaderPayloads
	piHeader          pdu.PIHeader
	dataHeader        pdu.DataHeader
	rate12            pdu.Rate12Data
	rate34            pdu.Rate34Data
	rate1             pdu.Rate1Data
	mbcHeader         pdu.MBCHeader
	mbcCont           pdu.MBCContinuation
	usbd              pdu.UnifiedSingleBlockData
	prFill            pdu.PRFill
}

// reset sets every field of b to its zero value, except the decode store,
// which the next decode overwrites, and sets the raw burst to data. It
// does not copy the store, as *b = Burst{} would.
func (b *Burst) reset(data RawBurst) {
	b.Kind = KindUnknown
	b.SyncPattern, b.NearestSync, b.SyncDistance = 0, 0, 0
	b.VoiceBurst = 0
	b.VoiceSlots = [VoiceSlotsPerBurst][72]bit.Bit{}
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
	return b.DecodeFromBytesWith(data, nil)
}

// DecodeFromBytesWith decodes data into the burst with receive-side options;
// a nil opts means the zero DecodeOptions. An FEC or CRC failure is not an
// error, because FEC reports it. The error is ErrReservedDataType,
// ErrColorCodeMismatch, an error for invalid options, or a join of these,
// and the burst is populated also when an error is returned.
func (b *Burst) DecodeFromBytesWith(data RawBurst, opts *DecodeOptions) error {
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

// decode takes the options by value: under TinyGo a pointer to options
// that DecodeFromBytes makes would otherwise go to the heap when decode is
// not inlined.
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
		return errVoicePosition
	}
	return nil
}

func (b *Burst) decodeVoice(pos enums.VoiceBurstType, sync enums.SyncPattern) {
	b.Kind, b.SyncPattern, b.VoiceBurst = KindVoice, sync, pos
	if sync == enums.SyncNone {
		b.setEmbedded(b.raw.EMB())
	}
	for i, slot := range b.raw.VoiceSlots() {
		unpackBits(b.VoiceSlots[i][:], slot[:])
	}
}

// setEmbedded sets the EMB emb and the embedded data field of a burst with
// HasEmbeddedSignalling, and the RC PDU when the field carries one.
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

// decodeData decodes a data burst. The center is the nearest data SYNC
// pattern, unless that is more than threshold and damagedSyncDistance bits
// off and the center decodes as an EMB with the color code of the slot
// type: then the burst carries embedded signalling.
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
	b.payloadBits, b.FEC.Payload = b.deinterleave(b.raw.info(), b.SlotType.DataType)
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
	st := b.raw.SlotType()
	if st.FEC.Uncorrectable || st.FEC.ErrorsCorrected > embeddedDataMaxSlotTypeErrors || !st.DataType.IsBPTC() {
		return false
	}
	if emb := b.raw.EMB(); emb.FEC.Uncorrectable || emb.ColorCode != st.ColorCode {
		return false
	}
	p, _, res := bptc.Decode196(b.raw.info())
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

// embCodeword returns the QR(16,7) codeword of EmbeddedSignalling, or an
// error when one of its fields does not fit.
func (b *Burst) embCodeword() (uint16, error) {
	emb, err := b.EmbeddedSignalling.Encode()
	if err != nil {
		return 0, err
	}
	return uint16(emb[0])<<8 | uint16(emb[1]), nil
}

// embeddedData returns the 32 bits of the embedded signalling data field,
// packed: the coded ReverseChannel when HasReverseChannel reports true, and
// EmbeddedSignallingData otherwise.
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

// deinterleave decodes the packed information bits info of a data burst of
// dataType into b.payload with the payload code, and returns the number of
// payload bits and the code's result.
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

// rate1Info returns the 196 information bits of a Rate 1 burst that carries
// the 192 data bits of data, with bits 96 to 99 zero (ETSI TS 102 361-1
// Table B.10B).
func rate1Info(data *[25]byte) [25]byte {
	var info [25]byte
	copy(info[:12], data[:12])
	bits.Copy(info[:], 100, data[:], 96, 96)
	return info
}

// String returns a string representation of the burst. It allocates the
// string.
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

// decodePayload decodes the packed information bits payload of a data burst
// of data type dt and SYNC pattern sync into st. The fec.Result is zero for
// the data blocks, whose only code is the payload code.
func decodePayload(st *decodeStore, dt enums.DataType, payload *[25]byte, sync enums.SyncPattern) (Data, fec.Result, error) {
	if dt.IsReserved() {
		return nil, fec.Result{}, reservedDataTypeError{dt}
	}
	p96 := [12]byte(payload[:12])
	switch dt {
	case enums.DataTypeCSBK:
		d := &st.csbk
		pdu.DecodeCSBKInto(d, &st.csbkPayloads, p96)
		d.Hint0x38 = opcode0x38Hint(sync)
		return d, d.FEC, nil
	case enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC:
		d := &st.lc
		pdu.DecodeFullLinkControlInto(d, &st.lcPayloads, p96, dt)
		return d, d.FEC, nil
	case enums.DataTypePIHeader:
		d := &st.piHeader
		*d = pdu.DecodePIHeader(p96)
		return d, d.FEC, nil
	case enums.DataTypeDataHeader:
		d := &st.dataHeader
		pdu.DecodeDataHeaderInto(d, &st.dataHeaderPayload, p96)
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
		pdu.DecodeMBCHeaderInto(d, &st.mbcHeaderPayloads, p96)
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
// not fit the kind, a missing Data, a Data whose data type is not
// SlotType.DataType, or a field value that does not fit its bits. For a data
// burst, its type switch on the Encode method of Data can allocate once for
// each type of Data, to fill the runtime cache of the switch.
func (b *Burst) Encode() (RawBurst, error) {
	if b.Kind == KindReverseChannel {
		return b.encodeStandaloneRC()
	}
	if err := b.checkCentre(); err != nil {
		return RawBurst{}, err
	}

	var r RawBurst

	if b.Kind == KindData {
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
		var slots [VoiceSlotsPerBurst][9]byte
		for i := range slots {
			packBits(slots[i][:], b.VoiceSlots[i][:])
		}
		r.SetVoiceSlots(slots)
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

// dataInfoBits returns, packed, Data coded for the slot type data type or,
// when Data is nil or codes to the same bits as the PDU that DecodeFromBytes
// decoded, the payload that DecodeFromBytes decoded.
func (b *Burst) dataInfoBits() ([25]byte, error) {
	dt := b.SlotType.DataType
	decoded := b.payloadBits > 0 && dt == b.decodedDataType
	switch {
	case b.Data != nil:
		info, err := encodeData(b.Data, dt)
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

// decodedCodesTo reports whether the PDU that DecodeFromBytes decoded from
// the payload codes to the 96 packed information bits want. A Rate 1/2,
// Rate 3/4 or Rate 1 PDU keeps every information bit, so it codes to the
// payload, which the caller compares.
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

// encoder96, encoder144 and encoder192 are the PDUs whose Encode returns
// their information bits as an array, which does not allocate.
type (
	encoder96  interface{ Encode() ([12]byte, error) }
	encoder144 interface{ Encode() ([18]byte, error) }
	encoder192 interface{ Encode() ([24]byte, error) }
)

// encodeData returns the information bits of d in a data burst of
// dataType, packed.
func encodeData(d Data, dataType enums.DataType) ([25]byte, error) {
	var info [25]byte
	if err := checkData(d, dataType); err != nil {
		return info, err
	}
	var n int
	var err error
	switch e := d.(type) {
	case encoder96:
		var coded [12]byte
		coded, err = e.Encode()
		n = copy(info[:], coded[:]) * 8
	case encoder144:
		var coded [18]byte
		coded, err = e.Encode()
		n = copy(info[:], coded[:]) * 8
	case encoder192:
		var coded [24]byte
		coded, err = e.Encode()
		n = copy(info[:], coded[:]) * 8
	default:
		return info, errNoEncode
	}
	if err != nil {
		return info, err
	}
	if want := infoBitsLen(dataType); n != want {
		return info, errInfoBits
	}
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
// selects, and returns the 196 coded bits packed. R(3) and the Rate 1 pad
// bits are zero.
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
		return bptc.Encode196([12]byte(info[:12]), b.BPTCReserved&0x7), nil
	default:
		return info, nil
	}
}
