package hytera

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
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
	// CallType is octet 62: 0x00 on the group calls of the DO1KBL
	// capture, 0x01 on those of other repeaters, 0x0A on wakeup packets.
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
// and an error when Slot is not Slot1 or Slot2, ColorCode is above 15, or
// Destination or Source does not fit in 24 bits. It allocates only when dst
// has no room for FrameLen more octets.
func (f Frame) AppendBinary(dst []byte) ([]byte, error) {
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
		return dst, errColor
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

// HasBurst reports whether Burst holds a burst: false for SlotTypeSync and
// SlotTypeWakeup, whose payload is not an ETSI burst.
func (f Frame) HasBurst() bool {
	return f.SlotType != SlotTypeSync && f.SlotType != SlotTypeWakeup
}

// Hint returns what the frame tells about its burst, for the service
// machines: the slot, the voice position or data type from SlotType, and
// the sequence number (8 bits). A slot type without a burst kind, such as
// one that no source names, gives KindUnknown; use HasBurst first.
func (f Frame) Hint() layer2.Hint {
	h := layer2.Hint{Slot: f.Slot, Seq: uint16(f.Seq), HasSeq: true, SeqBits: 8}
	if f.SlotType >= SlotTypeVoiceA && f.SlotType <= SlotTypeVoiceF && f.SlotType%0x1111 == 0 {
		h.Kind = layer2.KindVoice
		h.VoiceBurst, _ = enums.VoiceBurstAt(int(f.SlotType-SlotTypeVoiceA) / 0x1111)
		return h
	}
	if dt, ok := dataTypeOf(f.SlotType); ok {
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, dt, true
	}
	return h
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

// SetKind sets SlotType from the layer2 view of the burst: KindVoice with
// pos A to F, or KindData with dt. It returns an error, and changes
// nothing, for another kind, a position that is not A to F, or a data type
// that Hytera has no slot type for (MBC, idle, rate 1 and unified single
// block data).
func (f *Frame) SetKind(k layer2.Kind, pos enums.VoiceBurstType, dt enums.DataType) error {
	s, ok := slotTypeOf(k, pos, dt)
	if !ok {
		return errKind
	}
	f.SlotType = s
	return nil
}

func slotTypeOf(k layer2.Kind, pos enums.VoiceBurstType, dt enums.DataType) (SlotType, bool) {
	switch k {
	case layer2.KindVoice:
		i, ok := pos.Index()
		return SlotTypeVoiceA + SlotType(i&7)*0x1111, ok
	case layer2.KindData:
		for _, s := range [...]SlotType{SlotTypePIHeader, SlotTypeVoiceLCHeader, SlotTypeTerminator, SlotTypeCSBK, SlotTypeDataHeader, SlotTypeRate12, SlotTypeRate34} {
			if d, _ := dataTypeOf(s); d == dt {
				return s, true
			}
		}
	case layer2.KindUnknown, layer2.KindReverseChannel:
	}
	return 0, false
}
