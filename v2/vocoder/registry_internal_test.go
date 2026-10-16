package vocoder

import (
	"errors"
	"fmt"
	"testing"
)

func TestRegistryFull(t *testing.T) {
	t.Parallel()

	var r registry
	factory := func() (Codec, error) { return nil, errors.New("unused") }
	for i := range MaxCodecs {
		if err := r.register(fmt.Sprint("c", i), factory); err != nil {
			t.Fatalf("register %d: %v", i, err)
		}
	}
	if err := r.register("one-more", factory); !errors.Is(err, ErrRegistryFull) {
		t.Errorf("register beyond MaxCodecs: %v, want ErrRegistryFull", err)
	}
	if err := r.register("c0", factory); !errors.Is(err, ErrDuplicateCodec) {
		t.Errorf("register duplicate in a full registry: %v, want ErrDuplicateCodec", err)
	}
	if got := r.appendNames(nil); len(got) != MaxCodecs || got[0] != "c0" {
		t.Errorf("appendNames = %v", got)
	}
}
