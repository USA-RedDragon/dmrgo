package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// RawBurst is a 33-octet burst in transmit bit order, the form
// NewBurstFromBytes takes. Its methods read or write one field and leave
// every other bit as it is; they do not classify the burst. The getters have
// value receivers and the setters have pointer receivers.
// (ETSI TS 102 361-1 Annex E)
type RawBurst [33]byte

func (r *RawBurst) field(first, a, second, b int) uint64 {
	return bits.Field(r[:], first, a)<<b | bits.Field(r[:], second, b)
}

func (r *RawBurst) setField(first, a, second, b int, v uint64) {
	bits.SetField(r[:], first, a, v>>b)
	bits.SetField(r[:], second, b, v)
}

// Sync returns the SYNC pattern nearest to burst bits 108 to 155 and its
// Hamming distance, as enums.NearestSyncPattern does.
func (r RawBurst) Sync() (enums.SyncPattern, int) {
	v := bits.Field(r[:], syncStart, syncEnd-syncStart)
	return enums.NearestSyncPattern([6]byte{byte(v >> 40 & 0xFF), byte(v >> 32 & 0xFF), byte(v >> 24 & 0xFF), byte(v >> 16 & 0xFF), byte(v >> 8 & 0xFF), byte(v & 0xFF)})
}

// SetSync writes the SYNC pattern p into burst bits 108 to 155. It returns
// an error unless p is a SYNC pattern (ETSI TS 102 361-1 Table 9.2).
func (r *RawBurst) SetSync(p enums.SyncPattern) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	if !isSyncPattern(p) {
		return errSyncPattern
	}
	r.setSync(p)
	return nil
}

func (r *RawBurst) setSync(p enums.SyncPattern) {
	bits.SetField(r[:], syncStart, syncEnd-syncStart, uint64(p))
}

// SlotType decodes the Golay(20,8) slot type in burst bits 98 to 107 and
// 156 to 165. Its FEC field reports the Golay result.
func (r RawBurst) SlotType() pdu.SlotType {
	info, res := pdu.DecodeSlotTypeCodeword(uint32(r.field(slotTypeFirstStart, 10, slotTypeSecondStart, 10) & 0xFFFFF))
	st := info.SlotType()
	st.FEC = res
	return st
}

// SetSlotType encodes st into the slot type bits. It returns the errors of
// st.Encode and then changes nothing.
func (r *RawBurst) SetSlotType(st pdu.SlotType) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	cw, err := st.Encode()
	if err != nil {
		return err
	}
	r.setField(slotTypeFirstStart, 10, slotTypeSecondStart, 10, uint64(cw[0])<<12|uint64(cw[1])<<4|uint64(cw[2])>>4)
	return nil
}

func (r *RawBurst) info() [25]byte {
	var p [25]byte
	copy(p[:12], r[:12])
	bits.Copy(p[:], 96, r[:], 96, 2)
	bits.Copy(p[:], infoFirstEnd, r[:], infoSecondStart, infoSecondEnd-infoSecondStart)
	return p
}

func (r *RawBurst) setInfo(p *[25]byte) {
	copy(r[:12], p[:12])
	bits.Copy(r[:], 96, p[:], 96, 2)
	bits.Copy(r[:], infoSecondStart, p[:], infoFirstEnd, infoSecondEnd-infoSecondStart)
}

// BPTCPayload decodes the 196 information bits as BPTC(196,96) and returns
// the 96 data bits packed most significant bit first, before any CRC or
// RS(12,9) check. It is meaningful only for the BPTC data types; a Rate 3/4
// or Rate 1 burst needs Rate34Payload or Rate1Payload.
func (r RawBurst) BPTCPayload() ([12]byte, fec.Result) {
	p, _, res := bptc.Decode196(r.info())
	return p, res
}

// SetBPTCPayload encodes p with BPTC(196,96) into the 196 information bits,
// with the reserved bits R(0) to R(3) zero, and keeps the slot type and
// SYNC. p must already carry its CRC or RS(12,9) parity (see pdu.Seal).
func (r *RawBurst) SetBPTCPayload(p [12]byte) {
	if r == nil {
		return
	}
	info := bptc.Encode196(p, 0)
	r.setInfo(&info)
}

