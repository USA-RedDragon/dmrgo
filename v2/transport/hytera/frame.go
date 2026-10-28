package hytera

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// FrameLen is the length of a DMR frame in octets.
const FrameLen = 72

// Frame is a packet of the DMR service: one burst with its slot, slot type
// and call fields. The fields whose meaning no source proves are kept as
// received, so a decoded frame encodes to the same octets.
type Frame struct {
	// Port is octets 0 and 1: 50002 (C3 52) on the bursts of the repeater
	// in the DO1KBL capture, 0 on its sync packets, 5A 5A from others.
	Port uint16
	// Mark is octets 2 and 3: 00 50 or 00 00 in the DO1KBL capture, 5A 5A
	// from others.
	Mark uint16
	// Seq is octet 4, one more for each packet; it wraps after 255.
	Seq uint8
	// PacketType is octet 8: 0x01 on bursts and 0x03 on terminators in the
	// DO1KBL capture, 0x41 and 0x43 from others.
	PacketType uint8
	// Slot is the TDMA channel, Slot1 (1111) or Slot2 (2222).
	Slot enums.Slot
	// SlotType is what the burst is.
	SlotType SlotType
	// ColorCode is octets 20 and 21, one nibble four times.
	ColorCode enums.ColorCode
	// FrameType is octets 22 and 23: 0000 on bursts, 1111 or 6666 on sync
	// packets.
	FrameType uint16
	// Burst is the 33-octet burst in transmit bit order, with the octet
	// pairs of the packet swapped back. A sync or wakeup packet carries
	// no burst here.
	Burst layer2.RawBurst
	// Pad is octet 58, the octet after the burst in the payload.
	Pad uint8
	// CallType is octet 62: 0x01 for a group call and 0x00 for a private
	// call from a Hytera repeater, 0x0A on wakeup packets. BrandMeister
	// sends 0x00 on group calls too. Call and SetCall read and write it.
	CallType uint8
	// Destination is the talkgroup or radio ID that is called (octets 64
	// to 66, little-endian).
	Destination uint32
	// Source is the radio ID of the caller (octets 68 to 70,
	// little-endian).
	Source uint32
	// Reserved is octets 5 to 7, 9 to 15, 24 and 25, 60 and 61, 63, 67 and
	// 71, in that order.
	Reserved [17]byte
}

// DecodeFrame decodes the 72-octet DMR frame p. It returns ErrShort,
// ErrLength, or ErrCode for a slot or color code field that is not one of
// its values.
func DecodeFrame(p []byte) (Frame, error) {
	l, err := decodeFrameLayout(p)
	if err != nil {
		return Frame{}, err
	}
	var f Frame
	switch l.Slot {
	case 0x1111:
		f.Slot = enums.Slot1
	case 0x2222:
		f.Slot = enums.Slot2
	default:
		return Frame{}, ErrCode
	}
	cc := l.ColorCode & 0xF
	if l.ColorCode != cc*0x1111 {
		return Frame{}, ErrCode
	}
	f.ColorCode = enums.ColorCode(cc)
	f.Port, f.Mark, f.Seq, f.PacketType = l.Port, l.Mark, l.Seq, l.PacketType
	f.SlotType, f.FrameType, f.CallType = SlotType(l.SlotType), l.FrameType, l.CallType
	copy(f.Burst[:], l.Payload[:33])
	f.Pad = l.Payload[33]
	f.Destination, f.Source = l.Destination, l.Source
	copy(f.Reserved[0:3], l.R5[:])
	copy(f.Reserved[3:10], l.R9[:])
	copy(f.Reserved[10:12], l.R24[:])
	copy(f.Reserved[12:14], l.R60[:])
	f.Reserved[14], f.Reserved[15], f.Reserved[16] = l.R63, l.R67, l.R71
	return f, nil
}

