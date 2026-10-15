package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// RawBurst is a 33-octet burst in transmit bit order (ETSI TS 102 361-1
// Annex E), the form NewBurstFromBytes takes. Its methods read or write one
// field in place and leave every other bit as it is. They do not classify
// the burst: the caller knows, or decides, which fields the burst carries.
// The getters have value receivers, so they work on a conversion such as
// RawBurst(p).EmbeddedData(); the setters have pointer receivers.
//
// A data burst carries the slot type and one of the BPTCPayload,
// Rate34Payload or Rate1Payload forms around a SYNC pattern or around EMB
// and EmbeddedData (§5.1.5.1, §6.4.2); its slot type
// names the form, but a caller that knows the data type from elsewhere (a
// network header, for example) can use it instead. A voice burst carries
// VoiceSlots around a SYNC pattern (burst A) or around EMB and
// EmbeddedData (bursts B to F). A standalone inbound RC burst (§6.4.1)
// carries the RC SYNC with StandaloneRCEMB and StandaloneRCData around it,
// at offsets that differ from EMB and EmbeddedData.
//
// FEC results follow the rule of package fec. A getter that decodes a PDU,
// such as SlotType and EMB, reports the result in the FEC field of the PDU.
// A getter that decodes a bare value, such as BPTCPayload, Rate34Payload,
// Rate1Payload and AMBEFrames, returns a fec.Result with it. Rate 1 has no
// FEC, so its Result is zero. A getter of uncoded bits, such as
// EmbeddedData and VoiceSlots, returns the bits only.
type RawBurst [33]byte

func (r RawBurst) readBits(dst []bit.Bit, start int) {
	for i := range dst {
		p := start + i
		dst[i] = bit.Bit(r[p/8] >> (7 - p%8) & 1)
	}
}

func (r *RawBurst) writeBits(src []bit.Bit, start int) {
	for i, b := range src {
		p := start + i
		m := byte(1) << (7 - p%8)
		if b&1 != 0 {
			r[p/8] |= m
		} else {
			r[p/8] &^= m
		}
	}
}

// Sync returns the Table 9.2 SYNC pattern nearest to burst bits 108 to 155
// and its Hamming distance, as enums.NearestSyncPattern does.
func (r RawBurst) Sync() (enums.SyncPattern, int) {
	var bits [48]bit.Bit
	r.readBits(bits[:], syncStart)
	var s [6]byte
	copy(s[:], bit.Pack(bits[:]))
	return enums.NearestSyncPattern(s)
}

// SetSync writes the SYNC pattern p into burst bits 108 to 155. It returns
// an error unless p is a Table 9.2 pattern.
func (r *RawBurst) SetSync(p enums.SyncPattern) error {
	if !isSyncPattern(p) {
		return fmt.Errorf("%#x is not a Table 9.2 SYNC pattern", uint64(p))
	}
	var bits [264]bit.Bit
	writeSync(&bits, p)
	r.writeBits(bits[syncStart:syncEnd], syncStart)
	return nil
}

// SlotType decodes the Golay(20,8) slot type in burst bits 98 to 107 and
// 156 to 165. Its FEC field reports the Golay result.
func (r RawBurst) SlotType() pdu.SlotType {
	var bits [20]bit.Bit
	r.readBits(bits[:10], slotTypeFirstStart)
	r.readBits(bits[10:], slotTypeSecondStart)
	return pdu.DecodeSlotType(bits)
}

// SetSlotType encodes st into the slot type bits: the low 4 bits of its
// color code and of its data type, as SlotType.Info returns them.
func (r *RawBurst) SetSlotType(st pdu.SlotType) {
	var bits [20]bit.Bit
	bit.PutUint(bits[:], uint64(st.Info().Codeword()))
	r.writeBits(bits[:10], slotTypeFirstStart)
	r.writeBits(bits[10:], slotTypeSecondStart)
}

func (r RawBurst) info() [196]bit.Bit {
	var bits [196]bit.Bit
	r.readBits(bits[:98], infoFirstStart)
	r.readBits(bits[98:], infoSecondStart)
	return bits
}

func (r *RawBurst) setInfo(bits [196]bit.Bit) {
	r.writeBits(bits[:98], infoFirstStart)
	r.writeBits(bits[98:], infoSecondStart)
}

// BPTCPayload decodes the 196 information bits as BPTC(196,96) and returns
// the 96 data bits packed most significant bit first, before any CRC or
// RS(12,9) check (see pdu.BlockCRCOK and pdu.FullLCParityOK). The BPTC code
// is the same for every data type that uses it, so the result does not
// depend on the slot type, but it is meaningful only for those data types:
// a Rate 3/4 or Rate 1 burst needs Rate34Payload or Rate1Payload.
func (r RawBurst) BPTCPayload() ([12]byte, fec.Result) {
	bits, res := bptc.Decode196(r.info())
	return bit.Pack96(bits), res
}

