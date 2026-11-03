//go:build !tinygo

package rf

import (
	"testing"
)

func TestRxNoBurstPastInputEnd(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	pad := int(benchPad * basebandRate)
	slack := float64(discSpan) / symbolRate
	for _, e := range []benchEntry{{benchIQ, Tier0}, {benchIQ, Tier1}, {benchIQ, Tier2}, {benchIQ, Tier3}, {benchAudio, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier2}, {benchAudio, Tier3}, {benchIQLow, Tier0}} {
		t.Run(e.name(), func(t *testing.T) {
			t.Parallel()
			for seed := uint64(1); seed <= 2; seed++ {
				for _, cut := range []int{0, slotSamplesN / 4, slotSamplesN / 2} {
					x := noisyBaseband(slots[:40], 1400, 20, seed)
					x = x[:len(x)-pad-cut]
					end := float64(len(x)) / basebandRate
					last := false
					for _, b := range benchReceive(t, x, e) {
						if b.Seconds+burstSeconds > end+slack {
							t.Errorf("seed %d, input cut %d samples into the last burst: burst at %.4f s runs %.1f symbols past the input end", seed, cut, b.Seconds, (b.Seconds+burstSeconds-end)*symbolRate)
						}
						last = last || b.Seconds+burstSeconds > end-slotSeconds
					}
					if cut == 0 && !last {
						t.Errorf("seed %d: no burst ends in the last slot of an input that stops at a burst end", seed)
					}
				}
			}
		})
	}
}
