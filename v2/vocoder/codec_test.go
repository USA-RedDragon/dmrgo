package vocoder_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type fakeCodec struct{ fid enums.FeatureSetID }

func (fakeCodec) FrameSamples() int                   { return 160 }
func (fakeCodec) Encode([]int16) ([72]bit.Bit, error) { return [72]bit.Bit{}, nil }
func (fakeCodec) Decode([72]bit.Bit) ([]int16, error) { return make([]int16, 160), nil }
func (fakeCodec) Silence() [72]bit.Bit                { return [72]bit.Bit{} }
func (c fakeCodec) FID() enums.FeatureSetID           { return c.fid }
func (fakeCodec) Close() error                        { return nil }

// uniqueName returns a codec name not yet registered, so tests stay
// repeatable under -count because the registry is process-global.
func uniqueName(base string) string {
	names := vocoder.Names()
	for i := 0; ; i++ {
		if name := fmt.Sprintf("%s-%d", base, i); !slices.Contains(names, name) {
			return name
		}
	}
}

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
	t.Parallel()

	factory := func() (vocoder.Codec, error) { return fakeCodec{fid: 0x7F}, nil }
	a, b := uniqueName("test-fake-a"), uniqueName("test-fake-b")
	vocoder.Register(b, factory)
	vocoder.Register(a, factory)

	names := vocoder.Names()
	if !slices.IsSorted(names) {
		t.Errorf("Names not sorted: %v", names)
	}
	for _, want := range []string{a, b} {
		if !slices.Contains(names, want) {
			t.Errorf("Names missing %q: %v", want, names)
		}
	}

	prefixed := vocoder.AppendNames([]string{"~first"})
	if prefixed[0] != "~first" || !slices.IsSorted(prefixed[1:]) || !slices.Contains(prefixed, a) || !slices.Contains(prefixed, b) {
		t.Errorf("AppendNames = %v", prefixed)
	}

	c, err := vocoder.New(a)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.FID() != 0x7F {
		t.Errorf("FID = %#x, want 0x7f", c.FID())
	}

	if _, err := vocoder.New("test-missing"); !errors.Is(err, vocoder.ErrUnknownCodec) {
		t.Errorf("New(missing) error = %v, want ErrUnknownCodec", err)
	}

	mustPanic(t, "duplicate", func() { vocoder.Register(a, factory) })
	mustPanic(t, "nil factory", func() { vocoder.Register("test-fake-nil", nil) })
}

func TestCodecRegistryFactoryError(t *testing.T) {
	t.Parallel()

	want := errors.New("factory failed")
	name := uniqueName("test-fake-err")
	vocoder.Register(name, func() (vocoder.Codec, error) { return nil, want })
	if _, err := vocoder.New(name); !errors.Is(err, want) {
		t.Errorf("New error = %v, want %v", err, want)
	}
}
