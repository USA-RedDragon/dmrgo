//go:build !tinygo

package rf

import (
	"sync"
	"testing"
)

// TestTier2FadeUncoded receives an MS stream through 50 Hz Rayleigh fading
// at 24 dB C/N, on a payload and noise where Tier 2 took the 25 unprotected
// bits of AMBE frames from its coherent detection whenever the core held
// more errors in the protected bits of that frame. Those bits come from the
// coherent detection again with the protected bits as priors, and in fast
// fading it gets them wrong about 2.7 times as often as the core, so the
// raw protected errors of a frame do not tell which detection to take them
// from. Tier 2 must recover at least as many whole AMBE frames as Tier 0.
func TestTier2FadeUncoded(t *testing.T) {
	t.Parallel()
	tb := pinnedRun(t, 1254475477399117690, "TestTierImpaired")
	fm, env, truth := msStream(tb, 3, 0.37)
	var got [2]int
	var mu sync.Mutex
	forSeeds(8, func(i int) {
		x := impairedFM(tb, fm, env, 1400, 0, 50, 24, 100+uint64(i))
		for k, tier := range []Tier{Tier0, Tier2} {
			n := scoreMS(truth, receiveBaseband(x, tier))["ambe"].Exact
			mu.Lock()
			got[k] += n
			mu.Unlock()
		}
	})
	if got[1] < got[0] {
		t.Errorf("Tier2 recovers %d whole AMBE frames, Tier0 %d", got[1], got[0])
	}
}

// TestRate34CheckedCore receives an MS stream of confirmed rate 3/4 data
// through 50 Hz Rayleigh fading at 18 dB C/N, on a payload and noise where
// the soft decode of Tier 1 gave blocks whose CRC-9 failed while the hard
// decode of its own core gave them with a good CRC-9. Each block of a
// confirmed packet carries a CRC-9 (ETSI TS 102 361-1 clause 9.2.2, Table
// 9.11), so Tiers 1 and 2 must recover at least as many rate 3/4 blocks as
// Tier 0.
func TestRate34CheckedCore(t *testing.T) {
	t.Parallel()
	tb := pinnedRun(t, 14405766787730588285, "TestTierImpaired")
	fm, env, truth := msStream(tb, 3, 0.37)
	var got [3]int
	var mu sync.Mutex
	forSeeds(8, func(i int) {
		x := impairedFM(tb, fm, env, 1400, 0, 50, 18, 100+uint64(i))
		for k, tier := range []Tier{Tier0, Tier1, Tier2} {
			n := scoreMS(truth, receiveBaseband(x, tier))["rate34"].Exact
			mu.Lock()
			got[k] += n
			mu.Unlock()
		}
	})
	for k := 1; k < len(got); k++ {
		if got[k] < got[0] {
			t.Errorf("Tier%d recovers %d rate 3/4 blocks, Tier0 %d", k, got[k], got[0])
		}
	}
}
