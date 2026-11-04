//go:build !tinygo

package rf

import (
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// benchDraw builds the stream of benchStream for payload draw d: fictional
// IDs and the text of benchMessage(d).
func benchDraw(t testing.TB, n int, d uint64) ([]rftx.BSSlot, []benchSlot) {
	t.Helper()
	return benchStreamOf(t, rftx.SeqConfig{Source: 3009101, BS: 990913, Message: benchMessage(d)}, 3009201, n)
}

func firstLockSlots(slots []rftx.BSSlot, cn float64, seeds []uint64, newRx func() *Receiver) []float64 {
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

// TestCoreFirstLock checks how soon Tiers 2 and 3 lock at 0 dB. The lock
// slot varies with the payload as much as with the noise, so the test takes
// 12 fixed payload draws with 4 noise seeds each. Over 32 draws of 48 seeds
// the first lock came at slot 7.82 (SD 2.81) at Tier 2 and 7.45 (SD 2.48)
// at Tier 3, after slot 16 in 2.0% and 0.46% of streams. It fails when a
// stream does not lock, when the mean slot is over the measured mean plus 3
// standard errors of 48 streams (9.0 and 8.5), so a regression of 2 slots in
// the mean fails 98% of the time, or when more streams than 4 and 2 lock
// after slot 16, which a correct receiver passes 99.7% and 99.9% of the time.
func TestCoreFirstLock(t *testing.T) {
	t.Parallel()
	const draws, seeds = 12, 4
	for _, rx := range []struct {
		tier Tier
		mean float64
		late int
	}{{Tier2, 9.0, 4}, {Tier3, 8.5, 2}} {
		var sum float64
		late := 0
		for d := range uint64(draws) {
			slots, _ := benchDraw(t, 6, d)
			ss := make([]uint64, seeds)
			for i := range ss {
				ss[i] = 7000 + d*seeds + uint64(i)
			}
			for i, s := range firstLockSlots(slots, 0, ss, func() *Receiver {
				return mustNew(RxConfig{Rate: basebandRate, Tier: rx.tier, Format: IQFormatCU8})
			}) {
				if s < 0 {
					t.Errorf("Tier%d at 0 dB draw %d seed %d: no lock", rx.tier-Tier0, d, ss[i])
				}
				sum += s
				if s >= 17 {
					late++
				}
			}
		}
		if mean := sum / (draws * seeds); mean > rx.mean || late > rx.late {
			t.Errorf("Tier%d at 0 dB: first lock at slot %.2f on average, want at most %.1f; %d of %d after slot 16, want at most %d", rx.tier-Tier0, mean, rx.mean, late, draws*seeds, rx.late)
		}
	}
}