// BPTCReserved decodes the 196 information bits as BPTC(196,96) and returns
// the reserved bits R(0) to R(2) after FEC correction, R(i) in bit i as
// Burst.BPTCReserved holds them, with the BPTC result. Motorola Capacity Max
// sets R(2) to flag RAS (ETSI TS 102 361-1 clause B.1.1).
func (r RawBurst) BPTCReserved() (uint8, fec.Result) {
	_, res, result := bptc.Decode196(r.info())
	return res, result
}

// SetBPTCReserved sets the BPTC(196,96) reserved bits R(0) to R(2) to res,
// R(i) in bit i, and updates the Hamming parity bits that cover them. The
// data bits and any bit errors stay as they are, so BPTCPayload returns the
// same data and FEC result as before. It returns a *dmrgo.FieldRangeError,
// and changes nothing, when res does not fit in 3 bits.
func (r *RawBurst) SetBPTCReserved(res uint8) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	if res>>3 != 0 {
		return errRawBurstBPTCReservedRange
	}
	info := r.info()
	_, cur, _ := bptc.Decode196(info)
	delta := bptc.Encode196([12]byte{}, res^cur)
	for i := range info {
		info[i] ^= delta[i]
	}
	r.setInfo(&info)
	return nil
}

// Rate34Payload decodes the 196 information bits as a Rate 3/4 trellis
// code and returns the 144 data bits packed most significant bit first
// (ETSI TS 102 361-1 clause B.1.1).
func (r RawBurst) Rate34Payload() ([18]byte, fec.Result) {
	return trellis.Decode(r.info())
}

// SetRate34Payload trellis encodes p into the 196 information bits.
func (r *RawBurst) SetRate34Payload(p [18]byte) {
	if r == nil {
		return
	}
	info := trellis.Encode(p)
	r.setInfo(&info)
}

// Rate1Payload returns the 192 uncoded data bits of a Rate 1 burst packed
// most significant bit first: information bits 0 to 95 and 100 to 195. The
// fec.Result is zero (ETSI TS 102 361-1 Table B.10B).
func (r RawBurst) Rate1Payload() ([24]byte, fec.Result) {
	info := r.info()
	var p [24]byte
	copy(p[:12], info[:12])
	bits.Copy(p[:], 96, info[:], 100, 96)
	return p, fec.Result{}
}

// SetRate1Payload writes p into the 192 data bits of a Rate 1 burst.
// Information bits 96 to 99, between the two halves, stay as they are.
func (r *RawBurst) SetRate1Payload(p [24]byte) {
	if r == nil {
		return
	}
	copy(r[:12], p[:12])
	bits.Copy(r[:], infoSecondStart+2, p[:], 96, 96)
}

// EMB decodes the QR(16,7) embedded signalling in burst bits 108 to 115 and
// 148 to 155 of a voice burst B to F or a data burst without a SYNC
// pattern. Its FEC field reports the QR result.
func (r RawBurst) EMB() pdu.EmbeddedSignalling {
	return r.emb(embFirstStart, embSecondStart)
}

// SetEMB encodes e into the EMB bits of a voice burst B to F or a data
// burst without a SYNC pattern. It returns the errors of e.Encode and then
// changes nothing.
func (r *RawBurst) SetEMB(e pdu.EmbeddedSignalling) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	return r.setEMB(e, embFirstStart, embSecondStart)
}

// EmbeddedData returns the 32-bit embedded signalling field, burst bits 116
// to 147, of a voice burst B to F or a data burst without a SYNC pattern:
// an embedded LC fragment, a Null embedded message or an outbound RC, still
// FEC coded.
func (r RawBurst) EmbeddedData() [4]byte {
	return r.word32(embeddedDataStart, embeddedDataStart+16)
}

// SetEmbeddedData writes d into the embedded signalling field.
func (r *RawBurst) SetEmbeddedData(d [4]byte) {
	if r == nil {
		return
	}
	r.setWord32(d, embeddedDataStart, embeddedDataStart+16)
}

// StandaloneRCEMB decodes the EMB of a standalone inbound RC burst, burst
// bits 84 to 91 and 172 to 179 (ETSI TS 102 361-1 clause 6.4.1, Figure 6.7).
func (r RawBurst) StandaloneRCEMB() pdu.EmbeddedSignalling {
	return r.emb(rcembFirstStart, rcembSecondStart)
}

// SetStandaloneRCEMB encodes e into the EMB bits of a standalone RC burst,
// as SetEMB does, with the errors of SetEMB.
func (r *RawBurst) SetStandaloneRCEMB(e pdu.EmbeddedSignalling) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	return r.setEMB(e, rcembFirstStart, rcembSecondStart)
}

