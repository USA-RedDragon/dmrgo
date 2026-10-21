package fsktx

import (
	"math"
	"testing"
)

func TestBandPassTopFromHalfRate(t *testing.T) {
	t.Parallel()
	for _, hz := range []float64{8000, 9600, 12000, 24000} {
		want := bandPass(optLPHz/4, 0, hz, 41)
		for _, hi := range []float64{hz / 2, hz/2 + 1, 12000, 20000} {
			if hi < hz/2 {
				continue
			}
			got := bandPass(optLPHz/4, hi, hz, 41)
			for i := range want {
				if math.Abs(got[i]-want[i]) > 1e-15 {
					t.Fatalf("%v Hz, top %v Hz: tap %d is %v, not %v", hz, hi, i, got[i], want[i])
				}
			}
			var re, im float64
			for i, v := range got {
				w := 2 * math.Pi * 0.45 * float64(i)
				re += v * math.Cos(w)
				im += v * math.Sin(w)
			}
			if g := math.Hypot(re, im); math.Abs(g-1) > 0.01 {
				t.Fatalf("%v Hz, top %v Hz: gain %v at 0.45 of the rate", hz, hi, g)
			}
		}
	}
}
