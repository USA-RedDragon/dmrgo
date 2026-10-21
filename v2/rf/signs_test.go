package rf

import (
	"math/bits"
	"math/rand/v2"
	"testing"
)

func TestSignsMatch(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(5, 6))
	match := func(r *leanCore, s uint32) (bool, int, bool) {
		for i := range int(r.sources) {
			m := bits.OnesCount32((s ^ syncSigns[i]) & (1<<syncSymbols - 1))
			switch {
			case m <= int(r.signMiss):
				return true, i, false
			case m >= syncSymbols-int(r.signMiss):
				return true, i, true
			}
		}
		return false, 0, false
	}
	for n := 0; n < 1<<18; n++ {
		r := &leanCore{sources: uint8(rng.IntN(numSources + 1)), signMiss: uint8(rng.IntN(8))}
		var a, b uint32
		for _, s := range []*uint32{&a, &b} {
			*s = rng.Uint32() & (1<<syncSymbols - 1)
			if r.sources > 0 && rng.IntN(2) == 0 {
				*s = syncSigns[rng.IntN(int(r.sources))]
				if rng.IntN(2) == 0 {
					*s = ^*s & (1<<syncSymbols - 1)
				}
				for range rng.IntN(9) {
					*s ^= 1 << rng.IntN(syncSymbols)
				}
			}
		}
		ma, mb := r.signsMatch2(a, b)
		wa, ia, da := match(r, a)
		wb, _, _ := match(r, b)
		if ma != wa || mb != wb {
			t.Fatalf("signsMatch(%06x, %06x) with %d sources, miss %d = %v, %v, want %v, %v", a, b, r.sources, r.signMiss, ma, mb, wa, wb)
		}
		if i, d := r.signsSource(a); wa && (i != ia || d != da) {
			t.Fatalf("signsSource(%06x) = %d, %v, want %d, %v", a, i, d, ia, da)
		}
	}
}
