//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func TestLockSamplePhase(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"s0.none.+0.ms.data1.0ppm.nom.cs16-48k.inf",
		"s0.none.-700.ms.data12.0ppm.nom.cs16-48k.inf",
		"gap.cw500ms.-3000.ms.data34.0ppm.nom.cs16-48k.inf",
		"s0.none.+0.bs.data1.0ppm.nom.cs16-48k.inf",
	} {
		row := mxRowOf(t, name)
		s, err := mxBuild(t, row)
		if err != nil {
			t.Fatal(err)
		}
		in := mxInputOf(s, row)
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			lo, hi := math.Inf(1), math.Inf(-1)
			for k := range 4 {
				x := append(make([]byte, in.iqFmt.bytes()*k), in.iq...)
				r := mustNew(RxConfig{Rate: in.rate, Tier: tier, Format: in.iqFmt})
				d := math.NaN()
				for _, b := range r.AppendFlush(r.AppendBursts(nil, x)) {
					at := b.Seconds + in.delay - float64(k)/float64(in.rate)
					if j := mxNearest(s, at); j >= 0 {
						d = (at - s.truth[j].at) * 1e6
						break
					}
				}
				lo, hi = min(lo, d), max(hi, d)
			}
			if math.IsNaN(lo) || hi-lo > mxSecondsMean*1e6 || max(-lo, hi) > mxSeconds*1e6 {
				t.Errorf("%s Tier%d: first burst %+.2f to %+.2f us over the four 12 kHz sample phases", name, tier-Tier0, lo, hi)
			}
		}
	}
}
