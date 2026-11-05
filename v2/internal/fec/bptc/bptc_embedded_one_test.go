package bptc_test

import (
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

func TestDecodeEmbeddedOne(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(11, 13))
	for range 64 {
		var info [10]byte
		for i := range info {
			info[i] = byte(rng.Uint32())
		}
		info[9] &= 0xF8
		f := bptc.EncodeEmbedded(info)
		if got, res := bptc.DecodeEmbeddedOne(f); res.Uncorrectable || res.ErrorsCorrected != 0 || got != info {
			t.Fatalf("clean: %x %+v, want %x", got, res, info)
		}
		for p := range 128 {
			g := f
			g[p/32] ^= 1 << (31 - p%32)
			if got, res := bptc.DecodeEmbeddedOne(g); res.Uncorrectable || res.ErrorsCorrected != 1 || got != info {
				t.Fatalf("error at %d: %x %+v, want %x", p, got, res, info)
			}
			q := rng.IntN(127)
			if q >= p {
				q++
			}
			g[q/32] ^= 1 << (31 - q%32)
			if _, res := bptc.DecodeEmbeddedOne(g); !res.Uncorrectable {
				t.Fatalf("errors at %d and %d: %+v", p, q, res)
			}
		}
	}
}