// AppendBinary appends the 72-octet frame to dst. It returns dst unchanged
// and an error that wraps ErrCode when Slot is not Slot1 or Slot2, and a
// *dmrgo.FieldRangeError when ColorCode is above 15 or Destination or Source
// does not fit in 24 bits. A nil f gives
// dmrgo.ErrNilReceiver. It allocates only when dst has no room for FrameLen
// more octets.
func (f *Frame) AppendBinary(dst []byte) ([]byte, error) {
	if f == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	var l frameLayout
	switch f.Slot {
	case enums.Slot1:
		l.Slot = 0x1111
	case enums.Slot2:
		l.Slot = 0x2222
	default:
		return dst, errSlot
	}
	if f.ColorCode > 15 {
		return dst, errFrameColorCodeRange
	}
	l.ColorCode = uint16(f.ColorCode) * 0x1111
	l.Port, l.Mark, l.Seq, l.PacketType = f.Port, f.Mark, f.Seq, f.PacketType
	l.SlotType, l.FrameType, l.CallType = uint16(f.SlotType), f.FrameType, f.CallType
	copy(l.Payload[:33], f.Burst[:])
	l.Payload[33] = f.Pad
	l.Destination, l.Source = f.Destination, f.Source
	copy(l.R5[:], f.Reserved[0:3])
	copy(l.R9[:], f.Reserved[3:10])
	copy(l.R24[:], f.Reserved[10:12])
	copy(l.R60[:], f.Reserved[12:14])
	l.R63, l.R67, l.R71 = f.Reserved[14], f.Reserved[15], f.Reserved[16]
	return l.AppendBinary(dst)
}

// Call returns the call of the frame: target is Destination and source is
// Source. The FLCO of the burst sets group when it is a voice LC header
// or terminator with a correctable LC, so that the group calls that
// BrandMeister sends with CallType 0x00 read as group. Otherwise CallType
// sets it: 0x01 is a group call, as Hytera repeaters send it, and any other
// value is not. A nil f gives zero values.
//
// (OK-DMR Hytera_Homebrew_Bridge captures ea5gvk_1.pcapng and
// do1kbl_BM_Connect.pcapng; OK-DMR dmr-kaitai ip_site_connect_protocol.ksy)
func (f *Frame) Call() (group bool, target, source uint32) {
	if f == nil {
		return false, 0, 0
	}
	group = f.CallType == 0x01
	if f.SlotType == SlotTypeVoiceLCHeader || f.SlotType == SlotTypeTerminator {
		if p, res := f.Burst.BPTCPayload(); !res.Uncorrectable {
			lcb := pdu.LCBytes(p[:9])
			if g, _, _, ok := lcb.Call(); ok {
				group = g
			}
		}
	}
	return group, f.Destination, f.Source
}

// SetCall sets CallType to 0x01 for a group call and 0x00 for a private
// call, as a Hytera repeater sends them, and Destination and Source. It
// returns a *dmrgo.FieldRangeError when target or source does not fit in 24
// bits, and dmrgo.ErrNilReceiver for a nil f, and then changes nothing.
func (f *Frame) SetCall(group bool, target, source uint32) error {
	switch {
	case f == nil:
		return dmrgo.ErrNilReceiver
	case target>>24 != 0:
		return errFrameDestinationRange
	case source>>24 != 0:
		return errFrameSourceRange
	}
	f.CallType = 0x00
	if group {
		f.CallType = 0x01
	}
	f.Destination, f.Source = target, source
	return nil
}

// HasBurst reports whether Burst holds a burst: false for SlotTypeSync and
// SlotTypeWakeup, whose payload is not an ETSI burst, and for a nil f.
func (f *Frame) HasBurst() bool {
	return f != nil && f.SlotType != SlotTypeSync && f.SlotType != SlotTypeWakeup
}

// Hint returns the data that the frame gives about its burst, for the service
// machines: the slot, the voice position or data type from SlotType, and
// the sequence number (8 bits), and now, the time at which the frame
// arrived, as At. Give now 0 when the time is not known. A data slot type
// gives the data type of the slot type in the burst instead when the burst
// has a data SYNC pattern and its slot type decodes, because that field has
// FEC and Hytera_Homebrew_Bridge sends CSBK and data bursts with
// SlotTypeTerminator. A slot type without a burst kind, such as one that no
// source names, gives KindUnknown; use HasBurst first. A nil f gives the
// zero Hint.
func (f *Frame) Hint(now time.Duration) layer2.Hint {
	if f == nil {
		return layer2.Hint{}
	}
	h := layer2.Hint{Slot: f.Slot, Seq: uint16(f.Seq), HasSeq: true, SeqBits: 8, At: now}
	if f.SlotType >= SlotTypeVoiceA && f.SlotType <= SlotTypeVoiceF && f.SlotType%0x1111 == 0 {
		h.Kind = layer2.KindVoice
		h.VoiceBurst, _ = enums.VoiceBurstAt(int(f.SlotType-SlotTypeVoiceA) / 0x1111)
		return h
	}
	if dt, ok := dataTypeOf(f.SlotType); ok {
		if bdt, ok := burstDataType(&f.Burst); ok {
			dt = bdt
		}
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, dt, true
	}
	return h
}

