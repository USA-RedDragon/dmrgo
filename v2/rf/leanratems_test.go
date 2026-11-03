//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

// TestLeanClockRateNullMS checks that a Tier 2 MS receiver learns no clock
// rate on a 0 ppm signal: a re-entry refit moves the timing by more than the
// loop's own corrections do, so its shift must count in the spread of the
// rate estimate as well as in its sum. The seeds are the ones of 768 that
// learned a false rate before. It fails when a stream applies a rate over
// the bound of TestLeanClockRateNull.
func TestLeanClockRateNullMS(t *testing.T) {
	t.Parallel()
	d := directShared(t)
	const sigmaS2 = 0.0062
	a := 1 - leanClockGain*slotSymbols
	bound := 5 * leanClockGain * math.Sqrt(sigmaS2/(1-a*a)) / leanMMSPS
	for _, c := range []struct {
		cn   float64
		seed uint64
	}{{0, 20012}, {2, 20079}, {8, 20108}} {
		r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier2, Format: IQFormatCU8})
		dd := r.host().iqd.d
		x := noisyFM(d.fm, d.env, 1400, c.cn, c.seed)
		worst := 0.0
		for k := 0; k < len(x); k += 2048 {
			must(r.writeBB(x[k:min(k+2048, len(x))]))
			if v := math.Abs(float64(dd.tm.rate / (2 * dd.tm.step))); dd.locked {
				worst = max(worst, v)
			}
		}
		if worst > bound {
			t.Errorf("%v dB seed %d: clock rate %.1e, bound %.1e, true rate 0", c.cn, c.seed, worst, bound)
		}
	}
}
