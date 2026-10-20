//go:build !cgo || nocodec2 || tinygo

package codec2

import "github.com/USA-RedDragon/dmrgo/v2/enums"

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

// DecodeMode returns zero.
func (c *Codec) DecodeMode() Mode { return 0 }

// Reset does nothing.
func (c *Codec) Reset() {}

// FID returns zero.
func (c *Codec) FID() enums.FeatureSetID { return 0 }

// Silence returns an all-zero slot.
func (c *Codec) Silence() [9]byte { return [9]byte{} }

// Encode returns ErrUnavailable.
func (c *Codec) Encode([]int16) ([9]byte, error) { return [9]byte{}, ErrUnavailable }

// Decode returns ErrUnavailable. It returns at most FrameSamples() samples.
func (c *Codec) Decode([9]byte) ([]int16, error) { return nil, ErrUnavailable }

// AppendDecode returns dst and ErrUnavailable.
func (c *Codec) AppendDecode(dst []int16, _ [9]byte) ([]int16, error) {
	return dst, ErrUnavailable
}

// Close does nothing.
func (c *Codec) Close() error { return nil }
