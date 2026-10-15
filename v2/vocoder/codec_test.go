package vocoder_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type fakeCodec struct{ fid uint8 }

func (fakeCodec) FrameSamples() int                   { return 160 }
func (fakeCodec) Encode([]int16) ([72]bit.Bit, error) { return [72]bit.Bit{}, nil }
func (fakeCodec) Decode([72]bit.Bit) ([]int16, error) { return make([]int16, 160), nil }
func (fakeCodec) Silence() [72]bit.Bit                { return [72]bit.Bit{} }
func (c fakeCodec) FID() uint8                        { return c.fid }

func mustPanic(t *testing.T, name string, f func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: expected panic", name)
		}
	}()
	f()
}

func TestCodecRegistry(t *testing.T) {
	factory := func() (vocoder.Codec, error) { return fakeCodec{fid: 0x7F}, nil }
	vocoder.Register("test-fake-b", factory)
	vocoder.Register("test-fake-a", factory)

	names := vocoder.Names()
	if !slices.IsSorted(names) {
		t.Errorf("Names not sorted: %v", names)
	}
	for _, want := range []string{"test-fake-a", "test-fake-b"} {
		if !slices.Contains(names, want) {
			t.Errorf("Names missing %q: %v", want, names)
		}
	}

	c, err := vocoder.New("test-fake-a")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.FID() != 0x7F {
		t.Errorf("FID = %#x, want 0x7f", c.FID())
	}

	if _, err := vocoder.New("test-missing"); !errors.Is(err, vocoder.ErrUnknownCodec) {
		t.Errorf("New(missing) error = %v, want ErrUnknownCodec", err)
	}

	mustPanic(t, "duplicate", func() { vocoder.Register("test-fake-a", factory) })
	mustPanic(t, "nil factory", func() { vocoder.Register("test-fake-nil", nil) })
}

func TestCodecRegistryFactoryError(t *testing.T) {
	want := errors.New("factory failed")
	vocoder.Register("test-fake-err", func() (vocoder.Codec, error) { return nil, want })
	if _, err := vocoder.New("test-fake-err"); !errors.Is(err, want) {
		t.Errorf("New error = %v, want %v", err, want)
	}
}
