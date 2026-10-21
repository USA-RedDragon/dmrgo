package ipsc

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// RTP is the RTP header of a user packet, octets 18 to 29.
type RTP struct {
	// Flags is octet 18: version 2, no padding, extension or CSRC (0x80).
	Flags uint8
	// Marker is the marker bit, set on the first voice header.
	Marker bool
	// PayloadType is 0x5D, or 0x5E on the last packet of a call.
	PayloadType uint8
	// Seq is the RTP sequence number, one more for each burst, lost ones
	// included.
	Seq uint16
	// Timestamp is the RTP timestamp, 480 more for each burst.
	Timestamp uint32
	// SSRC is 0 on MOTOTRBO.
	SSRC uint32
}

// Voice is the burst of a voice packet.
type Voice struct {
	// Layout is the form of the packet: A, BCDF or E.
	Layout VoiceLayout
	// Length is octet 31, the number of octets after it.
	Length uint8
	// Control is octet 32: 0x40 on burst A, 0x10 on burst E, and 0x01 when
	// AMBE frame 1 was bad on receive.
	Control uint8
	// AMBE holds the three AMBE+2 frames without FEC (see
	// vocoder.UnpackAMBEVoiceFlags).
	AMBE [19]byte
	// Embedded is the 32-bit embedded signalling field of bursts B to F.
	Embedded [4]byte
	// LC is the full LC of the call that burst E carries.
	LC [9]byte
	// EMB is the EMB of bursts B to F without its FEC, as the repeater sent
	// it.
	EMB pdu.EMBInfo
}

// Payload is the burst of a data packet: the decoded payload with its
// status octets.
type Payload struct {
	// Status is octet 31: bit 7 is slot 2, the rest are status bits.
	Status uint8
	// Words is octets 32 and 33, the number of 16-bit words after them. An
	// XPR 8400 sends some BurstRaw packets with a Words 1 more than the
	// packet holds, so it is kept as sent.
	Words uint16
	// HasBody reports whether octets 34 onward are present; a BurstSyncLost
	// packet has none.
	HasBody bool
	// Flags is octets 34 and 35: 0x80 in octet 34 means an RSSI follows,
	// 0x08 in octet 35 a slot type, and 0x40 in octet 35 six octets of
	// unknown meaning.
	Flags [2]byte
	// Bits is the payload size in bits: 96 for a BPTC burst, 144 for rate
	// 3/4, 196 for BurstRaw.
	Bits uint16
	// Octets holds the payload, the first (Bits+7)/8 octets.
	Octets [26]byte
	// Trailer holds the octets after the payload, TrailerLen of them: the
	// slot type, the unknown octets and the RSSI.
	Trailer [16]byte
	// TrailerLen is the number of octets in Trailer.
	TrailerLen uint8
}

// SlotType returns the slot type octet of the trailer and whether the
// packet has one.
func (d *Payload) SlotType() (pdu.SlotTypeInfo, bool) {
	if d.Flags[1]&0x08 == 0 || d.TrailerLen < 2 {
		return 0, false
	}
	return pdu.SlotTypeInfo(d.Trailer[1]), true
}

// RSSI returns the last two trailer octets and whether the packet has them.
func (d *Payload) RSSI() (uint16, bool) {
	if d.Flags[0]&0x80 == 0 || d.TrailerLen < 2 {
		return 0, false
	}
	return uint16(d.Trailer[d.TrailerLen-2])<<8 | uint16(d.Trailer[d.TrailerLen-1]), true
}

// User is a voice or data packet, opcodes 0x80, 0x81, 0x83 and 0x84.
type User struct {
	// Opcode is the packet.
	Opcode Opcode
	// ID is the peer ID of the sender.
	ID uint32
	// CallSeq is octet 5, the same in every packet of a call and one more
	// for each call on an XPR 8400.
	CallSeq uint8
	// Source is the radio ID of the caller (24 bits).
	Source uint32
	// Destination is the talkgroup or radio ID (24 bits).
	Destination uint32
	// Priority is octet 12: 0x02 on voice and 0x01 on data on MOTOTRBO.
	Priority uint8
	// CallControl identifies the call.
	CallControl uint32
	// Info is octet 17: privacy, end and slot.
	Info CallInfo
	// RTP is the RTP header.
	RTP RTP
	// Burst is octet 30.
	Burst BurstType
	// Voice is the burst of a voice packet.
	Voice Voice
	// Data is the burst of a data packet.
	Data Payload
}

