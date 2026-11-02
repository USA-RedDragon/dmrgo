//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// TestRxRCReentry checks that an MS RC burst after a held slot is timed on
// its own sync: over the seeds, the time from the wakeup burst to the RC
// burst must match the transmitted 60 ms within mxSeconds, pairedZ standard
// errors allowed, at a clock error of +-500 ppm.
func TestRxRCReentry(t *testing.T) {
	t.Parallel()
	const seeds = 8
	cfg := testConfig()
	cfg.Wakeups = 1
	built, err := rftx.Sequence("rc", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	tx := rftx.ToIQ(append(make([]float64, pad), fm0...), append(make([]float64, pad), env0...), 2e6, 0, 0.9)
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		for _, ppm := range []float64{500, -500} {
			gap := make([]float64, seeds)
			forSeeds(seeds, func(i int) {
				iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, 1500, 30-22.1, uint64(i+1))
				r := mustNew(RxConfig{Rate: 2.048e6, Tier: tier, Format: IQFormatCU8})
				got := r.AppendFlush(r.AppendBursts(nil, iq))
				gap[i] = math.NaN()
				for k := 1; k < len(got); k++ {
					if got[k].Sync == enums.SyncMSSourcedRC {
						gap[i] = (got[k].Seconds - got[k-1].Seconds - 2*slotSeconds*(1+ppm*1e-6)) * 1e6
					}
				}
			})
			mean, se := meanSE(gap)
			t.Logf("Tier%d %+.0f ppm: RC burst %+.2f us, SE %.2f", tier-Tier0, ppm, mean, se)
			if !(math.Abs(mean)-pairedZ*se <= mxSeconds*1e6) {
				t.Errorf("Tier%d %+.0f ppm: the RC burst after a held slot is %+.2f us off the wakeup burst plus 60 ms, SE %.2f", tier-Tier0, ppm, mean, se)
			}
		}
	}
}
