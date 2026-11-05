//go:build !tinygo

package rf

import (
	"slices"
	"strings"
	"testing"
)

func TestRxCACHCut(t *testing.T) {
	t.Parallel()
	for _, format := range []string{"a48000", "cs16-48k", "cu8-2048k"} {
		for _, cut := range []float64{0.0021, 0.0028} {
			row := mxRowOf(t, "s0.none.+0.bs.csbk.0ppm.nom."+format+".inf")
			s, err := mxBuild(t, row)
			if err != nil {
				t.Fatal(err)
			}
			for i := range s.parts {
				s.parts[i].t0 -= cut
			}
			for i := range s.truth {
				s.truth[i].at -= cut
				s.truth[i].hasCACH = s.truth[i].at*symbolRate >= cachSymbols
			}
			s.truth = slices.DeleteFunc(s.truth, func(tr mxTruth) bool { return tr.at < 0 })
			if s.truth[0].hasCACH {
				t.Fatalf("%s cut %v: the first CACH is whole", format, cut)
			}
			in := mxInputOf(s, row)
			for _, x := range mxReceivers(format, true) {
				got, dev, err := mxReceive(in, x)
				if err != nil {
					t.Fatal(err)
				}
				r := &mxResult{}
				mxScore(s, in, x.tier, got, dev, false, r)
				if r.fail["missing"] || r.fail["bits"] || r.fail["cach"] || r.fail["bs"] {
					t.Errorf("%s cut %v %s: %s: %s", format, cut, x.name(), r.symptoms(), strings.Join(r.detail, " | "))
				}
				for _, b := range got {
					if b.HasCACH && b.Seconds*symbolRate < cachSymbols {
						t.Errorf("%s cut %v %s: burst at %.4f s has a CACH that starts before the input", format, cut, x.name(), b.Seconds)
					}
				}
			}
		}
	}
}
