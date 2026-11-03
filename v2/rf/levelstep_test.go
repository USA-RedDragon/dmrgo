//go:build !tinygo

package rf

import (
	"fmt"
	"math"
	"testing"
)

func TestRxLevelStep(t *testing.T) {
	t.Parallel()
	for _, db := range []float64{-10, 10, 20} {
		for _, off := range []string{"+0", "-2500"} {
			row := mxRowOf(t, "s0.none."+off+".bs.data12.0ppm.nom.a24000.inf")
			for _, ms := range []float64{0, 2, 3, 8, 12} {
				for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
					t.Run(fmt.Sprintf("%+.0fdB/%s/%gms/t%d", db, off, ms, tier-Tier0), func(t *testing.T) {
						t.Parallel()
						s, err := mxBuild(row)
						if err != nil {
							t.Fatal(err)
						}
						const j = 4
						s.step = s.truth[j].at + ms/1000
						s.gain = [2]float64{1, math.Pow(10, db/20)}
						if db > 0 {
							s.gain = [2]float64{math.Pow(10, -db/20), 1}
						}
						from := j + 2
						if db > 0 && tier >= Tier2 {
							from = j + 4
						}
						if db > 10 {
							from++
						}
						in := mxInputOf(s, row)
						got, dev, err := mxReceive(in, mxRx{"a24000", tier})
						if err != nil {
							t.Fatal(err)
						}
						ok := make([]bool, len(s.truth))
						for i := range got {
							b := &got[i]
							if k := mxNearest(s, b.Seconds+in.delay); k == j && b.CACH != s.truth[k].cach {
								t.Errorf("CACH of the burst with the %+.0f dB step: %d bits wrong", db, hamming(b.CACH[:], s.truth[k].cach[:]))
							}
						}
						for i := range got {
							b := &got[i]
							if k := mxNearest(s, b.Seconds+in.delay); k >= 0 && b.Data == s.truth[k].data {
								ok[k] = true
							}
						}
						for k := from; k < len(s.truth); k++ {
							if !ok[k] {
								t.Errorf("burst %d of %d after a %+.0f dB step at burst %d: not decoded exactly", k, len(s.truth), db, j)
							}
						}
						res := &mxResult{}
						last := -1
						for k, v := range ok {
							if v {
								last = k
							}
						}
						mxDeviation(s, in, tier, dev, ok, last, res)
						if len(res.detail) > 0 {
							t.Errorf("level after a %+.0f dB step: %v", db, res.detail)
						}
					})
				}
			}
		}
	}
}
