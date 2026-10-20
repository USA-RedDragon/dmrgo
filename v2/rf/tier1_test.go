package rf

import "testing"

func TestTier1Weak(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	for _, c := range []struct {
		mode string
		cn   float64
	}{{benchIQ, 2}, {benchAudio, 3}} {
		rate := func(tier Tier, k string) float64 {
			var e, n int
			for seed := range uint64(2) {
				x := noisyBaseband(slots, 1400, c.cn, 5+seed)
				s := scoreBench(truth, benchReceive(t, x, benchEntry{c.mode, tier}))[k]
				e, n = e+s.Exact, n+s.Truth
			}
			return float64(e) / float64(max(n, 1))
		}
		for _, p := range []struct {
			k    string
			gain float64
		}{{"rate12", 0.3}, {"ambe_c0c1", 0.05}} {
			t0, t1 := rate(Tier0, p.k), rate(Tier1, p.k)
			t.Logf("%s at C/N %v dB, %s: %.2f Tier0, %.2f Tier1", c.mode, c.cn, p.k, t0, t1)
			if t1 < t0+p.gain {
				t.Errorf("%s at C/N %v dB, %s: %.2f Tier1, %.2f Tier0", c.mode, c.cn, p.k, t1, t0)
			}
		}
	}
}
