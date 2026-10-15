// Package codec2 implements vocoder.Codec with the Codec2 speech codec,
// in the DMR framing that QRadioLink uses.
//
// Build requirement: by default the package binds the system libcodec2
// through cgo, so a build that imports it needs a C compiler and libcodec2
// with its pkg-config file (pkg-config name codec2), for example the
// libcodec2-dev package on Debian. Without them the build fails with
// "Package 'codec2' not found". Build with -tags nocodec2 or CGO_ENABLED=0
// to remove the dependency; New then returns ErrUnavailable.
//
// Each 20 ms Codec2 frame (160 samples at 8 kHz) fills one 72-bit DMR
// vocoder frame, so a 60 ms voice burst carries three. The Codec2 bytes go
// MSB first at the start of the 9-byte frame and the remaining bits are zero.
// There is no FEC, scrambling or interleaving. Calls are marked with the
// Feature set ID 0xC2 in the Full LC. The FID does not identify the Codec2
// mode, so both ends must agree on it out of band.
//
// libcodec2 is licensed under the GNU LGPL 2.1. This package links it
// dynamically and contains none of its source. A program that links
// libcodec2 statically must meet the LGPL terms for that copy.
package codec2

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// Name is the name the codec registers under in the vocoder registry.
const Name = "codec2"

// DefaultFID is the Full LC Feature set ID QRadioLink sends for Codec2
// calls. It is in the range ETSI TS 102 361-1 Table 9.21 reserves for future
// MFID allocation (1xxxxxxx), so it is self-assigned, not allocated by ETSI.
const DefaultFID enums.FeatureSetID = 0xC2

// FrameSamples is the number of 8 kHz PCM samples in one Codec2 frame.
const FrameSamples = 160

// Mode is a Codec2 bit rate in bits per second.
type Mode int

// The Codec2 modes that fit in a 72-bit DMR vocoder frame.
const (
	Mode2400 Mode = 2400
	Mode3200 Mode = 3200
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

var (
	// ErrUnavailable is returned when the package was built without
	// libcodec2 (CGO_ENABLED=0 or -tags nocodec2).
	ErrUnavailable = errors.New("codec2: built without libcodec2")
	// ErrUnsupportedMode is returned for a Mode other than Mode2400 or Mode3200.
	ErrUnsupportedMode = errors.New("codec2: unsupported mode")
	// ErrFrameLength is returned when Encode gets other than FrameSamples samples.
	ErrFrameLength = errors.New("codec2: wrong number of samples")
	// ErrClosed is returned when a closed Codec is used.
	ErrClosed = errors.New("codec2: codec is closed")
)

var _ vocoder.Codec = (*Codec)(nil)

func init() { //nolint:gochecknoinits // registers the codec, like database/sql drivers
	vocoder.Register(Name, func() (vocoder.Codec, error) {
		c, err := New(Config{})
		if err != nil {
			return nil, err
		}
		return c, nil
	})
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

func unpackSlot(slot [72]bit.Bit, n int) []byte {
	frame := make([]byte, n)
	for i := 0; i < n*8; i++ {
		frame[i/8] |= byte(slot[i]&1) << (7 - i%8)
	}
	return frame
}
