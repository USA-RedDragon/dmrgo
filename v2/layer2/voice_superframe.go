package layer2

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// ETSI TS 102 361-1 §5.1.2 — Voice superframe, initiation and termination
//
// A voice transmission is a Voice LC Header, then complete superframes of
// six voice bursts A to F, then a Terminator with LC in the burst after the
// last burst F (§5.1.2.3; ETSI TS 102 361-2 §5.2.1.1 and §5.2.2.1 transmit
// the entire last superframe). Burst A carries a voice SYNC pattern. Bursts B to E
// carry the embedded Full LC with LCSS First, Continuation, Continuation
// and Last (§7.1.3.0). Burst F carries the Null embedded message (§D.1)
// with LCSS Single fragment, or an outbound RC (§7.1.3.1, §6.4.2).
//
// Each voice burst carries three 72-bit vocoder slots (§6.1). The slots are
// opaque here: an AMBE frame is a slot after vocoder.EncodeAMBE72, and any
// other codec supplies its own 72-bit slots.

// VoiceSlotsPerBurst is the number of 72-bit vocoder slots in a voice burst.
const VoiceSlotsPerBurst = 3

// VoiceBurstsPerSuperframe is the number of voice bursts in a superframe.
const VoiceBurstsPerSuperframe = 6

// ErrVoiceTerminated is returned by VoiceTransmitter methods called after
// Terminate.
var ErrVoiceTerminated = errors.New("voice transmission already terminated")

// SyncSource selects the SYNC patterns of a voice transmission (ETSI TS 102
// 361-1 Table 9.2).
type SyncSource int

const (
	// SyncSourceBS is a BS-sourced (outbound) transmission.
	SyncSourceBS SyncSource = iota
	// SyncSourceMS is an MS-sourced (inbound) transmission.
	SyncSourceMS
	// SyncSourceDirectTS1 is a direct mode transmission on TDMA time slot 1.
	SyncSourceDirectTS1
	// SyncSourceDirectTS2 is a direct mode transmission on TDMA time slot 2.
	SyncSourceDirectTS2
)

// SyncPatterns returns the voice and data SYNC patterns of the source. ok is
// false for an unknown source.
func (s SyncSource) SyncPatterns() (voice, data enums.SyncPattern, ok bool) {
	switch s {
	case SyncSourceBS:
		return enums.SyncBSSourcedVoice, enums.SyncBSSourcedData, true
	case SyncSourceMS:
		return enums.SyncMSSourcedVoice, enums.SyncMSSourcedData, true
	case SyncSourceDirectTS1:
		return enums.SyncTDMA1Voice, enums.SyncTDMA1Data, true
	case SyncSourceDirectTS2:
		return enums.SyncTDMA2Voice, enums.SyncTDMA2Data, true
	}
	return 0, 0, false
}

// SyncSourceOf returns the source and the burst kind of the SYNC pattern p
// (ETSI TS 102 361-1 Table 9.2): KindVoice or KindData for the voice and
// data patterns, and SyncSourceMS with KindReverseChannel for the
// MS-sourced RC pattern. It is the inverse of SyncSource.SyncPatterns. ok
// is false for enums.SyncNone, the reserved pattern and any other value.
func SyncSourceOf(p enums.SyncPattern) (src SyncSource, k Kind, ok bool) {
	switch p {
	case enums.SyncBSSourcedVoice:
		return SyncSourceBS, KindVoice, true
	case enums.SyncBSSourcedData:
		return SyncSourceBS, KindData, true
	case enums.SyncMSSourcedVoice:
		return SyncSourceMS, KindVoice, true
	case enums.SyncMSSourcedData:
		return SyncSourceMS, KindData, true
	case enums.SyncMSSourcedRC:
		return SyncSourceMS, KindReverseChannel, true
	case enums.SyncTDMA1Voice:
		return SyncSourceDirectTS1, KindVoice, true
	case enums.SyncTDMA1Data:
		return SyncSourceDirectTS1, KindData, true
	case enums.SyncTDMA2Voice:
		return SyncSourceDirectTS2, KindVoice, true
	case enums.SyncTDMA2Data:
		return SyncSourceDirectTS2, KindData, true
	case enums.SyncNone, enums.SyncReserved:
	}
	return 0, KindUnknown, false
}

// VoiceTransmitter builds the bursts of one voice transmission from a
// stream of 72-bit vocoder slots. Each call returns the bursts it completed,
// one burst for every three slots, so a caller that adds three slots every
// 60 ms gets one burst to send in each of its time slots. A
// VoiceTransmitter is not safe for concurrent use.
type VoiceTransmitter struct {
	colorCode  uint8
	voiceSync  enums.SyncPattern
	outbound   bool
	header     [33]byte
	terminator [33]byte
	embedded   [4][32]bit.Bit
	callLC     [4][32]bit.Bit
	nextLC     *[4][32]bit.Bit
	lcHook     func(superframe int) *pdu.FullLinkControl
	superframe int
	piHeader   [33]byte
	hasPI      bool
	silence    [72]bit.Bit
	slots      [VoiceSlotsPerBurst][72]bit.Bit
	pending    int
	position   int
	rc         [32]bit.Bit
	hasRC      bool
	terminated bool
}

