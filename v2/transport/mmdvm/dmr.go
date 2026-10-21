package mmdvm

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Control is the first payload octet of a DMR data frame: how the modem
// found the burst. MMDVMHost sends 0x00 toward the modem.
type Control uint8

// The Control bits.
const (
	// ControlIdle marks a burst that the modem heard while idle, such as
	// the BS_Dwn_Act CSBK that wakes a repeater (0x80 | 0x40 | CSBK).
	ControlIdle Control = 0x80
	// ControlDataSync marks a burst with a data SYNC; the low nibble is its
	// data type.
	ControlDataSync Control = 0x40
	// ControlVoiceSync marks voice burst A, which has the voice SYNC, or a
	// voice burst at the position of A whose SYNC the modem missed.
	ControlVoiceSync Control = 0x20
)

// DataSync reports a burst with a data SYNC.
func (c Control) DataSync() bool { return c&ControlDataSync != 0 }

// VoiceSync reports voice burst A.
func (c Control) VoiceSync() bool { return c&ControlVoiceSync != 0 }

// Idle reports a burst that the modem heard while idle.
func (c Control) Idle() bool { return c&ControlIdle != 0 }

// DataType returns the data type of a burst with a data SYNC: the low
// nibble.
func (c Control) DataType() enums.DataType { return enums.DataType(c & 0x0F) }

// N returns the low nibble: 1 to 5 for voice bursts B to F.
func (c Control) N() uint8 { return uint8(c & 0x0F) }

// DMRDataLen is the length of the payload of a DMR data frame without RSSI.
const DMRDataLen = 34

// DMRData is a CommandDMRData1 or CommandDMRData2 frame, one burst of slot
// 1 or 2, or a CommandDMRLost1 or CommandDMRLost2 frame, which tells that
// the modem lost the signal of the slot. In simplex, CommandDMRData2 is the
// DMO channel.
type DMRData struct {
	// Slot is Slot1 for CommandDMRData1 and CommandDMRLost1, Slot2 for the
	// others.
	Slot enums.Slot
	// Lost reports a CommandDMRLost1 or CommandDMRLost2 frame, without a
	// burst.
	Lost bool
	// Control is how the modem found the burst.
	Control Control
	// Burst is the 33-octet burst in transmit bit order.
	Burst layer2.RawBurst
	// RSSI is the raw received signal strength, big-endian after the burst
	// from a modem built with SEND_RSSI_DATA. HasRSSI reports it.
	RSSI uint16
	// HasRSSI reports whether RSSI is set.
	HasRSSI bool
}

// DecodeDMRData decodes a DMR data or lost frame. It returns ErrCommand for
// another command, ErrShort, and ErrLength for a payload that is not 34 or
// 36 octets, or a lost frame with a payload.
func DecodeDMRData(f Frame) (DMRData, error) {
	var d DMRData
	switch f.Command {
	case CommandDMRData1, CommandDMRLost1:
		d.Slot = enums.Slot1
	case CommandDMRData2, CommandDMRLost2:
		d.Slot = enums.Slot2
	default:
		return DMRData{}, ErrCommand
	}
	if f.Command == CommandDMRLost1 || f.Command == CommandDMRLost2 {
		if len(f.Payload) != 0 {
			return DMRData{}, ErrLength
		}
		d.Lost = true
		return d, nil
	}
	p := f.Payload
	switch len(p) {
	case DMRDataLen + 2:
		d.RSSI, d.HasRSSI = uint16(p[34])<<8|uint16(p[35]), true
	case DMRDataLen:
	default:
		if len(p) < DMRDataLen {
			return DMRData{}, ErrShort
		}
		return DMRData{}, ErrLength
	}
	d.Control = Control(p[0])
	copy(d.Burst[:], p[1:DMRDataLen])
	return d, nil
}

