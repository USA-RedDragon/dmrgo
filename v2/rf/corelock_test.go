//go:build !tinygo

package rf

import (
	"math"
	"slices"
	"sync"
	"testing"
)

type lockErr struct {
	hz, sym float64
	seed    uint64
}

func lockErrors(t *testing.T, cn float64, newRx func() *Receiver) []lockErr {
	slots, truth := benchStream(t, 6)
	sig := cleanBaseband(slots, 1400)
	start := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	var mu sync.Mutex
	var out []lockErr
	var wg sync.WaitGroup
	for seed := uint64(7000); seed < 7048; seed++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			x := sig.noisy(cn, seed)
			r := newRx()
			h := r.host().iqd
			var hz []float64
			seen := int32(0)
			var got []Burst
			for k := 0; k < len(x); k += 256 {
				got = append(got, must(r.writeBB(x[k:min(k+256, len(x))]))...)
				if h.d.streams != seen {
					seen = h.d.streams
					f, _ := h.cohTrack()
					hz = append(hz, f)
				}
			}
			got = append(got, r.flushAll()...)
			errs := map[int][]float64{}
			for _, b := range got {
				i := math.Round((b.Seconds - start) / slotSeconds)
				if i >= 0 && int(i) < len(truth) {
					errs[b.Stream] = append(errs[b.Stream], (b.Seconds-start-i*slotSeconds)*symbolRate)
				}
			}
			mu.Lock()
			defer mu.Unlock()
			for k, f := range hz {
				e := errs[k+1]
				sym := 0.0
				if len(e) > 1 {
					sorted := slices.Clone(e[1:])
					slices.Sort(sorted)
					sym = e[0] - sorted[len(sorted)/2]
				}
				out = append(out, lockErr{f - 1400, sym, seed})
			}
		}()
	}
	wg.Wait()
	return out
}

func TestTier2LockError(t *testing.T) {
	t.Parallel()
	for _, rx := range []struct {
		name string
		new  func() *Receiver
	}{
		{"Tier 2", func() *Receiver { return mustNew(RxConfig{Rate: basebandRate, Tier: Tier2, Format: IQFormatCU8}) }},
		{"core Tier 3", func() *Receiver {
			return mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8}, coreIQ3Options())
		}},
	} {
		for _, cn := range []float64{0, 1, 2} {
			for _, e := range lockErrors(t, cn, rx.new) {
				if math.Abs(e.hz) > cohFreqSpan {
					t.Errorf("%s at %v dB seed %d: lock %.0f Hz off", rx.name, cn, e.seed, e.hz)
				}
			}
		}
	}
}