// StandaloneRCData returns the 32 FEC coded RC bits of a standalone RC
// burst, burst bits 92 to 107 and 156 to 171, in the order that
// DecodeEmbeddedRC takes.
func (r RawBurst) StandaloneRCData() [4]byte {
	return r.word32(rcDataFirstStart, rcDataSecondStart)
}

// SetStandaloneRCData writes d into the RC bits of a standalone RC burst.
func (r *RawBurst) SetStandaloneRCData(d [4]byte) {
	if r == nil {
		return
	}
	r.setWord32(d, rcDataFirstStart, rcDataSecondStart)
}

func (r RawBurst) emb(first, second int) pdu.EmbeddedSignalling {
	info, res := pdu.DecodeEMBCodeword(uint16(r.field(first, 8, second, 8) & 0xFFFF))
	e := info.EmbeddedSignalling()
	e.FEC = res
	return e
}

func (r *RawBurst) setEMB(e pdu.EmbeddedSignalling, first, second int) error {
	cw, err := e.Encode()
	if err != nil {
		return err
	}
	r.setField(first, 8, second, 8, uint64(cw[0])<<8|uint64(cw[1]))
	return nil
}

func (r RawBurst) word32(first, second int) [4]byte {
	v := r.field(first, 16, second, 16)
	return [4]byte{byte(v >> 24 & 0xFF), byte(v >> 16 & 0xFF), byte(v >> 8 & 0xFF), byte(v & 0xFF)}
}

func (r *RawBurst) setWord32(d [4]byte, first, second int) {
	r.setField(first, 16, second, 16, uint64(d[0])<<24|uint64(d[1])<<16|uint64(d[2])<<8|uint64(d[3]))
}

// VoiceSlots returns the three 72-bit vocoder slots of a voice burst as
// they are on air, before any FEC. Slot i is voice bits 72i to 72i+71, where
// voice bits 0 to 107 are burst bits 0 to 107 and voice bits 108 to 215 are
// burst bits 156 to 263.
func (r RawBurst) VoiceSlots() [VoiceSlotsPerBurst][9]byte {
	var s [VoiceSlotsPerBurst][9]byte
	copy(s[0][:], r[:9])
	copy(s[1][:4], r[9:13])
	bits.Copy(s[1][:], 32, r[:], 104, 4)
	bits.Copy(s[1][:], 36, r[:], syncEnd, 36)
	copy(s[2][:], r[24:])
	return s
}

// SetVoiceSlots writes the three vocoder slots s into the voice bits.
func (r *RawBurst) SetVoiceSlots(s [VoiceSlotsPerBurst][9]byte) {
	if r == nil {
		return
	}
	copy(r[:9], s[0][:])
	copy(r[9:13], s[1][:4])
	bits.Copy(r[:], 104, s[1][:], 32, 4)
	bits.Copy(r[:], syncEnd, s[1][:], 36, 36)
	copy(r[24:], s[2][:])
}

// AMBEFrames decodes the three voice slots as AMBE+2 frames with
// vocoder.DecodeAMBE72, as Burst.AMBEFrames does, and returns them with the
// combined FEC result. The result is meaningful only for a burst that
// carries AMBE+2 voice.
func (r RawBurst) AMBEFrames() ([VoiceSlotsPerBurst]vocoder.AMBEFrame, fec.Result) {
	var frames [VoiceSlotsPerBurst]vocoder.AMBEFrame
	var result fec.Result
	for i, slot := range r.VoiceSlots() {
		var res fec.Result
		frames[i], res = ambe.Decode(slot)
		result = result.Combine(res)
	}
	return frames, result
}

// SetAMBEFrames writes the AMBE+2 coding of frames (vocoder.EncodeAMBE72)
// into the three voice slots. It returns the errors of EncodeAMBE72 and
// then changes nothing.
func (r *RawBurst) SetAMBEFrames(frames [VoiceSlotsPerBurst]vocoder.AMBEFrame) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}
	var s [VoiceSlotsPerBurst][9]byte
	for i := range frames {
		var err error
		if s[i], err = vocoder.EncodeAMBE72(frames[i]); err != nil {
			return err
		}
	}
	r.SetVoiceSlots(s)
	return nil
}

