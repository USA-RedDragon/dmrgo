package rf

import (
	"math"
	"testing"
)

// TestHGateInterval checks that the estimate of options.HGate finds the
// peak of the anchor scores and that its interval is narrow only when the
// noise is low.
func TestHGateInterval(t *testing.T) {
	t.Parallel()
	g := newHGate(64, newSyncSearch(true, 1))
	rng := uint64(1)
	for _, c := range []struct {
		h, noise float64
		wide     bool
	}{{0.243, 0.001, false}, {0.27, 0.001, false}, {0.243, 1, true}} {
		for i := range 64 {
			for k := range hGateN {
				d := hGateLo + float64(k)*hGateStep - c.h
				rng ^= rng << 13
				rng ^= rng >> 7
				rng ^= rng << 17
				u := float64(rng%1000)/1000 - 0.5
				g.sc[i*hGateN+k] = float32(1 - 100*d*d + c.noise*u)
			}
		}
		est, lo, hi := g.interval(64)
		if !c.wide && math.Abs(est-c.h) > hGateStep {
			t.Errorf("h %v: estimate %v", c.h, est)
		}
		if w := hi - lo; (w > 2*hGateStep) != c.wide || lo > est || hi < est {
			t.Errorf("h %v noise %v: interval %v to %v around %v", c.h, c.noise, lo, hi, est)
		}
	}
}
