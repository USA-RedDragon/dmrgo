package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// VoiceTransmitterOptions configures NewVoiceTransmitter.
type VoiceTransmitterOptions struct {
	// LC is the Full LC of the call. The Voice LC Header and the Terminator
	// with LC carry it, with its FID, and so does the embedded LC unless
	// EmbeddedLC or SetEmbeddedLC replaces it. LC.BurstDataType is ignored.
	LC pdu.FullLinkControl
	// ColorCode is the color code of the slot types and EMBs.
	ColorCode enums.ColorCode
	// Source selects the SYNC patterns. The zero value is SyncSourceBS.
	Source SyncSource
	// Silence is the 72-bit slot that pads the last superframe. It is codec
	// specific: for AMBE+2 it is vocoder.AMBESilenceSlot().
	Silence [72]bit.Bit
	// PIHeader, when set, adds a PI Header burst after the Voice LC Header
	// (ETSI TS 102 361-1 §5.1.2.2, §6.2).
	PIHeader *pdu.PIHeader
	// EmbeddedLC, when set, is called at the start of each superframe with
	// its index, 0 for the first one. The Full LC it returns is the embedded
	// LC of bursts B to E of that superframe, for example a talker alias or
	// GPS LC. A nil return keeps LC. An LC that does not encode makes
	// AddSlots, AddFrames or Terminate return the error.
	EmbeddedLC func(superframe int) *pdu.FullLinkControl
}

// NewVoiceTransmitter returns a VoiceTransmitter for opts. A nil opts means
// the zero VoiceTransmitterOptions. It returns an error when Source is
// unknown, or LC or PIHeader does not encode.
func NewVoiceTransmitter(opts *VoiceTransmitterOptions) (*VoiceTransmitter, error) {
	if opts == nil {
		opts = &VoiceTransmitterOptions{}
	}
	silence := opts.Silence
	lowbit.Mask(silence[:])
	voiceSync, _, ok := opts.Source.SyncPatterns()
	if !ok {
		return nil, fmt.Errorf("unknown sync source %d", opts.Source)
	}

	t := &VoiceTransmitter{
		colorCode: opts.ColorCode & 0x0F,
		voiceSync: voiceSync,
		outbound:  opts.Source == SyncSourceBS,
		silence:   silence,
		lcHook:    opts.EmbeddedLC,
	}
	var err error
	if t.header, err = lcDataBurst(opts.LC, enums.DataTypeVoiceLCHeader, opts.ColorCode, opts.Source); err != nil {
		return nil, err
	}
	if t.terminator, err = lcDataBurst(opts.LC, enums.DataTypeTerminatorWithLC, opts.ColorCode, opts.Source); err != nil {
		return nil, err
	}
	if t.callLC, err = encodeEmbeddedFullLinkControl(&opts.LC); err != nil {
		return nil, err
	}
	t.embedded = t.callLC
	if opts.PIHeader != nil {
		pi := *opts.PIHeader
		if t.piHeader, err = BuildBurst(&pi, opts.ColorCode, opts.Source); err != nil {
			return nil, fmt.Errorf("cannot encode PI header: %w", err)
		}
		t.hasPI = true
	}
	return t, nil
}

// NewVoiceTransmitterWithOptions returns NewVoiceTransmitter(&opts).
//
// Deprecated: use NewVoiceTransmitter.
func NewVoiceTransmitterWithOptions(opts VoiceTransmitterOptions) (*VoiceTransmitter, error) {
	return NewVoiceTransmitter(&opts)
}

// Headers returns the bursts sent before the first voice burst: the Voice
// LC Header, then the PI Header when the options set one.
func (t *VoiceTransmitter) Headers() []RawBurst {
	if t.hasPI {
		return []RawBurst{t.header, t.piHeader}
	}
	return []RawBurst{t.header}
}

// SetEmbeddedLC sets the embedded LC of the next superframe that starts. It
// takes precedence over the EmbeddedLC option for that superframe only.
func (t *VoiceTransmitter) SetEmbeddedLC(lc pdu.FullLinkControl) error {
	if t.terminated {
		return ErrVoiceTerminated
	}
	frags, err := encodeEmbeddedFullLinkControl(&lc)
	if err != nil {
		return err
	}
	t.nextLC = &frags
	return nil
}

// SetEmbeddedFragments sets the four embedded signalling fields of bursts B
// to E of the next superframe that starts, as RawBurst.EmbeddedData returns
// them, so a bridge can pass received fragments through unchanged. Like
// SetEmbeddedLC, it applies to that superframe only.
func (t *VoiceTransmitter) SetEmbeddedFragments(f [4][4]byte) error {
	if t.terminated {
		return ErrVoiceTerminated
	}
	frags := fragmentBits(f)
	t.nextLC = &frags
	return nil
}

func (t *VoiceTransmitter) startSuperframe() error {
	n := t.superframe
	frags := t.callLC
	switch {
	case t.nextLC != nil:
		frags = *t.nextLC
	case t.lcHook != nil:
		if lc := t.lcHook(n); lc != nil {
			var err error
			if frags, err = encodeEmbeddedFullLinkControl(lc); err != nil {
				return fmt.Errorf("superframe %d embedded LC: %w", n, err)
			}
		}
	}
	t.embedded, t.nextLC = frags, nil
	t.superframe++
	return nil
}