// IsVoice reports whether u carries a voice burst.
func (u User) IsVoice() bool { return u.Burst&0x7F == BurstVoice }

// DecodeUser decodes the user packet p, without the authentication trailer
// (see package auth). It returns ErrCommand for another opcode, ErrShort,
// and ErrLength when the length does not match a voice layout or a data
// packet is too short for its payload size. It does not allocate.
func DecodeUser(p []byte) (User, error) {
	if len(p) < 31 {
		return User{}, ErrShort
	}
	switch Opcode(p[0]) {
	case OpGroupVoice, OpPrivateVoice, OpGroupData, OpPrivateData:
	default:
		return User{}, ErrCommand
	}
	h, err := decodeUserHeader(p[:31])
	if err != nil {
		return User{}, err
	}
	u := User{Opcode: h.Opcode, ID: h.ID, CallSeq: h.CallSeq, Source: h.Source, Destination: h.Destination, Priority: h.Priority,
		CallControl: h.CallControl, Info: h.Info, Burst: BurstType(h.Burst),
		RTP: RTP{Flags: h.RTPFlags, Marker: h.Marker, PayloadType: h.PayloadType, Seq: h.Seq, Timestamp: h.Timestamp, SSRC: h.SSRC}}
	if u.IsVoice() {
		u.Voice, err = decodeVoice(p[31:])
	} else {
		u.Data, err = decodeData(p)
	}
	if err != nil {
		return User{}, err
	}
	return u, nil
}

func decodeVoice(b []byte) (Voice, error) {
	var v Voice
	switch len(b) {
	case 21:
		m, err := decodeVoiceA(b)
		if err != nil {
			return v, err
		}
		v = Voice{Layout: VoiceLayoutA, Length: m.Length, Control: m.Control, AMBE: m.AMBE}
	case 26:
		m, err := decodeVoiceBCDF(b)
		if err != nil {
			return v, err
		}
		v = Voice{Layout: VoiceLayoutBCDF, Length: m.Length, Control: m.Control, AMBE: m.AMBE, Embedded: m.Embedded, EMB: pdu.EMBInfo(m.EMB)}
	case 35:
		m, err := decodeVoiceE(b)
		if err != nil {
			return v, err
		}
		v = Voice{Layout: VoiceLayoutE, Length: m.Length, Control: m.Control, AMBE: m.AMBE, Embedded: m.Embedded, LC: m.LC, EMB: pdu.EMBInfo(m.EMB)}
	default:
		if len(b) < 21 {
			return v, ErrShort
		}
		return v, ErrLength
	}
	return v, nil
}

func decodeData(p []byte) (Payload, error) {
	var d Payload
	if len(p) < 34 {
		return d, ErrShort
	}
	h, err := decodeDataHead(p[31:34])
	if err != nil {
		return d, err
	}
	d.Status, d.Words = h.Status, h.Words
	if len(p) == 34 {
		return d, nil
	}
	if len(p) < 38 {
		return d, ErrLength
	}
	d.HasBody = true
	d.Flags = [2]byte{p[34], p[35]}
	d.Bits = uint16(p[36])<<8 | uint16(p[37])
	n := (int(d.Bits) + 7) / 8
	if n > len(d.Octets) || 38+n > len(p) || len(p)-38-n > len(d.Trailer) {
		return d, ErrLength
	}
	copy(d.Octets[:], p[38:38+n])
	d.TrailerLen = uint8(copy(d.Trailer[:], p[38+n:]) & 0xFF)
	return d, nil
}

// AppendBinary appends the packet u to dst, without the authentication
// trailer. It returns dst unchanged and an error for an opcode that is not
// a user packet, an unknown voice layout, a data payload that does not fit,
// and a *dmrgo.FieldRangeError for a Source or
// Destination over 24 bits or a PayloadType over 7. It allocates only to
// grow dst.
func (u User) AppendBinary(dst []byte) ([]byte, error) {
	switch u.Opcode {
	case OpGroupVoice, OpPrivateVoice, OpGroupData, OpPrivateData:
	default:
		return dst, errOpcode
	}
	n := len(dst)
	h := userHeader{Opcode: u.Opcode, ID: u.ID, CallSeq: u.CallSeq, Source: u.Source, Destination: u.Destination, Priority: u.Priority,
		CallControl: u.CallControl, Info: u.Info, RTPFlags: u.RTP.Flags, Marker: u.RTP.Marker, PayloadType: u.RTP.PayloadType,
		Seq: u.RTP.Seq, Timestamp: u.RTP.Timestamp, SSRC: u.RTP.SSRC, Burst: uint8(u.Burst)}
	dst, err := h.AppendBinary(dst)
	if err != nil {
		return dst[:n], err
	}
	if u.IsVoice() {
		dst, err = appendVoice(dst, u.Voice)
	} else {
		dst, err = appendPayload(dst, u.Data)
	}
	if err != nil {
		return dst[:n], err
	}
	return dst, nil
}

