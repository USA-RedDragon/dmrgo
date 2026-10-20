package rf

import "testing"

func receiveDisc(t testing.TB, a []int16, hz int, tier Tier) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: hz, Tier: tier, OnBurst: func(b *Burst) { out = append(out, *b) }})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		must(r.Write(a[k:min(k+1024, len(a))]))
	}
	r.Flush()
	return out
}

func TestDiscTier2Loopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 1)
	for _, hz := range []int{24000, 12000, 9600} {
		sc := scoreBench(truth, receiveDisc(t, discAudio(x, hz), hz, Tier2))
		for _, k := range []string{"ambe", "csbk", "rate12", "rate34", "elc", "sync", "emb", "slottype"} {
			miss := 0
			if k == "sync" || k == "slottype" {
				miss = 1
			}
			if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth-miss {
				t.Errorf("%d Hz, %s: %d of %d recovered at 30 dB", hz, k, s.Exact, s.Truth)
			}
		}
	}
}

func TestDiscTier2Weak(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	x := noisyBaseband(slots, 1400, 3, 5)
	for _, hz := range []int{24000, 12000, 9600} {
		a := discAudio(x, hz)
		rate := func(tier Tier, k string) float64 {
			s := scoreBench(truth, receiveDisc(t, a, hz, tier))[k]
			return float64(s.Exact) / float64(max(s.Truth, 1))
		}
		for _, c := range []struct {
			k    string
			gain float64
		}{{"ambe_c0c1", 0.15}, {"rate12", 0.5}} {
			t0, t2 := rate(Tier0, c.k), rate(Tier2, c.k)
			t.Logf("%d Hz, %s: %.2f Tier0, %.2f Tier2", hz, c.k, t0, t2)
			if t2 < 0.8 || t2 < t0+c.gain {
				t.Errorf("%d Hz, %s: %.2f Tier2, %.2f Tier0", hz, c.k, t2, t0)
			}
		}
	}
}
