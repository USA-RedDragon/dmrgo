//go:build !cgo || nocodec2 || tinygo

package codec2

import (
	"errors"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

var registerOnce sync.Once

func TestStubUnavailable(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("New error = %v, want ErrUnavailable", err)
	}
	registerOnce.Do(func() { _ = vocoder.Register(Name, NewFactory(Config{})) })
	if _, err := vocoder.New(Name); !errors.Is(err, ErrUnavailable) {
		t.Errorf("vocoder.New error = %v, want ErrUnavailable", err)
	}
	var c *Codec
	if _, err := c.Encode(make([]int16, FrameSamples)); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Encode error = %v", err)
	}
	if _, err := c.Decode([9]byte{}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("Decode error = %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("Close error = %v", err)
	}
}