func appendVoice(dst []byte, v Voice) ([]byte, error) {
	switch v.Layout {
	case VoiceLayoutA:
		m := voiceA{Length: v.Length, Control: v.Control, AMBE: v.AMBE}
		return m.AppendBinary(dst)
	case VoiceLayoutBCDF:
		m := voiceBCDF{Length: v.Length, Control: v.Control, AMBE: v.AMBE, Embedded: v.Embedded, EMB: uint8(v.EMB)}
		return m.AppendBinary(dst)
	case VoiceLayoutE:
		m := voiceE{Length: v.Length, Control: v.Control, AMBE: v.AMBE, Embedded: v.Embedded, LC: v.LC, EMB: uint8(v.EMB)}
		return m.AppendBinary(dst)
	}
	return dst, errLayout
}

func appendPayload(dst []byte, d Payload) ([]byte, error) {
	dst = append(dst, d.Status, byte(d.Words>>8), byte(d.Words&0xFF))
	if !d.HasBody {
		return dst, nil
	}
	n := (int(d.Bits) + 7) / 8
	if n > len(d.Octets) || int(d.TrailerLen) > len(d.Trailer) {
		return dst, errPayload
	}
	dst = append(dst, d.Flags[0], d.Flags[1], byte(d.Bits>>8), byte(d.Bits&0xFF))
	dst = append(dst, d.Octets[:n]...)
	return append(dst, d.Trailer[:d.TrailerLen]...), nil
}

// Hint returns what the packet tells about its burst, for the service
// machines: the slot from the call info, the RTP sequence number (16 bits),
// the call control as the stream, burst A or E from the voice layout (the
// receiver places B, C, D and F by the sequence number), the data type of a
// data packet, EndWeak for the end flag on a voice packet, EndStrong on a
// terminator or a data packet, and SyncLost for BurstSyncLost.
func (u *User) Hint() layer2.Hint {
	h := layer2.Hint{Slot: u.Info.Slot(), Seq: u.RTP.Seq, HasSeq: true, Stream: u.CallControl, HasStream: true}
	switch {
	case u.IsVoice():
		h.Kind = layer2.KindVoice
		switch u.Voice.Layout {
		case VoiceLayoutA:
			h.VoiceBurst = enums.VoiceBurstA
		case VoiceLayoutE:
			h.VoiceBurst = enums.VoiceBurstE
		case VoiceLayoutBCDF:
		}
		if u.Info.End() {
			h.End = layer2.EndWeak
		}
	case u.Burst&0x7F == BurstSyncLost:
		h.SyncLost = true
	case u.Burst < 0x10:
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, enums.DataType(u.Burst), true
		if u.Burst == BurstType(enums.DataTypeTerminatorWithLC) || u.Info.End() {
			h.End = layer2.EndStrong
		}
	case u.Burst == BurstRaw:
		h.Kind = layer2.KindData
		if st, ok := u.Data.SlotType(); ok {
			h.DataType, h.HasDataType = st.DataType(), true
		}
	}
	return h
}