// SetBPTCPayload BPTC(196,96) encodes p into the 196 information bits. The
// slot type and the SYNC stay as they are. p must already carry its CRC or
// RS(12,9) parity with the Data Type CRC Mask (see pdu.Seal). The reserved
// bits R(0) to R(3) are zero; see SetBPTCReserved.
func (r *RawBurst) SetBPTCPayload(p [12]byte) {
	r.setInfo(bptc.Encode196(bit.Unpack96(p)))
}

// BPTCReserved decodes the 196 information bits as BPTC(196,96) and returns
// the reserved bits R(0) to R(2), R(i) at index i, after FEC correction, with
// the BPTC result. ETSI TS 102 361-1 B.1.1 sets them to zero. Motorola
// Capacity Max sets R(2) to flag RAS (Restricted Access System), which
// replaces the CRC with a keyed check.
func (r RawBurst) BPTCReserved() ([3]bit.Bit, fec.Result) {
	_, res, result := bptc.Decode196WithReserved(r.info())
	return res, result
}

// SetBPTCReserved sets the BPTC(196,96) reserved bits R(0) to R(2) to res,
// R(i) at index i, and updates the Hamming parity bits that cover them. The
// data bits and any bit errors stay as they are, so BPTCPayload returns the
// same data and FEC result as before.
func (r *RawBurst) SetBPTCReserved(res [3]bit.Bit) {
	info := r.info()
	_, cur, _ := bptc.Decode196WithReserved(info)
	for i := range res {
		res[i] = (res[i] ^ cur[i]) & 1
	}
	delta := bptc.Encode196WithReserved([96]bit.Bit{}, res)
	for i := range info {
		info[i] ^= delta[i]
	}
	r.setInfo(info)
}

// Rate34Payload decodes the 196 information bits as a Rate 3/4 trellis
// code (§B.1.1) and returns the 144 data bits packed most significant bit
// first.
func (r RawBurst) Rate34Payload() ([18]byte, fec.Result) {
	bits, res := trellis.Decode(r.info())
	var p [18]byte
	copy(p[:], bit.Pack(bits[:]))
	return p, res
}

// SetRate34Payload trellis encodes p into the 196 information bits.
func (r *RawBurst) SetRate34Payload(p [18]byte) {
	var bits [144]bit.Bit
	copy(bits[:], bit.Unpack(p[:]))
	r.setInfo(trellis.Encode(bits))
}

// Rate1Payload returns the 192 uncoded data bits of a Rate 1 burst (Table
// B.10B) packed most significant bit first: information bits 0 to 95 and
// 100 to 195. Rate 1 has no FEC, so the fec.Result is zero.
func (r RawBurst) Rate1Payload() ([24]byte, fec.Result) {
	info := r.info()
	var bits [192]bit.Bit
	copy(bits[:96], info[:96])
	copy(bits[96:], info[100:])
	var p [24]byte
	copy(p[:], bit.Pack(bits[:]))
	return p, fec.Result{}
}

// SetRate1Payload writes p into the 192 data bits of a Rate 1 burst.
// Information bits 96 to 99, between the two halves, stay as they are.
func (r *RawBurst) SetRate1Payload(p [24]byte) {
	bits := bit.Unpack(p[:])
	r.writeBits(bits[:96], infoFirstStart)
	r.writeBits(bits[96:], infoSecondStart+2)
}

// EMB decodes the QR(16,7) embedded signalling in burst bits 108 to 115 and
// 148 to 155 of a voice burst B to F or a data burst without a SYNC
// pattern. Its FEC field reports the QR result.
func (r RawBurst) EMB() pdu.EmbeddedSignalling {
	return r.emb(embFirstStart, embSecondStart)
}

// SetEMB encodes e into the EMB bits of a voice burst B to F or a data
// burst without a SYNC pattern: the low 4 bits of the color code, the PI
// and the low 2 bits of the LCSS, as EmbeddedSignalling.Info returns them.
func (r *RawBurst) SetEMB(e pdu.EmbeddedSignalling) {
	r.setEMB(e, embFirstStart, embSecondStart)
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
	r.setWord32(d, embeddedDataStart, embeddedDataStart+16)
}

// StandaloneRCEMB decodes the EMB of a standalone inbound RC burst (§6.4.1,
// Figure 6.7), burst bits 84 to 91 and 172 to 179.
func (r RawBurst) StandaloneRCEMB() pdu.EmbeddedSignalling {
	return r.emb(rcembFirstStart, rcembSecondStart)
}

// SetStandaloneRCEMB encodes e into the EMB bits of a standalone RC burst,
// as SetEMB does.
func (r *RawBurst) SetStandaloneRCEMB(e pdu.EmbeddedSignalling) {
	r.setEMB(e, rcembFirstStart, rcembSecondStart)
}

// StandaloneRCData returns the 32 FEC coded RC bits of a standalone RC
// burst, burst bits 92 to 107 and 156 to 171, in the order that
// DecodeEmbeddedRC takes.
func (r RawBurst) StandaloneRCData() [4]byte {
	return r.word32(rcDataFirstStart, rcDataSecondStart)
}

