package layer2

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// VoiceTransmitterOptions configures NewVoiceTransmitterWithOptions.
type VoiceTransmitterOptions struct {
	// LC is the Full LC of the call. The Voice LC Header and the Terminator
	// with LC carry it, and so does the embedded LC unless EmbeddedLC or
	// SetEmbeddedLC replaces it. LC.DataType is ignored.
	LC pdu.FullLinkControl
	// ColorCode is the 4-bit color code.
	ColorCode uint8
	// Source selects the SYNC patterns.
	Source SyncSource
	// Silence is the 72-bit slot that pads the last superframe.
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

// NewVoiceTransmitterWithOptions returns a VoiceTransmitter for opts. It
// returns an error when ColorCode does not fit in 4 bits, Source is unknown,
// or LC or PIHeader does not encode.
func NewVoiceTransmitterWithOptions(opts VoiceTransmitterOptions) (*VoiceTransmitter, error) {
	bit.Mask(opts.Silence[:])
	voiceSync, dataSync, ok := opts.Source.SyncPatterns()
	if !ok {
		return nil, fmt.Errorf("unknown sync source %d", opts.Source)
	}
	if opts.ColorCode > 15 {
		return nil, fmt.Errorf("color code %d does not fit in 4 bits", opts.ColorCode)
	}
	t := &VoiceTransmitter{
		colorCode: int(opts.ColorCode),
		voiceSync: voiceSync,
		outbound:  opts.Source == SyncSourceBS,
		silence:   opts.Silence,
		lcHook:    opts.EmbeddedLC,
	}
	var err error
	if t.header, err = lcDataBurst(opts.LC, enums.DataTypeVoiceLCHeader, opts.ColorCode, dataSync); err != nil {
		return nil, err
	}
	if t.terminator, err = lcDataBurst(opts.LC, enums.DataTypeTerminatorWithLC, opts.ColorCode, dataSync); err != nil {
		return nil, err
	}
	if t.callLC, err = EncodeEmbeddedFullLinkControl(&opts.LC); err != nil {
		return nil, err
	}
	t.embedded = t.callLC
	if opts.PIHeader != nil {
		pi := *opts.PIHeader
		pi.DataType = enums.DataTypePIHeader
		bits, err := pdu.EncodePIHeader(&pi)
		if err != nil {
			return nil, fmt.Errorf("cannot encode PI header: %w", err)
		}
		var piBytes [12]byte
		copy(piBytes[:], bit.PackBits(bits[:]))
		if t.piHeader, err = buildLCDataBurst(piBytes, enums.DataTypePIHeader, opts.ColorCode, dataSync); err != nil {
			return nil, err
		}
		t.hasPI = true
	}
	return t, nil
}

// Headers returns the bursts sent before the first voice burst: the Voice
// LC Header, then the PI Header when the options set one.
func (t *VoiceTransmitter) Headers() [][33]byte {
	if t.hasPI {
		return [][33]byte{t.header, t.piHeader}
	}
	return [][33]byte{t.header}
}

// SetEmbeddedLC sets the embedded LC of the next superframe that starts. It
// takes precedence over the EmbeddedLC option for that superframe only.
func (t *VoiceTransmitter) SetEmbeddedLC(lc pdu.FullLinkControl) error {
	if t.terminated {
		return ErrVoiceTerminated
	}
	frags, err := EncodeEmbeddedFullLinkControl(&lc)
	if err != nil {
		return err
	}
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
			if frags, err = EncodeEmbeddedFullLinkControl(lc); err != nil {
				return fmt.Errorf("superframe %d embedded LC: %w", n, err)
			}
		}
	}
	t.embedded, t.nextLC = frags, nil
	t.superframe++
	return nil
}
