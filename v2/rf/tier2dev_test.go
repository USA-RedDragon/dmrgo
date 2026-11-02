package rf

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestTier2DeviationRange checks that Tier 2 decodes an MS data call whose
// deviation is 30% below or above nominal, from the first sample and after
// 0.1 s of silence: at least 95% of the bursts over the seeds come out exact,
// where Tiers 0, 1 and 3 decode all of them.
func TestTier2DeviationRange(t *testing.T) {
	t.Parallel()
	const seeds = 8
	built, err := rftx.Sequence("data12", testConfig())
	if err != nil {
		t.Fatal(err)
	}
	fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	for _, dev := range []float64{0.7, 1.3} {
		fm := make([]float64, len(fm0))
		for i := range fm0 {
			fm[i] = fm0[i] * dev
		}
		for _, pad := range []float64{0, 0.1} {
			n := int(pad * basebandRate)
			tx := rftx.ToIQ(append(make([]float64, n), fm...), append(make([]float64, n), env0...), 2e6, 0, 0.9)
			exact := make([]float64, seeds)
			forSeeds(seeds, func(i int) {
				iq := rftx.Channel(tx, 2e6, 2.048e6, 0, 25-22.1, uint64(i+1))
				r := mustNew(RxConfig{Rate: 2.048e6, Tier: Tier2, Format: IQFormatCU8})
				for _, g := range r.AppendFlush(r.AppendBursts(nil, iq)) {
					for _, w := range built.Bursts {
						c := 0
						for k := range w {
							c += bits.OnesCount8(g.Data[k] ^ w[k])
						}
						if c == 0 {
							exact[i]++
							break
						}
					}
				}
			})
			var sum float64
			for _, e := range exact {
				sum += e
			}
			frac := sum / float64(seeds*len(built.Bursts))
			if frac < 0.95 {
				t.Errorf("deviation %.1f, %.1f s silence: %.2f of the bursts exact at Tier 2, want 0.95", dev, pad, frac)
			}
		}
	}
}