// Command returns the command of the frame: CommandDMRData1 or
// CommandDMRData2, or CommandDMRLost1 or CommandDMRLost2 when Lost is set.
// A Slot other than Slot1 gives the slot 2 command.
func (d DMRData) Command() Command {
	switch {
	case d.Slot == enums.Slot1 && d.Lost:
		return CommandDMRLost1
	case d.Slot == enums.Slot1:
		return CommandDMRData1
	case d.Lost:
		return CommandDMRLost2
	}
	return CommandDMRData2
}

// AppendBinary appends the payload to dst: nothing for a lost frame, else
// the control octet, the burst and the RSSI when HasRSSI is set. Wrap it
// with AppendFrame and Command. It does not allocate when dst has room for
// DMRDataLen+2 more octets, and fails only for a nil d, with ErrCommand.
func (d *DMRData) AppendBinary(dst []byte) ([]byte, error) {
	if d == nil {
		return dst, ErrCommand
	}
	if d.Lost {
		return dst, nil
	}
	dst = append(dst, byte(d.Control))
	dst = append(dst, d.Burst[:]...)
	if d.HasRSSI {
		dst = append(dst, byte(d.RSSI>>8), byte(d.RSSI&0xFF))
	}
	return dst, nil
}

// Hint returns what the frame tells about its burst, for the service
// machines: the slot, and from Control the data type, voice burst A, or
// voice bursts B to F from N 1 to 5. A frame without a sync bit and with N
// 0, such as one that MMDVMHost sends, gives KindUnknown, which classifies
// the burst by its SYNC. A lost frame gives only the slot and SyncLost. The
// modem sends no sequence number; the slot period is the clock. A nil d
// gives the zero Hint.
func (d *DMRData) Hint() layer2.Hint {
	if d == nil {
		return layer2.Hint{}
	}
	h := layer2.Hint{Slot: d.Slot}
	switch {
	case d.Lost:
		h.SyncLost = true
	case d.Control.DataSync():
		h.Kind, h.DataType, h.HasDataType = layer2.KindData, d.Control.DataType(), true
	case d.Control.VoiceSync():
		h.Kind, h.VoiceBurst = layer2.KindVoice, enums.VoiceBurstA
	case d.Control.N() >= 1 && d.Control.N() <= 5:
		h.Kind = layer2.KindVoice
		h.VoiceBurst, _ = enums.VoiceBurstAt(int(d.Control.N()))
	}
	return h
}

// SetHint sets Control from what h says the burst is, as a machine Output
// gives it: ControlDataSync with the data type for KindData, ControlVoiceSync
// for voice burst A, and N 1 to 5 for voice bursts B to F. It sets Slot
// when h.Slot is Slot1 or Slot2, and Lost from h.SyncLost. It returns an
// error, and changes nothing, for another kind, a position that is not A
// to F, a KindData hint without a data type, a data type above 15, or a
// nil d. A SyncLost hint needs no kind.
func (d *DMRData) SetHint(h layer2.Hint) error {
	if d == nil {
		return errNilDMRData
	}
	var c Control
	switch {
	case h.SyncLost:
	case h.Kind == layer2.KindVoice:
		i, ok := h.VoiceBurst.Index()
		if !ok {
			return errHintVoice
		}
		c = Control(i & 0xF)
		if i == 0 {
			c = ControlVoiceSync
		}
	case h.Kind == layer2.KindData:
		if !h.HasDataType || h.DataType > 15 {
			return errHintData
		}
		c = ControlDataSync | Control(h.DataType)
	default:
		return errHintKind
	}
	d.Control, d.Lost = c, h.SyncLost
	if h.Slot == enums.Slot1 || h.Slot == enums.Slot2 {
		d.Slot = h.Slot
	}
	return nil
}

// DMRAloha is the payload of a CommandDMRAloha frame: the C_ALOHA CSBK
// burst that a Tier III control channel sends. The firmware sets its slot
// type to the configured color code.
//
//dmr:frame 33
type DMRAloha struct {
	// Burst is the 33-octet burst in transmit bit order.
	Burst layer2.RawBurst `dmr:"bits:0-263,packed"`
}
