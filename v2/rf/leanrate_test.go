//go:build !tinygo

package rf

import (
	"math"
	"sync"
	"testing"
)

func maxLockedRate(t *testing.T, tr Tier, cn float64, seeds []uint64) []float64 {
	slots, _ := benchStream(t, 60)
	sig := cleanBaseband(slots, 1400)
	out := make([]float64, len(seeds))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, seed := range seeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := mustNew(RxConfig{Rate: basebandRate, Tier: tr, Format: IQFormatCU8})
			d := r.host().iqd.d
			x := sig.noisy(cn, seed)
			for k := 0; k < len(x); k += 2048 {
				must(r.writeBB(x[k:min(k+2048, len(x))]))
				if v := math.Abs(float64(d.tm.rate / (2 * d.tm.step))); d.locked && v > out[i] {
					out[i] = v
				}
			}
		}()
	}
	wg.Wait()
	return out
}

func TestLeanClockRateNull(t *testing.T) {
	t.Parallel()
	const sigmaS2 = 0.0062
	a := 1 - leanClockGain*slotSymbols
	sigmaRate := leanClockGain * math.Sqrt(sigmaS2/(1-a*a)) / leanMMSPS
	bound := 5 * sigmaRate
	seeds := make([]uint64, 192)
	for i := range seeds {
		seeds[i] = 11000 + uint64(i)
	}
	for _, tr := range []Tier{Tier0, Tier2} {
		for _, cn := range []float64{0, 1, 2} {
			bad := 0
			worst := 0.0
			for _, v := range maxLockedRate(t, tr, cn, seeds) {
				if v > bound {
					bad++
				}
				worst = max(worst, v)
			}
			if bad > 0 {
				t.Errorf("%v at %v dB: %d of %d streams with a clock rate over %.1e (worst %.1e), true rate 0", tr, cn, bad, len(seeds), bound, worst)
			}
		}
	}
}