func burstDataType(b *layer2.RawBurst) (enums.DataType, bool) {
	var sync [6]byte
	for i := range sync {
		sync[i] = b[13+i]<<4 | b[14+i]>>4
	}
	p, d := enums.NearestSyncPattern(sync)
	if _, k, ok := layer2.SyncSourceOf(p); !ok || k != layer2.KindData || d > layer2.DefaultSyncThreshold {
		return 0, false
	}
	cw := (uint32(b[12])&0x3F)<<14 | uint32(b[13])>>4<<10 | (uint32(b[19])&0x0F)<<6 | uint32(b[20])>>2
	info, res := pdu.DecodeSlotTypeCodeword(cw)
	if res.Uncorrectable {
		return 0, false
	}
	return info.SlotType().DataType, true
}

func dataTypeOf(s SlotType) (enums.DataType, bool) {
	switch s {
	case SlotTypePIHeader:
		return enums.DataTypePIHeader, true
	case SlotTypeVoiceLCHeader:
		return enums.DataTypeVoiceLCHeader, true
	case SlotTypeTerminator:
		return enums.DataTypeTerminatorWithLC, true
	case SlotTypeCSBK:
		return enums.DataTypeCSBK, true
	case SlotTypeDataHeader:
		return enums.DataTypeDataHeader, true
	case SlotTypeRate12:
		return enums.DataTypeRate12, true
	case SlotTypeRate34:
		return enums.DataTypeRate34, true
	case SlotTypeVoiceA, SlotTypeVoiceB, SlotTypeVoiceC, SlotTypeVoiceD, SlotTypeVoiceE, SlotTypeVoiceF, SlotTypeWakeup, SlotTypeSync:
	}
	return 0, false
}

// SetHint sets SlotType from the burst kind in h, as a machine Output
// gives it: KindVoice with a VoiceBurst of A to F, or KindData with a
// DataType. It sets Slot when h.Slot is Slot1 or Slot2. It returns an
// error, and changes nothing, for another kind, a position that is not A
// to F, a KindData hint without a data type, or a data type that Hytera
// has no slot type for (MBC, idle, rate 1 and unified single block data).
// The kind, position and data type errors wrap layer2.ErrKind,
// layer2.ErrVoicePosition and layer2.ErrDataType.
func (f *Frame) SetHint(h layer2.Hint) error {
	if f == nil {
		return dmrgo.ErrNilReceiver
	}
	s, err := slotTypeOf(&h)
	if err != nil {
		return err
	}
	f.SlotType = s
	if h.Slot == enums.Slot1 || h.Slot == enums.Slot2 {
		f.Slot = h.Slot
	}
	return nil
}

// SetCallHint sets the call as SetCall does and the slot type as SetHint
// does, from the Call and Hint of a machine Output. It returns the error of
// either, and then changes nothing.
func (f *Frame) SetCallHint(h layer2.Hint, c layer2.Call) error {
	if f == nil {
		return dmrgo.ErrNilReceiver
	}
	callType, target, source := f.CallType, f.Destination, f.Source
	if err := f.SetCall(c.Group, c.Target, c.Source); err != nil {
		return err
	}
	if err := f.SetHint(h); err != nil {
		f.CallType, f.Destination, f.Source = callType, target, source
		return err
	}
	return nil
}

func slotTypeOf(h *layer2.Hint) (SlotType, error) {
	switch h.Kind {
	case layer2.KindVoice:
		i, ok := h.VoiceBurst.Index()
		if !ok {
			return 0, errKindVoice
		}
		return SlotTypeVoiceA + SlotType(i&7)*0x1111, nil
	case layer2.KindData:
		if !h.HasDataType {
			return 0, errKindData
		}
		for _, s := range [...]SlotType{SlotTypePIHeader, SlotTypeVoiceLCHeader, SlotTypeTerminator, SlotTypeCSBK, SlotTypeDataHeader, SlotTypeRate12, SlotTypeRate34} {
			if d, _ := dataTypeOf(s); d == h.DataType {
				return s, nil
			}
		}
		return 0, errKindData
	case layer2.KindUnknown, layer2.KindReverseChannel:
	}
	return 0, errKind
}
