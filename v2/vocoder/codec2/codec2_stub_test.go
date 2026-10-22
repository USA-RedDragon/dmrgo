//go:build !cgo || nocodec2 || tinygo

package codec2

import (
	"errors"
	"testing"
)

func TestStubUnavailable(t *testing.T) {
	t.Parallel()

	if _, err := New(nil); !errors.Is(err, ErrUnavailable) {
		t.Errorf("New error = %v, want ErrUnavailable", err)
	}
	var c *Codec
	if _, err := c.Encode(make([]int16, FrameSamples)); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Encode error = %v", err)
	}
	if _, err := c.Decode(nil, [9]byte{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Decode error = %v", err)
	}
	c.Reset()
	if c.DecodeMode() != 0 {
		t.Errorf("DecodeMode = %d", c.DecodeMode())
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close error = %v", err)
	}
}
