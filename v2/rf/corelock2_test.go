//go:build !tinygo

package rf

import (
	"sync"
	"testing"
)

func firstLockSlots(t *testing.T, cn float64, seeds []uint64, newRx func() *Receiver) []float64 {
	slots, _ := benchStream(t, 6)
	sig := cleanBaseband(slots, 1400)
	start := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	out := make([]float64, len(seeds))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, seed := range seeds {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			r := newRx()
			h := r.host().iqd
			x := sig.noisy(cn, seed)
			out[i] = -1
			for k := 0; k < len(x); k += 256 {
				must(r.writeBB(x[k:min(k+256, len(x))]))
				if h.d.streams > 0 {
					out[i] = (float64(k)/basebandRate - start) / slotSeconds
					break
				}
			}
		}()
	}
	wg.Wait()
	return out
}

func TestCoreFirstLock(t *testing.T) {
	t.Parallel()
	seeds := make([]uint64, 48)
	for i := range seeds {
		seeds[i] = 7000 + uint64(i)
	}
	for _, rx := range []struct {
		name string
		new  func() *Receiver
	}{
		{"Tier 2", func() *Receiver { return mustNew(RxConfig{Rate: basebandRate, Tier: Tier2, Format: IQFormatCU8}) }},
		{"core Tier 3", func() *Receiver {
			return mustNewOpt(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8}, coreIQ3Options())
		}},
	} {
		for i, s := range firstLockSlots(t, 0, seeds, rx.new) {
			if s < 0 || s >= 17 {
				t.Errorf("%s at 0 dB seed %d: first lock at slot %.1f, want within slot 16", rx.name, seeds[i], s)
			}
		}
	}
}
