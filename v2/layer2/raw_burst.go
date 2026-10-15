package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/fec/trellis"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// RawBurst is a 33-octet burst in transmit bit order (ETSI TS 102 361-1
// Annex E), the form NewBurstFromBytes takes. Its methods read or write one
// field in place and leave every other bit as it is. They do not classify
// the burst: the caller knows, or decides, which fields the burst carries.
//
// A data burst carries the slot type and one of the BPTCPayload,
// Rate34Payload or Rate1Payload forms around a SYNC pattern; its slot type
// names the form, but a caller that knows the data type from elsewhere (a
// network header, for example) can use it instead. A voice burst carries
// VoiceSlots around a SYNC pattern (burst A) or around EMB and
// EmbeddedData (bursts B to F). A standalone inbound RC burst (§6.4.1)
// carries the RC SYNC with StandaloneRCEMB and StandaloneRCData around it,
// at offsets that differ from EMB and EmbeddedData.
type RawBurst [33]byte

func (r *RawBurst) readBits(dst []bit.Bit, start int) {
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
// and its Hamming distance, as enums.SyncPatternFromBytesWithDistance
// does.
func (r *RawBurst) Sync() (enums.SyncPattern, int) {
	var bits [48]bit.Bit
	r.readBits(bits[:], SyncStart)
	var s [6]byte
	copy(s[:], bit.PackBits(bits[:]))
	return enums.SyncPatternFromBytesWithDistance(s)
}

// SetSync writes the SYNC pattern p into burst bits 108 to 155. It returns
// an error unless p is a Table 9.2 pattern.
func (r *RawBurst) SetSync(p enums.SyncPattern) error {
	if !isSyncPattern(p) {
		return fmt.Errorf("%#x is not a Table 9.2 SYNC pattern", int64(p))
	}
	var bits [264]bit.Bit
	writeSync(&bits, p)
	r.writeBits(bits[SyncStart:SyncEnd], SyncStart)
	return nil
}

// SlotType decodes the Golay(20,8) slot type in burst bits 98 to 107 and
// 156 to 165.
func (r *RawBurst) SlotType() (pdu.SlotType, fec.Result) {
	var bits [20]bit.Bit
	r.readBits(bits[:10], SlotTypeFirstStart)
	r.readBits(bits[10:], SlotTypeSecondStart)
	return pdu.DecodeSlotType(bits)
}

// SetSlotType encodes st into the slot type bits. It returns an error, and
// writes nothing, when the color code or data type does not fit in 4 bits.
func (r *RawBurst) SetSlotType(st pdu.SlotType) error {
	bits, err := pdu.EncodeSlotType(&st)
	if err != nil {
		return err
	}
	r.writeBits(bits[:10], SlotTypeFirstStart)
	r.writeBits(bits[10:], SlotTypeSecondStart)
	return nil
}

func (r *RawBurst) info() [196]bit.Bit {
	var bits [196]bit.Bit
	r.readBits(bits[:98], InfoFirstStart)
	r.readBits(bits[98:], InfoSecondStart)
	return bits
}

func (r *RawBurst) setInfo(bits [196]bit.Bit) {
	r.writeBits(bits[:98], InfoFirstStart)
	r.writeBits(bits[98:], InfoSecondStart)
}

// BPTCPayload decodes the 196 information bits as BPTC(196,96) and returns
// the 96 data bits packed most significant bit first, before any CRC or
// RS(12,9) check (see pdu.BlockCRCOK and pdu.FullLCParityOK). The BPTC code
// is the same for every data type that uses it, so the result does not
// depend on the slot type, but it is meaningful only for those data types:
// a Rate 3/4 or Rate 1 burst needs Rate34Payload or Rate1Payload.
func (r *RawBurst) BPTCPayload() ([12]byte, fec.Result) {
	var d bptc.BPTC19696
	bits, res := d.DeinterleaveDataBits(r.info())
	var p [12]byte
	copy(p[:], bit.PackBits(bits[:]))
	return p, res
}

// SetBPTCPayload BPTC(196,96) encodes p into the 196 information bits. The
// slot type and the SYNC stay as they are. p must already carry its CRC or
// RS(12,9) parity with the Data Type CRC Mask (see pdu.SealBlock and
// pdu.FullLCCodeword).
func (r *RawBurst) SetBPTCPayload(p [12]byte) {
	var bits [96]bit.Bit
	copy(bits[:], bit.UnpackBits(p[:]))
	r.setInfo(bptc.Encode(bits))
}

// Rate34Payload decodes the 196 information bits as a Rate 3/4 trellis
// code (§B.1.1) and returns the 144 data bits packed most significant bit
// first.
func (r *RawBurst) Rate34Payload() ([18]byte, fec.Result) {
	var t trellis.Trellis34
	bits, res := t.Decode(r.info())
	var p [18]byte
	copy(p[:], bit.PackBits(bits[:]))
	return p, res
}

// SetRate34Payload trellis encodes p into the 196 information bits.
func (r *RawBurst) SetRate34Payload(p [18]byte) {
	var bits [144]bit.Bit
	copy(bits[:], bit.UnpackBits(p[:]))
	var t trellis.Trellis34
	r.setInfo(t.Encode(bits))
}

// Rate1Payload returns the 192 uncoded data bits of a Rate 1 burst (Table
// B.10B) packed most significant bit first: information bits 0 to 95 and
// 100 to 195. Rate 1 has no FEC.
func (r *RawBurst) Rate1Payload() [24]byte {
	info := r.info()
	var bits [192]bit.Bit
	copy(bits[:96], info[:96])
	copy(bits[96:], info[100:])
	var p [24]byte
	copy(p[:], bit.PackBits(bits[:]))
	return p
}

// SetRate1Payload writes p into the 192 data bits of a Rate 1 burst.
// Information bits 96 to 99, between the two halves, stay as they are.
func (r *RawBurst) SetRate1Payload(p [24]byte) {
	bits := bit.UnpackBits(p[:])
	r.writeBits(bits[:96], InfoFirstStart)
	r.writeBits(bits[96:], InfoSecondStart+2)
}

// EMB decodes the QR(16,7) embedded signalling in burst bits 108 to 115 and
// 148 to 155 of a voice burst B to F.
func (r *RawBurst) EMB() (pdu.EmbeddedSignalling, fec.Result) {
	return r.emb(EMBFirstStart, EMBSecondStart)
}

// SetEMB encodes e into the EMB bits of a voice burst B to F. It returns an
// error, and writes nothing, when a field does not fit.
func (r *RawBurst) SetEMB(e pdu.EmbeddedSignalling) error {
	return r.setEMB(e, EMBFirstStart, EMBSecondStart)
}

// EmbeddedData returns the 32-bit embedded signalling field, burst bits 116
// to 147, of a voice burst B to F: an embedded LC fragment, a Null
// embedded message or an outbound RC, still FEC coded.
func (r *RawBurst) EmbeddedData() [4]byte {
	return r.word32(EmbeddedDataStart, EmbeddedDataStart+16)
}

// SetEmbeddedData writes d into the embedded signalling field.
func (r *RawBurst) SetEmbeddedData(d [4]byte) {
	r.setWord32(d, EmbeddedDataStart, EmbeddedDataStart+16)
}

// StandaloneRCEMB decodes the EMB of a standalone inbound RC burst (§6.4.1,
// Figure 6.7), burst bits 84 to 91 and 172 to 179.
func (r *RawBurst) StandaloneRCEMB() (pdu.EmbeddedSignalling, fec.Result) {
	return r.emb(RCEMBFirstStart, RCEMBSecondStart)
}

// SetStandaloneRCEMB encodes e into the EMB bits of a standalone RC burst.
func (r *RawBurst) SetStandaloneRCEMB(e pdu.EmbeddedSignalling) error {
	return r.setEMB(e, RCEMBFirstStart, RCEMBSecondStart)
}

// StandaloneRCData returns the 32 FEC coded RC bits of a standalone RC
// burst, burst bits 92 to 107 and 156 to 171, in the order that
// DecodeRCFromEmbeddedData takes.
func (r *RawBurst) StandaloneRCData() [4]byte {
	return r.word32(RCDataFirstStart, RCDataSecondStart)
}

// SetStandaloneRCData writes d into the RC bits of a standalone RC burst.
func (r *RawBurst) SetStandaloneRCData(d [4]byte) {
	r.setWord32(d, RCDataFirstStart, RCDataSecondStart)
}

func (r *RawBurst) emb(first, second int) (pdu.EmbeddedSignalling, fec.Result) {
	var bits [16]bit.Bit
	r.readBits(bits[:8], first)
	r.readBits(bits[8:], second)
	return pdu.DecodeEmbeddedSignalling(bits)
}

func (r *RawBurst) setEMB(e pdu.EmbeddedSignalling, first, second int) error {
	bits, err := pdu.EncodeEmbeddedSignalling(&e)
	if err != nil {
		return err
	}
	r.writeBits(bits[:8], first)
	r.writeBits(bits[8:], second)
	return nil
}

func (r *RawBurst) word32(first, second int) [4]byte {
	var bits [32]bit.Bit
	r.readBits(bits[:16], first)
	r.readBits(bits[16:], second)
	var d [4]byte
	copy(d[:], bit.PackBits(bits[:]))
	return d
}

func (r *RawBurst) setWord32(d [4]byte, first, second int) {
	bits := bit.UnpackBits(d[:])
	r.writeBits(bits[:16], first)
	r.writeBits(bits[16:], second)
}

// VoiceSlots returns the three 72-bit vocoder slots of a voice burst as
// they are on air, before any FEC, as Burst.VoiceSlots does: slot i is
// voice bits 72i to 72i+71, where voice bits 0 to 107 are burst bits 0 to
// 107 and voice bits 108 to 215 are burst bits 156 to 263.
func (r *RawBurst) VoiceSlots() [VoiceSlotsPerBurst][9]byte {
	var voice [216]bit.Bit
	r.readBits(voice[:108], 0)
	r.readBits(voice[108:], SyncEnd)
	var s [VoiceSlotsPerBurst][9]byte
	for i := range s {
		copy(s[i][:], bit.PackBits(voice[i*72:(i+1)*72]))
	}
	return s
}

// SetVoiceSlots writes the three vocoder slots s into the voice bits.
func (r *RawBurst) SetVoiceSlots(s [VoiceSlotsPerBurst][9]byte) {
	var voice [216]bit.Bit
	for i := range s {
		copy(voice[i*72:(i+1)*72], bit.UnpackBits(s[i][:]))
	}
	r.writeBits(voice[:108], 0)
	r.writeBits(voice[108:], SyncEnd)
}

// BuildDataBurst returns a data burst that carries the BPTC(196,96) payload
// p with a slot type of color code cc and data type dt and the data SYNC
// pattern of src. p must already carry its CRC or RS(12,9) parity with the
// Data Type CRC Mask of dt (see pdu.SealBlock and pdu.FullLCCodeword). It
// returns an error when dt is not BPTC coded (Rate 3/4, Rate 1 and the
// reserved data types; see BuildRate34DataBurst and BuildRate1DataBurst),
// cc does not fit in 4 bits or src is unknown.
func BuildDataBurst(p [12]byte, dt elements.DataType, cc uint8, src SyncSource) (RawBurst, error) {
	if !isBPTCDataType(dt) {
		return RawBurst{}, fmt.Errorf("data type %s is not BPTC(196,96) coded", elements.DataTypeToName(dt))
	}
	r, err := dataBurstFrame(dt, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetBPTCPayload(p)
	return r, nil
}

// BuildRate34DataBurst returns a Rate 3/4 data burst that carries p, with
// a slot type of color code cc and the data SYNC pattern of src.
func BuildRate34DataBurst(p [18]byte, cc uint8, src SyncSource) (RawBurst, error) {
	r, err := dataBurstFrame(elements.DataTypeRate34, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetRate34Payload(p)
	return r, nil
}

// BuildRate1DataBurst returns a Rate 1 data burst that carries p, with a
// slot type of color code cc and the data SYNC pattern of src. Information
// bits 96 to 99 are zero.
func BuildRate1DataBurst(p [24]byte, cc uint8, src SyncSource) (RawBurst, error) {
	r, err := dataBurstFrame(elements.DataTypeRate1, cc, src)
	if err != nil {
		return RawBurst{}, err
	}
	r.SetRate1Payload(p)
	return r, nil
}

func dataBurstFrame(dt elements.DataType, cc uint8, src SyncSource) (RawBurst, error) {
	var r RawBurst
	_, sync, ok := src.SyncPatterns()
	if !ok {
		return r, fmt.Errorf("unknown sync source %d", src)
	}
	if cc > 15 {
		return r, fmt.Errorf("color code %d does not fit in 4 bits", cc)
	}
	if err := r.SetSync(sync); err != nil {
		return r, err
	}
	if err := r.SetSlotType(pdu.SlotType{ColorCode: int(cc), DataType: dt}); err != nil {
		return r, err
	}
	return r, nil
}
