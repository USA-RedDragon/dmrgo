package dmrd

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// FrameType is the frame type of a DMRD packet, octet 15 bits 5 and 4.
type FrameType uint8

// The FrameType values.
const (
	FrameVoice     FrameType = iota // a voice burst B to F, or A with Value 0
	FrameVoiceSync                  // voice burst A
	FrameDataSync                   // a data or control burst; Value is its data type
	FrameReserved                   // not used; MMDVMHost reads it as FrameDataSync
)

// String returns the name of the frame type. It allocates only for a value
// without a name.
func (f FrameType) String() string {
	switch f {
	case FrameVoice:
		return "Voice"
	case FrameVoiceSync:
		return "Voice Sync"
	case FrameDataSync:
		return "Data Sync"
	case FrameReserved:
		return "Reserved"
	}
	return "FrameType(" + text.Int(int64(f)) + ")"
}

// Data is a DMRD packet: one burst with its call fields. The 53-octet core
// is the same in every dialect; DMRGateway and MMDVMHost add the BER and
// RSSI octets (55 octets), and DMRGateway with trunking adds a UUID (71
// octets).
//
//dmr:frame 53 55 71
//dmr:magic 0-31="DMRD"
type Data struct {
	// Seq is the sequence number of the packet in its stream; it wraps after 255.
	Seq uint8 `dmr:"bits:32-39"`
	// Source is the radio ID of the caller (24 bits).
	Source uint32 `dmr:"bits:40-63"`
	// Destination is the talkgroup or radio ID that is called (24 bits).
	Destination uint32 `dmr:"bits:64-87"`
	// Repeater is the repeater ID, or for OpenBridge the network ID.
	Repeater uint32 `dmr:"bits:88-119"`
	// Slot is the TDMA channel, Slot1 or Slot2.
	Slot enums.Slot `dmr:"bits:120,enum,bias:1"`
	// Private reports a unit to unit call; false is a group call.
	Private bool `dmr:"bit:121"`
	// Frame is the frame type.
	Frame FrameType `dmr:"bits:122-123,enum"`
	// Value is the data type of a FrameDataSync packet, or the voice
	// position N (0 to 5 for A to F) of a voice packet, as sent.
	Value uint8 `dmr:"bits:124-127"`
	// Stream identifies the call. DMRGateway copies it in the byte order of
	// its host, so treat it as opaque.
	Stream uint32 `dmr:"bits:128-159"`
	// Burst is the 33-octet burst in transmit bit order.
	Burst layer2.RawBurst `dmr:"bits:160-423,packed"`
	// BER is the bit error count that the repeater measured (55 and 71 octets).
	BER uint8 `dmr:"bits:424-431"`
	// RSSI is the received signal strength in dBm, as a positive number (55
	// and 71 octets).
	RSSI uint8 `dmr:"bits:432-439"`
	// UUID is the trunking UUID of DMRGateway (71 octets).
	UUID [16]byte `dmr:"bits:440-567,packed"`
	// Size is the length of the packet: 53, 55 or 71, or 0 for 53.
	Size uint8 `dmr:"-"`
}

// Hint returns what the packet tells about its burst, for the service
// machines: the slot, the voice position or data type, the sequence number
// (8 bits) and the stream ID. A FrameVoice packet with a position above 5
// gives KindVoice with the position unknown. A nil d gives the zero Hint.
func (d *Data) Hint() layer2.Hint {
	if d == nil {
		return layer2.Hint{}
	}
	h := layer2.Hint{
		Slot:      d.Slot,
		Seq:       uint16(d.Seq),
		HasSeq:    true,
		SeqBits:   8,
		Stream:    d.Stream,
		HasStream: true,
	}
	switch d.Frame {
	case FrameVoiceSync:
		h.Kind, h.VoiceBurst = layer2.KindVoice, enums.VoiceBurstA
	case FrameVoice:
		h.Kind = layer2.KindVoice
		h.VoiceBurst, _ = enums.VoiceBurstAt(int(d.Value))
	case FrameDataSync, FrameReserved:
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, enums.DataType(d.Value), true
	}
	return h
}

// SetKind sets Frame and Value from the layer2 view of the burst:
// KindVoice with pos A to F, or KindData with dt. Burst A becomes
// FrameVoiceSync, as MMDVMHost sends it. It returns an error, and changes
// nothing, for another kind, an error that wraps layer2.ErrVoicePosition
// for a position that is not A to F, and one that wraps layer2.ErrDataType
// for a data type above 15.
func (d *Data) SetKind(k layer2.Kind, pos enums.VoiceBurstType, dt enums.DataType) error {
	if d == nil {
		return dmrgo.ErrNilReceiver
	}

	switch k {
	case layer2.KindVoice:
		if _, ok := pos.Index(); !ok {
			return errKindVoice
		}
		d.Frame, d.Value = FrameVoice, uint8(pos-enums.VoiceBurstA)
		if pos == enums.VoiceBurstA {
			d.Frame = FrameVoiceSync
		}
	case layer2.KindData:
		if dt > 15 {
			return errKindData
		}
		d.Frame, d.Value = FrameDataSync, uint8(dt)
	case layer2.KindUnknown, layer2.KindReverseChannel:
		return errKind
	default:
		return errKind
	}
	return nil
}