// SetStandaloneRCData writes d into the RC bits of a standalone RC burst.
func (r *RawBurst) SetStandaloneRCData(d [4]byte) {
	r.setWord32(d, rcDataFirstStart, rcDataSecondStart)
}

func (r RawBurst) emb(first, second int) pdu.EmbeddedSignalling {
	var bits [16]bit.Bit
	r.readBits(bits[:8], first)
	r.readBits(bits[8:], second)
	return pdu.DecodeEmbeddedSignalling(bits)
}

func (r *RawBurst) setEMB(e pdu.EmbeddedSignalling, first, second int) {
	var bits [16]bit.Bit
	bit.PutUint(bits[:], uint64(e.Info().Codeword()))
	r.writeBits(bits[:8], first)
	r.writeBits(bits[8:], second)
}

func (r RawBurst) word32(first, second int) [4]byte {
	var bits [32]bit.Bit
	r.readBits(bits[:16], first)
	r.readBits(bits[16:], second)
	return bit.Pack32(bits)
}

func (r *RawBurst) setWord32(d [4]byte, first, second int) {
	bits := bit.Unpack32(d)
	r.writeBits(bits[:16], first)
	r.writeBits(bits[16:], second)
}

// VoiceSlots returns the three 72-bit vocoder slots of a voice burst as
// they are on air, before any FEC, as Burst.VoiceSlots does: slot i is
// voice bits 72i to 72i+71, where voice bits 0 to 107 are burst bits 0 to
// 107 and voice bits 108 to 215 are burst bits 156 to 263.
func (r RawBurst) VoiceSlots() [VoiceSlotsPerBurst][9]byte {
	var voice [216]bit.Bit
	r.readBits(voice[:108], 0)
	r.readBits(voice[108:], syncEnd)
	var s [VoiceSlotsPerBurst][9]byte
	for i := range s {
		copy(s[i][:], bit.Pack(voice[i*72:(i+1)*72]))
	}
	return s
}

// SetVoiceSlots writes the three vocoder slots s into the voice bits.
func (r *RawBurst) SetVoiceSlots(s [VoiceSlotsPerBurst][9]byte) {
	var voice [216]bit.Bit
	for i := range s {
		copy(voice[i*72:(i+1)*72], bit.Unpack(s[i][:]))
	}
	r.writeBits(voice[:108], 0)
	r.writeBits(voice[108:], syncEnd)
}

// BuildBurst returns the data burst that carries d, with a slot type of
// color code cc and data type d.DataType() and the data SYNC pattern of
// src. It codes d.EncodeBits(), which carries the CRC or RS(12,9) parity of
// d, with BPTC(196,96), the Rate 3/4 trellis or Rate 1 as the data type
// selects. The BPTC reserved bits are zero. It returns an error when d is
// nil or does not encode, or src is unknown.
func BuildBurst(d Data, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	if d == nil {
		return RawBurst{}, fmt.Errorf("cannot build a burst from nil Data")
	}
	dt := d.DataType()
	info, err := encodeData(d, dt)
	if err != nil {
		return RawBurst{}, err
	}
	r, err := dataBurstFrame(dt, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	switch dt {
	case enums.DataTypeRate34:
		var bits [144]bit.Bit
		copy(bits[:], info[:])
		r.setInfo(trellis.Encode(bits))
	case enums.DataTypeRate1:
		var bits [196]bit.Bit
		copy(bits[:96], info[:96])
		copy(bits[100:], info[96:192])
		r.setInfo(bits)
	case enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC, enums.DataTypeCSBK,
		enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader, enums.DataTypeRate12,
		enums.DataTypeIdle, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		r.setInfo(bptc.Encode196([96]bit.Bit(info[:96])))
	}
	return r, nil
}

// BuildDataBurst returns a data burst that carries the BPTC(196,96) payload
// p with a slot type of color code cc and data type dt and the data SYNC
// pattern of src. p must already carry its CRC or RS(12,9) parity with the
// Data Type CRC Mask of dt (see pdu.Seal). It returns an error when dt is
// not BPTC coded (Rate 3/4, Rate 1 and the reserved data types; see
// BuildRate34DataBurst and BuildRate1DataBurst) or src is unknown.
func BuildDataBurst(p [12]byte, dt enums.DataType, cc enums.ColorCode, src SyncSource) (RawBurst, error) {
	if !dt.IsBPTC() {
		return RawBurst{}, fmt.Errorf("data type %s is not BPTC(196,96) coded", dt.String())
	}
	r, err := dataBurstFrame(dt, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetBPTCPayload(p)
	return r, nil
}

// BuildRate34DataBurst returns a Rate 3/4 data burst that carries p, with
// a slot type of color code cc and the data SYNC pattern of src. It returns
// an error when src is unknown.
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
// bits 96 to 99 are zero. It returns an error when src is unknown.
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
		return r, fmt.Errorf("unknown sync source %d", src)
	}
	if err := r.SetSync(sync); err != nil {
		return r, err
	}
	r.SetSlotType(pdu.SlotType{ColorCode: cc, DataType: dt})
	return r, nil
}
