//go:build !cgo || nocodec2 || tinygo

package codec2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
)

// Codec is a Codec2 vocoder.Codec. This build has no libcodec2, so New
// always returns ErrUnavailable and every method reports ErrUnavailable.
type Codec struct{}

// New returns ErrUnavailable because this build has no libcodec2.
func New(Config) (*Codec, error) {
	return nil, ErrUnavailable
}

// FrameSamples returns FrameSamples.
func (c *Codec) FrameSamples() int { return FrameSamples }

// Mode returns zero.
func (c *Codec) Mode() Mode { return 0 }

// FID returns zero.
func (c *Codec) FID() enums.FeatureSetID { return 0 }

// Silence returns all-zero bits.
func (c *Codec) Silence() [72]bit.Bit { return [72]bit.Bit{} }

// Encode returns ErrUnavailable.
func (c *Codec) Encode([]int16) ([72]bit.Bit, error) { return [72]bit.Bit{}, ErrUnavailable }

// Decode returns ErrUnavailable.
func (c *Codec) Decode([72]bit.Bit) ([]int16, error) { return nil, ErrUnavailable }

// AppendDecode returns dst and ErrUnavailable.
func (c *Codec) AppendDecode(dst []int16, _ [72]bit.Bit) ([]int16, error) {
	return dst, ErrUnavailable
}

// Close does nothing.
func (c *Codec) Close() error { return nil }