// BuildVoiceBurst returns voice burst pos of a superframe that carries the
// AMBE+2 frames. Burst A carries the voice SYNC pattern of src; bursts B to
// F carry emb, with its LCSS set from pos, and the embedded signalling field
// embedded. It returns ErrVoicePosition, wrapped, when pos is not A to F,
// ErrSyncSource when src is unknown, and a *dmrgo.FieldRangeError when a
// padding bit of a frame is set or a field of emb does not fit its bits.
func BuildVoiceBurst(frames [VoiceSlotsPerBurst]vocoder.AMBEFrame, pos enums.VoiceBurstType, emb pdu.EmbeddedSignalling,
	embedded [4]byte, src SyncSource,
) (RawBurst, error) {
	var r RawBurst
	voice, _, ok := src.SyncPatterns()
	if !ok {
		return r, ErrSyncSource
	}
	if err := r.SetAMBEFrames(frames); err != nil {
		return RawBurst{}, err
	}
	if pos == enums.VoiceBurstA {
		err := r.SetSync(voice)
		return r, err
	}
	lcss, ok := pos.LCSS()
	if !ok {
		return RawBurst{}, errNotAToF
	}
	emb.LCSS = lcss
	if err := r.SetEMB(emb); err != nil {
		return RawBurst{}, err
	}
	r.SetEmbeddedData(embedded)
	return r, nil
}

// BuildBurst returns the data burst that carries d, with a slot type of
// color code cc and data type d.DataType() and the data SYNC pattern of src.
// The BPTC reserved bits are zero. d must be a non-nil pointer to one of the
// Data types of package pdu: FullLinkControl, CSBK, PIHeader, DataHeader,
// MBCHeader, MBCContinuation, CGAPContinuation, MVAPContinuation,
// BCAPContinuation, VNAPContinuation, UnifiedSingleBlockData, Rate12Data,
// Rate34Data, Rate1Data or PRFill. It returns an error for any other d or
// when d does not encode, ErrSyncSource when src is unknown, and a
// *dmrgo.FieldRangeError when cc does not fit in 4 bits. d does not escape
// to the heap.
func BuildBurst(d Data, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	dt, info, err := encodeData(d, 0, true)
	if err != nil {
		return RawBurst{}, err
	}
	r, err := dataBurstFrame(dt, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	var coded [25]byte
	switch dt {
	case enums.DataTypeRate34:
		coded = trellis.Encode([18]byte(info[:18]))
	case enums.DataTypeRate1:
		coded = rate1Info(&info)
	case enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK,
		enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader, enums.DataTypeRate12,
		enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		coded = bptc.Encode196([12]byte(info[:12]), 0)
	}
	r.setInfo(&coded)
	return r, nil
}

// BuildDataBurst returns a data burst that carries the BPTC(196,96) payload
// p, which must already carry its CRC or parity (see pdu.Seal), with a slot
// type of color code cc and data type dt and the data SYNC pattern of src.
// It returns ErrDataType, wrapped, when dt is not BPTC coded, ErrSyncSource
// when src is unknown, and a *dmrgo.FieldRangeError when cc does not fit in
// 4 bits.
func BuildDataBurst(p [12]byte, dt enums.DataType, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	if !dt.IsBPTC() {
		return RawBurst{}, errNotBPTC
	}
	r, err := dataBurstFrame(dt, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetBPTCPayload(p)
	return r, nil
}

// BuildRate34DataBurst returns a Rate 3/4 data burst that carries p, with a
// slot type of color code cc and the data SYNC pattern of src. It returns
// ErrSyncSource when src is unknown, and a *dmrgo.FieldRangeError when cc
// does not fit in 4 bits.
func BuildRate34DataBurst(p [18]byte, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	r, err := dataBurstFrame(enums.DataTypeRate34, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetRate34Payload(p)
	return r, nil
}

// BuildRate1DataBurst returns a Rate 1 data burst that carries p, with a
// slot type of color code cc and the data SYNC pattern of src. Information
// bits 96 to 99 are zero. It returns ErrSyncSource when src is unknown, and
// a *dmrgo.FieldRangeError when cc does not fit in 4 bits.
func BuildRate1DataBurst(p [24]byte, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	r, err := dataBurstFrame(enums.DataTypeRate1, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetRate1Payload(p)
	return r, nil
}

func dataBurstFrame(dt enums.DataType, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	var r RawBurst
	_, sync, ok := src.SyncPatterns()
	if !ok {
		return r, ErrSyncSource
	}
	if err := r.SetSync(sync); err != nil {
		return r, err
	}
	if err := r.SetSlotType(pdu.SlotType{ColorCode: cc, DataType: dt}); err != nil {
		return RawBurst{}, err
	}
	return r, nil
}