// RawBurst rebuilds the 264-bit burst of the packet. A voice burst gets
// the FEC of its AMBE frames, the voice SYNC of src at burst A, and the EMB
// and embedded data as the repeater sent them. A data burst gets the BPTC
// or rate 3/4 coding of its payload, the data SYNC of src and the slot type
// of the packet, or of cc and the burst type when the packet has none. It
// returns an error for BurstSyncLost, BurstRaw and other packets that carry
// no burst it can rebuild, and the errors of the layer2 builders.
func (u *User) RawBurst(cc enums.ColorCode, src layer2.SyncSource) (layer2.RawBurst, error) {
	if u.IsVoice() {
		var r layer2.RawBurst
		if err := r.SetAMBEFrames(vocoder.UnpackAMBEVoice(u.Voice.AMBE)); err != nil {
			return layer2.RawBurst{}, err
		}
		if u.Voice.Layout == VoiceLayoutA {
			voice, _, ok := src.SyncPatterns()
			if !ok {
				return layer2.RawBurst{}, errBurst
			}
			err := r.SetSync(voice)
			return r, err
		}
		if err := r.SetEMB(u.Voice.EMB.EmbeddedSignalling()); err != nil {
			return layer2.RawBurst{}, err
		}
		r.SetEmbeddedData(u.Voice.Embedded)
		return r, nil
	}
	d := &u.Data
	if st, ok := d.SlotType(); ok {
		cc = st.ColorCode()
	}
	switch {
	case u.Burst == BurstType(enums.DataTypeRate34) && d.Bits == 144:
		var block [18]byte
		copy(block[:2], d.Octets[16:18])
		copy(block[2:], d.Octets[:16])
		return layer2.BuildRate34DataBurst(block, cc, src)
	case u.Burst < BurstType(enums.DataTypeRate34) && d.Bits == 96:
		return layer2.BuildDataBurst([12]byte(d.Octets[:12]), enums.DataType(u.Burst), cc, src)
	}
	return layer2.RawBurst{}, errBurst
}

// SetRawBurst sets Burst and Voice or Data from the 264-bit burst raw and
// its layer2 view, as an XPR 8400 sends them: KindVoice with the position
// pos, which picks the layout (the full LC of burst E stays as it is in
// u.Voice.LC), or KindData with the data type dt. It takes the slot from
// u.Info and leaves the header and RTP fields as they are. It returns an
// error for another kind, a voice position that is not A to F, a data type
// that IPSC does not carry (Idle, rate 1, unified single block and the
// reserved types), and the errors of vocoder.PackAMBEVoice.
func (u *User) SetRawBurst(raw layer2.RawBurst, k layer2.Kind, pos enums.VoiceBurstType, dt enums.DataType) error {
	var slotBit uint8
	if u.Info.Slot() == enums.Slot2 {
		slotBit = 0x80
	}
	switch k {
	case layer2.KindVoice:
		v, err := voiceOf(raw, pos, u.Voice.LC)
		if err != nil {
			return err
		}
		u.Burst, u.Voice = BurstVoice|BurstType(slotBit), v
		return nil
	case layer2.KindData:
		d, err := dataOf(raw, dt, slotBit)
		if err != nil {
			return err
		}
		u.Burst, u.Data = BurstType(dt), d
		return nil
	case layer2.KindUnknown, layer2.KindReverseChannel:
	}
	return errKind
}

func voiceOf(raw layer2.RawBurst, pos enums.VoiceBurstType, lc [9]byte) (Voice, error) {
	if _, ok := pos.Index(); !ok {
		return Voice{}, errKind
	}
	frames, _ := raw.AMBEFrames()
	ambe, err := vocoder.PackAMBEVoice(frames)
	if err != nil {
		return Voice{}, err
	}
	v := Voice{Layout: VoiceLayoutBCDF, Length: 0x19, Control: 0x06, AMBE: ambe, Embedded: raw.EmbeddedData(), LC: lc}
	switch pos {
	case enums.VoiceBurstA:
		return Voice{Layout: VoiceLayoutA, Length: 0x14, Control: 0x40, AMBE: ambe, LC: lc}, nil
	case enums.VoiceBurstE:
		v.Layout, v.Length, v.Control = VoiceLayoutE, 0x22, 0x16
	default:
	}
	emb := raw.EMB()
	if v.EMB, err = emb.Info(); err != nil {
		return Voice{}, err
	}
	return v, nil
}

func dataOf(raw layer2.RawBurst, dt enums.DataType, slotBit uint8) (Payload, error) {
	d := Payload{Status: 0x40 | slotBit, HasBody: true, Flags: [2]byte{0x80, 0x0A}, TrailerLen: 4, Words: 10}
	switch {
	case dt == enums.DataTypeRate34:
		p, _ := raw.Rate34Payload()
		copy(d.Octets[:16], p[2:])
		copy(d.Octets[16:18], p[:2])
		d.Bits, d.Words = 144, 13
	case dt < enums.DataTypeRate34:
		p, _ := raw.BPTCPayload()
		copy(d.Octets[:12], p[:])
		d.Bits = 96
	default:
		return Payload{}, errDataType
	}
	st := raw.SlotType()
	st.DataType = dt
	info, err := st.Info()
	if err != nil {
		return Payload{}, err
	}
	d.Trailer[1] = byte(info)
	return d, nil
}
