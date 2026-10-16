package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
	// (ETSI TS 102 361-1 clause 5.1.2.2, clause 6.2).
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
	t := new(VoiceTransmitter)
	if err := t.Reset(opts); err != nil {
		return nil, err
	}
	return t, nil
}

// Reset starts a new transmission with opts, as NewVoiceTransmitter does,
// in the storage of t, so that a caller can keep one VoiceTransmitter for
// many calls. A nil opts means the zero VoiceTransmitterOptions. On an
// error t is left as it was. It does not allocate.
func (t *VoiceTransmitter) Reset(opts *VoiceTransmitterOptions) error {
	if opts == nil {
		opts = &VoiceTransmitterOptions{}
	}
	voiceSync, _, ok := opts.Source.SyncPatterns()
	if !ok {
		return newSyncSourceError(opts.Source)
	}
	header, err := lcDataBurst(&opts.LC, enums.DataTypeVoiceLCHeader, opts.ColorCode, opts.Source)
	if err != nil {
		return err
	}
	terminator, err := lcDataBurst(&opts.LC, enums.DataTypeTerminatorWithLC, opts.ColorCode, opts.Source)
	if err != nil {
		return err
	}
	callLC, err := encodeEmbeddedFullLinkControl(&opts.LC)
	if err != nil {
		return err
	}
	var piHeader RawBurst
	if opts.PIHeader != nil {
		coded, err := opts.PIHeader.Encode()
		if err != nil {
			return newWrappedError("cannot encode PI header", err)
		}
		var p [12]byte
		packBits(p[:], coded[:])
		if piHeader, err = BuildDataBurst(p, enums.DataTypePIHeader, opts.ColorCode, opts.Source); err != nil {
			return err
		}
	}
	*t = VoiceTransmitter{
		colorCode:  opts.ColorCode & 0x0F,
		voiceSync:  voiceSync,
		outbound:   opts.Source == SyncSourceBS,
		header:     header,
		terminator: terminator,
		embedded:   callLC,
		callLC:     callLC,
		lcHook:     opts.EmbeddedLC,
		piHeader:   piHeader,
		hasPI:      opts.PIHeader != nil,
	}
	packBits(t.silence[:], opts.Silence[:])
	return nil
}

// Headers returns the bursts sent before the first voice burst: the Voice
// LC Header, then the PI Header when the options set one. It allocates the
// slice; AppendHeaders does not.
func (t *VoiceTransmitter) Headers() []RawBurst {
	return t.AppendHeaders(make([]RawBurst, 0, 2))
}

// AppendHeaders appends the bursts that Headers returns to dst and returns
// the extended slice. It does not allocate when dst has room for 2 bursts.
func (t *VoiceTransmitter) AppendHeaders(dst []RawBurst) []RawBurst {
	dst = append(dst, t.header)
	if t.hasPI {
		dst = append(dst, t.piHeader)
	}
	return dst
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
	t.nextLC, t.hasNextLC = frags, true
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
	t.nextLC, t.hasNextLC = fragmentWords(f), true
	return nil
}

func (t *VoiceTransmitter) startSuperframe() error {
	n := t.superframe
	frags := t.callLC
	switch {
	case t.hasNextLC:
		frags = t.nextLC
	case t.lcHook != nil:
		if lc := t.lcHook(n); lc != nil {
			var err error
			if frags, err = encodeEmbeddedFullLinkControl(lc); err != nil {
				return newSuperframeLCError(n, err)
			}
		}
	}
	t.embedded, t.hasNextLC = frags, false
	t.superframe++
	return nil
}
