package codec2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Name is the name to register the codec under in the vocoder registry.
const Name = "codec2"

// DefaultFID is the Full LC Feature set ID QRadioLink sends for Codec2
// calls. It is in the range that is reserved for future MFID allocation
// (1xxxxxxx), and QRadioLink assigned it itself (ETSI TS 102 361-1 Table 9.21).
const DefaultFID enums.FeatureSetID = 0xC2

// FrameSamples is the number of 8 kHz PCM samples in one Codec2 frame.
const FrameSamples = 160

// Mode is a Codec2 bit rate in bits per second.
type Mode int

// The Codec2 modes that fit in a 72-bit DMR vocoder frame.
const (
	Mode2400 Mode = 2400 // 2400 bit/s, 6 bytes per frame
	Mode3200 Mode = 3200 // 3200 bit/s, 8 bytes per frame
)

// Config selects how a Codec encodes and decodes.
type Config struct {
	// Mode is the Codec2 bit rate. Zero selects Mode2400.
	Mode Mode
	// FID is the Feature set ID to send in the Full LC. Zero selects
	// DefaultFID. FID 0 is the ETSI feature set, which repeaters treat as
	// AMBE, so it cannot be selected.
	FID enums.FeatureSetID
}

var _ vocoder.AppendDecoder = (*Codec)(nil)

// NewFactory returns a vocoder codec factory that creates a Codec with cfg,
// for vocoder.Register:
//
//	vocoder.Register(codec2.Name, codec2.NewFactory(codec2.Config{}))
func NewFactory(cfg Config) func() (vocoder.Codec, error) {
	return func() (vocoder.Codec, error) {
		c, err := New(cfg)
		if err != nil {
			return nil, err
		}
		return c, nil
	}
}

func (cfg Config) withDefaults() (Config, error) {
	if cfg.Mode == 0 {
		cfg.Mode = Mode2400
	}
	if cfg.FID == 0 {
		cfg.FID = DefaultFID
	}
	switch cfg.Mode {
	case Mode2400, Mode3200:
		return cfg, nil
	default:
		return cfg, ErrUnsupportedMode
	}
}

func (m Mode) frameBytes() int {
	return int(m) / 50 / 8
}

func packSlot(frame []byte) [72]bit.Bit {
	var slot [72]bit.Bit
	for i := 0; i < len(frame)*8 && i < len(slot); i++ {
		slot[i] = bit.Bit((frame[i/8] >> (7 - i%8)) & 1)
	}
	return slot
}

// unpackSlot fills frame with the first len(frame)*8 bits of slot.
func unpackSlot(frame []byte, slot [72]bit.Bit) {
	clear(frame)
	for i := 0; i < len(frame)*8; i++ {
		frame[i/8] |= byte(slot[i]&1) << (7 - i%8)
	}
}