// NewVoiceTransmitter returns a VoiceTransmitter for a transmission with
// Full LC lc. The header, the embedded LC and the terminator all carry lc,
// with its FID, and lc.BurstDataType is ignored. silence is the 72-bit slot that
// pads the last superframe; it is codec specific, for AMBE the
// vocoder.EncodeAMBE72 of a silence frame. NewVoiceTransmitter returns an
// error when colorCode does not fit in 4 bits, source is unknown or lc does
// not encode. It is NewVoiceTransmitterWithOptions with only these options.
func NewVoiceTransmitter(lc pdu.FullLinkControl, colorCode uint8, source SyncSource, silence [72]bit.Bit) (*VoiceTransmitter, error) {
	return NewVoiceTransmitterWithOptions(VoiceTransmitterOptions{
		LC:        lc,
		ColorCode: colorCode,
		Source:    source,
		Silence:   silence,
	})
}

func lcDataBurst(lc pdu.FullLinkControl, dataType enums.DataType, colorCode uint8, src SyncSource) ([33]byte, error) {
	lc.BurstDataType = dataType
	bits, err := lc.Encode()
	if err != nil {
		return [33]byte{}, fmt.Errorf("cannot encode full link control: %w", err)
	}
	return BuildDataBurst(bit.Pack96(bits), dataType, colorCode, src)
}

// Header returns the Voice LC Header burst, sent before the first voice
// burst.
func (t *VoiceTransmitter) Header() [33]byte {
	return t.header
}

// SetReverseChannel sends rc in place of the Null embedded message in the
// next burst F (ETSI TS 102 361-1 §7.1.3.1). Only a BS-sourced transmission
// carries an RC in burst F, and it carries one RC per superframe at most, so
// a later call before that burst F replaces rc.
func (t *VoiceTransmitter) SetReverseChannel(rc pdu.ReverseChannel) error {
	if t.terminated {
		return ErrVoiceTerminated
	}
	if !t.outbound {
		return errors.New("only a BS-sourced voice burst F carries a reverse channel")
	}
	bits, err := EncodeEmbeddedRC(&rc)
	if err != nil {
		return err
	}
	t.rc, t.hasRC = bits, true
	return nil
}

// AddSlots adds 72-bit vocoder slots in transmit order and returns the
// voice bursts they complete.
func (t *VoiceTransmitter) AddSlots(slots ...[72]bit.Bit) ([][33]byte, error) {
	if t.terminated {
		return nil, ErrVoiceTerminated
	}
	var out [][33]byte
	for _, slot := range slots {
		t.slots[t.pending] = slot
		t.pending++
		if t.pending == VoiceSlotsPerBurst {
			burst, err := t.flush()
			if err != nil {
				return out, err
			}
			out = append(out, burst)
		}
	}
	return out, nil
}

// AddFrames adds AMBE+2 frames in transmit order, each as the slot
// vocoder.EncodeAMBE72 returns, and returns the voice bursts they complete.
func (t *VoiceTransmitter) AddFrames(frames ...vocoder.AMBEFrame) ([][33]byte, error) {
	slots := make([][72]bit.Bit, len(frames))
	for i, frame := range frames {
		slots[i] = vocoder.EncodeAMBE72(frame)
	}
	return t.AddSlots(slots...)
}

// Terminate ends the transmission. It fills the last burst and then the last
// superframe with the silence slot and returns those bursts followed by the
// Terminator with LC. A transmission without voice bursts ends with the
// terminator alone. Later calls return ErrVoiceTerminated.
func (t *VoiceTransmitter) Terminate() ([][33]byte, error) {
	if t.terminated {
		return nil, ErrVoiceTerminated
	}
	var out [][33]byte
	for t.pending > 0 || t.position != 0 {
		for t.pending < VoiceSlotsPerBurst {
			t.slots[t.pending] = t.silence
			t.pending++
		}
		burst, err := t.flush()
		if err != nil {
			return out, err
		}
		out = append(out, burst)
	}
	t.terminated = true
	return append(out, t.terminator), nil
}

func (t *VoiceTransmitter) flush() ([33]byte, error) {
	var bits [264]bit.Bit
	var voice [216]bit.Bit
	for i, slot := range t.slots {
		copy(voice[i*72:], slot[:])
	}
	copy(bits[:108], voice[:108])
	copy(bits[156:], voice[108:])

	if t.position == 0 {
		if err := t.startSuperframe(); err != nil {
			t.pending = 0
			return [33]byte{}, err
		}
		writeSync(&bits, t.voiceSync)
	} else {
		emb := pdu.EmbeddedSignalling{ColorCode: t.colorCode}
		var data [32]bit.Bit
		switch t.position {
		case 1:
			emb.LCSS, data = enums.LCSSFirstFragment, t.embedded[0]
		case 2, 3:
			emb.LCSS, data = enums.LCSSContinuationFragment, t.embedded[t.position-1]
		case 4:
			emb.LCSS, data = enums.LCSSLastFragment, t.embedded[3]
		default:
			emb.LCSS = enums.LCSSSingleFragment
			if t.hasRC {
				emb.PreemptionAndPowerControlIndicator, data = true, t.rc
				t.hasRC = false
			}
		}
		embBits, err := emb.Encode()
		if err != nil {
			return [33]byte{}, fmt.Errorf("cannot encode embedded signalling: %w", err)
		}
		copy(bits[embFirstStart:embFirstEnd], embBits[:8])
		copy(bits[embeddedDataStart:embeddedDataEnd], data[:])
		copy(bits[embSecondStart:embSecondEnd], embBits[8:])
	}

	t.pending = 0
	t.position = (t.position + 1) % VoiceBurstsPerSuperframe
	return bit.Pack264(bits), nil
}
