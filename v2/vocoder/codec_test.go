package vocoder_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type fakeCodec struct{ fid enums.FeatureSetID }

func (fakeCodec) FrameSamples() int               { return 160 }
func (fakeCodec) Encode([]int16) ([9]byte, error) { return [9]byte{}, nil }
func (fakeCodec) Decode([9]byte) ([]int16, error) { return make([]int16, 160), nil }
func (fakeCodec) Silence() [9]byte                { return [9]byte{} }
func (c fakeCodec) FID() enums.FeatureSetID       { return c.fid }
func (fakeCodec) Close() error                    { return nil }

var errFactory = errors.New("factory failed")

var registerTestCodecs = sync.OnceValue(func() error {
	factory := func() (vocoder.Codec, error) { return fakeCodec{fid: 0x7F}, nil }
	for _, name := range []string{"test-fake-b", "test-fake-a"} {
		if err := vocoder.Register(name, factory); err != nil {
			return err
		}
	}
	return vocoder.Register("test-fake-err", func() (vocoder.Codec, error) { return nil, errFactory })
})

func TestCodecRegistry(t *testing.T) {
	t.Parallel()

	if err := registerTestCodecs(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	a, b := "test-fake-a", "test-fake-b"
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

	factory := func() (vocoder.Codec, error) { return fakeCodec{}, nil }
	if err := vocoder.Register(a, factory); !errors.Is(err, vocoder.ErrDuplicateCodec) {
		t.Errorf("Register(duplicate) error = %v, want ErrDuplicateCodec", err)
	}
	if err := vocoder.Register("test-fake-nil", nil); !errors.Is(err, vocoder.ErrNilFactory) {
		t.Errorf("Register(nil) error = %v, want ErrNilFactory", err)
	}
}

func TestCodecRegistryFactoryError(t *testing.T) {
	t.Parallel()

	if err := registerTestCodecs(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := vocoder.New("test-fake-err"); !errors.Is(err, errFactory) {
		t.Errorf("New error = %v, want %v", err, errFactory)
	}
}
