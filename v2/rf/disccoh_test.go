package rf

import (
	"sync"
	"testing"
)

func receiveDisc(t testing.TB, a []int16, hz int, tier Tier) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) })})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		must(r.writeAll(a[k:min(k+1024, len(a))]))
	}
	r.flushAll()
	return out
}

func TestDiscTier2Loopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(t, slots, 1400, 30, 1)
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

const discWeakSeeds = 8

func TestDiscTier2Weak(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 10)
	type key struct {
		hz   int
		tier Tier
		k    string
	}
	exact, total := map[key]int{}, map[key]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for seed := uint64(1); seed <= discWeakSeeds; seed++ {
		x := noisyBaseband(t, slots, 1400, 3, seed)
		for _, hz := range []int{24000, 12000, 9600} {
			a := discAudio(x, hz)
			for _, tier := range []Tier{Tier0, Tier2} {
				wg.Add(1)
				go func() {
					defer wg.Done()
					sc := scoreBench(truth, receiveDisc(t, a, hz, tier))
					mu.Lock()
					defer mu.Unlock()
					for _, k := range []string{"ambe_c0c1", "rate12"} {
						exact[key{hz, tier, k}] += sc[k].Exact
						total[key{hz, tier, k}] += sc[k].Truth
					}
				}()
			}
		}
	}
	wg.Wait()
	rate := func(hz int, tier Tier, k string) float64 {
		return float64(exact[key{hz, tier, k}]) / float64(max(total[key{hz, tier, k}], 1))
	}
	for _, hz := range []int{24000, 12000, 9600} {
		for _, c := range []struct {
			k    string
			gain float64
		}{{"ambe_c0c1", 0.15}, {"rate12", 0.5}} {
			t0, t2 := rate(hz, Tier0, c.k), rate(hz, Tier2, c.k)
			t.Logf("%d Hz, %s: %.3f Tier0, %.3f Tier2 over %d seeds", hz, c.k, t0, t2, discWeakSeeds)
			if t2 < 0.8 || t2 < t0+c.gain {
				t.Errorf("%d Hz, %s: %.3f Tier2, %.3f Tier0 over %d seeds", hz, c.k, t2, t0, discWeakSeeds)
			}
		}
	}
}

func TestDiscTier2FalseLockBeforeStream(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 30)
	a := discAudio(cleanBaseband(slots, 1400).noisy(t, 12, 29363428), benchAudioHz)
	got := receiveDisc(t, a, benchAudioHz, Tier2)
	if len(got) == 0 || got[len(got)-1].Stream < 2 {
		t.Fatal("no false lock in the noise before the stream")
	}
	if got[0].Seconds < benchPad {
		t.Errorf("a burst at %.3f s from the noise before the stream", got[0].Seconds)
	}
	lost := func(b []Burst) int { s := scoreBench(truth, b)["ambe"]; return s.Truth - s.Exact }
	if n, n1 := lost(got), lost(receiveDisc(t, a, benchAudioHz, Tier1)); n > n1 {
		t.Errorf("%d voice frames lost after a false lock at 12 dB, %d with Tier1", n, n1)
	}
}
