package layer2

import (
	"errors"
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// VoiceSlotsPerBurst is the number of 72-bit vocoder slots in a voice burst.
const VoiceSlotsPerBurst = 3

// VoiceBurstsPerSuperframe is the number of voice bursts in a superframe.
const VoiceBurstsPerSuperframe = 6

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
	colorCode  enums.ColorCode
	voiceSync  enums.SyncPattern
	outbound   bool
	header     RawBurst
	terminator RawBurst
	embedded   [4][32]bit.Bit
	callLC     [4][32]bit.Bit
	nextLC     [4][32]bit.Bit
	hasNextLC  bool
	lcHook     func(superframe int) *pdu.FullLinkControl
	superframe int
	piHeader   RawBurst
	hasPI      bool
	silence    [72]bit.Bit
	slots      [VoiceSlotsPerBurst][72]bit.Bit
	pending    int
	position   int
	rc         [32]bit.Bit
	hasRC      bool
	terminated bool
}

func lcDataBurst(lc pdu.FullLinkControl, dataType enums.DataType, colorCode enums.ColorCode, src SyncSource) (RawBurst, error) {
	lc.BurstDataType = dataType
	r, err := BuildBurst(&lc, colorCode, src)
	if err != nil {
		return RawBurst{}, fmt.Errorf("cannot encode full link control: %w", err)
	}
	return r, nil
}

// Header returns the Voice LC Header burst, sent before the first voice
// burst.
func (t *VoiceTransmitter) Header() RawBurst {
	return t.header
}

// SetReverseChannel sends rc in place of the Null embedded message in the
// next burst F, and a later call before that burst replaces rc. It returns
// an error for a transmission that is not BS-sourced (ETSI TS 102 361-1
// clause 7.1.3.1).
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
// voice bursts they complete, as AppendSlots does with a nil dst.
func (t *VoiceTransmitter) AddSlots(slots ...[72]bit.Bit) ([]RawBurst, error) {
	return t.AppendSlots(nil, slots...)
}

// AppendSlots adds 72-bit vocoder slots in transmit order, appends the voice
// bursts they complete to dst and returns the extended slice. On an error it
// returns dst with the bursts completed before the error. It does not
// allocate when dst has room for the bursts.
func (t *VoiceTransmitter) AppendSlots(dst []RawBurst, slots ...[72]bit.Bit) ([]RawBurst, error) {
	if t.terminated {
		return dst, ErrVoiceTerminated
	}
	var err error
	for i := range slots {
		if dst, err = t.addSlot(dst, &slots[i]); err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// AddFrames adds AMBE+2 frames in transmit order, each as the slot
// vocoder.EncodeAMBE72 returns, and returns the voice bursts they complete,
// as AppendFrames does with a nil dst.
func (t *VoiceTransmitter) AddFrames(frames ...vocoder.AMBEFrame) ([]RawBurst, error) {
	return t.AppendFrames(nil, frames...)
}

// AppendFrames adds AMBE+2 frames in transmit order, each as the slot
// vocoder.EncodeAMBE72 returns, appends the voice bursts they complete to dst
// and returns the extended slice, as AppendSlots does.
func (t *VoiceTransmitter) AppendFrames(dst []RawBurst, frames ...vocoder.AMBEFrame) ([]RawBurst, error) {
	if t.terminated {
		return dst, ErrVoiceTerminated
	}
	var err error
	for _, frame := range frames {
		slot := vocoder.EncodeAMBE72(frame)
		if dst, err = t.addSlot(dst, &slot); err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// addSlot adds one slot and appends the burst that it completes to dst.
func (t *VoiceTransmitter) addSlot(dst []RawBurst, slot *[72]bit.Bit) ([]RawBurst, error) {
	t.slots[t.pending] = *slot
	t.pending++
	if t.pending < VoiceSlotsPerBurst {
		return dst, nil
	}
	burst, err := t.flush()
	if err != nil {
		return dst, err
	}
	return append(dst, burst), nil
}

// Terminate ends the transmission. It fills the last burst and then the last
// superframe with the silence slot and returns those bursts followed by the
// Terminator with LC. A transmission without voice bursts ends with the
// terminator alone. Later calls return ErrVoiceTerminated.
func (t *VoiceTransmitter) Terminate() ([]RawBurst, error) {
	if t.terminated {
		return nil, ErrVoiceTerminated
	}
	out, err := t.AppendTerminate(make([]RawBurst, 0, VoiceBurstsPerSuperframe+1))
	if len(out) == 0 {
		return nil, err
	}
	return out, err
}

// AppendTerminate ends the transmission as Terminate does, appends the
// bursts to dst and returns the extended slice. On an error it returns dst
// with the bursts completed before the error. It does not allocate when dst
// has room for VoiceBurstsPerSuperframe+1 bursts.
func (t *VoiceTransmitter) AppendTerminate(dst []RawBurst) ([]RawBurst, error) {
	if t.terminated {
		return dst, ErrVoiceTerminated
	}
	for t.pending > 0 || t.position != 0 {
		for t.pending < VoiceSlotsPerBurst {
			t.slots[t.pending] = t.silence
			t.pending++
		}
		burst, err := t.flush()
		if err != nil {
			return dst, err
		}
		dst = append(dst, burst)
	}
	t.terminated = true
	return append(dst, t.terminator), nil
}

func (t *VoiceTransmitter) flush() (RawBurst, error) {
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
			return RawBurst{}, err
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
			return RawBurst{}, fmt.Errorf("cannot encode embedded signalling: %w", err)
		}
		copy(bits[embFirstStart:embFirstEnd], embBits[:8])
		copy(bits[embeddedDataStart:embeddedDataEnd], data[:])
		copy(bits[embSecondStart:embSecondEnd], embBits[8:])
	}

	t.pending = 0
	t.position = (t.position + 1) % VoiceBurstsPerSuperframe
	return bit.Pack264(bits), nil
}
