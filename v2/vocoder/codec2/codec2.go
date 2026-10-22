package codec2

import (
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// DefaultFID is the Full LC Feature set ID QRadioLink sends for Codec2
// calls. It is in the range that is reserved for future MFID allocation
// (1xxxxxxx), and QRadioLink assigned it itself (ETSI TS 102 361-1 Table 9.21).
const DefaultFID enums.FID = 0xC2

// FrameSamples is the number of 8 kHz PCM samples in one Codec2 frame.
const FrameSamples = 160

// Mode is a Codec2 bit rate in bits per second.
type Mode int

// The Codec2 modes that fit in a 72-bit DMR vocoder frame, and ModeAuto.
const (
	ModeAuto Mode = 0    // decode the mode that the frames of each call show, encode Mode2400
	Mode2400 Mode = 2400 // 2400 bit/s, 6 bytes per frame
	Mode3200 Mode = 3200 // 3200 bit/s, 8 bytes per frame
)

// Config selects how a Codec encodes and decodes.
type Config struct {
	// Mode is the Codec2 bit rate. Zero is ModeAuto: the decoder detects
	// Mode2400 or Mode3200 from the padding of the frames of each call, and
	// the encoder uses Mode2400.
	Mode Mode
	// FID is the Feature set ID to send in the Full LC. Zero selects
	// DefaultFID. FID 0 is the ETSI feature set, which repeaters treat as
	// AMBE, so it cannot be selected.
	FID enums.FID
}

var _ vocoder.Codec = (*Codec)(nil)

func (cfg Config) withDefaults() (Config, error) {
	if cfg.FID == 0 {
		cfg.FID = DefaultFID
	}
	switch cfg.Mode {
	case ModeAuto, Mode2400, Mode3200:
		return cfg, nil
	default:
		return cfg, ErrUnsupportedMode
	}
}

func (m Mode) frameBytes() int {
	if m == ModeAuto {
		m = Mode2400
	}
	return int(m) / 50 / 8
}
