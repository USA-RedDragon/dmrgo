package rf

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func unionReceive(t testing.TB, a []int16, tier Tier) ([]Burst, *discUnion) {
	t.Helper()
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: tier, OnBurst: func(b *Burst) { out = append(out, *b) }})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 4096 {
		must(r.Write(a[k:min(k+4096, len(a))]))
	}
	r.Flush()
	e, _ := r.ext.(*softExt)
	if e == nil {
		return out, nil
	}
	return out, e.union
}

func unionSlotOf(truth []benchSlot, b Burst, ms bool) int {
	first := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	s := b.Seconds
	if ms {
		s += float64(cachSymbols) / symbolRate
	}
	i := int(math.Round((s - first) / slotSeconds))
	if i >= 0 && i < len(truth) && math.Abs(s-first-float64(i)*slotSeconds) < 0.002 {
		return i
	}
	return -1
}

func unionDataOK(ts benchSlot, data layer2.RawBurst) bool {
	if ts.kind != "data" || data.SlotType() != ts.burst.SlotType() {
		return false
	}
	switch ts.dt {
	case enums.DataTypeRate34:
		a, _ := data.Rate34Payload()
		b, _ := ts.burst.Rate34Payload()
		return a == b
	case enums.DataTypeRate1:
		a, _ := data.Rate1Payload()
		b, _ := ts.burst.Rate1Payload()
		return a == b
	}
	a, _ := data.BPTCPayload()
	b, _ := ts.burst.BPTCPayload()
	return a == b
}

func unionSuperset(truth []benchSlot, ms bool, t2, t3 []Burst) (int, error) {
	j, swaps := 0, 0
	for i := range t3 {
		if j < len(t2) && t3[i] == t2[j] {
			j++
			continue
		}
		if j < len(t2) {
			a, b := t2[j], t3[i]
			b.Fixed, b.FixedCACH, b.HasFixed = a.Fixed, a.FixedCACH, a.HasFixed
			if a == b {
				k := unionSlotOf(truth, a, ms)
				if k < 0 {
					return swaps, fmt.Errorf("Tier3 changed Tier2 burst %d at %.4f s, which is in no slot", j, a.Seconds)
				}
				d, _ := t2[j].Decoded()
				if unionDataOK(truth[k], d) {
					return swaps, fmt.Errorf("Tier3 changed Tier2 burst %d at %.4f s, which was right", j, a.Seconds)
				}
				swaps++
				j++
				continue
			}
		}
		for k := range t2 {
			if math.Abs(t2[k].Seconds-t3[i].Seconds) < unionSlot {
				return swaps, fmt.Errorf("Tier3 burst %d at %.4f s is near Tier2 burst %d at %.4f s", i, t3[i].Seconds, k, t2[k].Seconds)
			}
		}
	}
	if j != len(t2) {
		return swaps, fmt.Errorf("Tier3 keeps %d of %d Tier2 bursts in order", j, len(t2))
	}
	return swaps, nil
}

func unionScore(truth []benchSlot, ms bool, got []Burst) map[string]*benchScore {
	c := append([]Burst(nil), got...)
	if ms {
		return scoreMS(truth, c)
	}
	return scoreBench(truth, c)
}

func TestDiscTier3Superset(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("receives many streams with audio Tier2 and Tier3")
	}
	seeds := 4
	if s, err := strconv.Atoi(os.Getenv("DMRGO_UNION_SEEDS")); err == nil {
		seeds = s
	}
	slots, bsTruth := benchStream(t, 6)
	fm, env, msTruth := msStream(t, 3, 0.37)
	type tcase struct {
		name  string
		truth []benchSlot
		ms    bool
		x     func() []complex64
	}
	var cases []tcase
	for _, cn := range []float64{-1, 0, 1, 2, 3, 4, 5, 6, 8, 12, 30} {
		for s := range uint64(seeds) {
			seed := 100 + s
			cases = append(cases,
				tcase{fmt.Sprintf("bs cn=%g seed=%d", cn, seed), bsTruth, false, func() []complex64 { return noisyBaseband(slots, 1400, cn, seed) }},
				tcase{fmt.Sprintf("ms cn=%g seed=%d", cn, seed), msTruth, true, func() []complex64 { return noisyFM(fm, env, 1400, cn, seed) }},
				tcase{fmt.Sprintf("ms impaired cn=%g seed=%d", cn, seed), msTruth, true, func() []complex64 { return impairedFM(fm, env, -900, 0.1, 80, cn+4, seed) }},
			)
		}
	}
	var mu sync.Mutex
	var kept, filled, swaps, late int
	var maxLag float64
	gain := map[string]int{}
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range cases {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			a := discAudio(c.x(), benchAudioHz)
			t2, _ := unionReceive(t, a, Tier2)
			t3, u := unionReceive(t, a, Tier3)
			if u == nil {
				t.Errorf("%s: Tier3 has no union", c.name)
				return
			}
			n, err := unionSuperset(c.truth, c.ms, t2, t3)
			if err != nil {
				t.Errorf("%s: %v", c.name, err)
			}
			s2, s3 := unionScore(c.truth, c.ms, t2), unionScore(c.truth, c.ms, t3)
			mu.Lock()
			defer mu.Unlock()
			for k, v := range s2 {
				if s3[k].Exact < v.Exact {
					t.Errorf("%s: Tier3 has %d of %d %s, Tier2 %d", c.name, s3[k].Exact, v.Truth, k, v.Exact)
				}
				gain[k] += s3[k].Exact - v.Exact
			}
			kept += len(t2)
			filled += len(t3) - len(t2)
			swaps += n
			late += u.late
			maxLag = max(maxLag, u.maxLag)
		}()
	}
	wg.Wait()
	if late != 0 {
		t.Errorf("%d Tier2 bursts came after the union decided their time", late)
	}
	t.Logf("%d cases: Tier2 %d bursts; Tier3 adds %d and replaces %d proven wrong; Tier2 latency at most %.3f s (horizon %.1f s); gains %v", len(cases), kept, filled, swaps, maxLag, unionHorizon, gain)
}
