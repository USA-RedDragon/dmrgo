package rf

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestEmbOf checks embOf against RawBurst.EMB on random bursts.
func TestEmbOf(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	for range 20000 {
		var d layer2.RawBurst
		for i := range d {
			d[i] = byte(rng.Uint32())
		}
		if got, want := embOf(&d), d.EMB(); got != want {
			t.Fatalf("embOf of %x: %+v, EMB: %+v", d, got, want)
		}
	}
}
