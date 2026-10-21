package fsktx

import (
	"math"
	"testing"
)

func TestInterpChainImages(t *testing.T) {
	t.Parallel()
	for _, rate := range []float64{48000, 96000, 192000, 1e6, 2.4e6} {
		step := float64(iqShaperHz / gcd(int(rate), iqShaperHz))
		for _, f := range []float64{300, 1200, 2400, 2880} {
			var sum float64
			for k := -100000 * step; k <= 100000*step; k += step {
				s := sinc((f - k*rate) / iqShaperHz)
				sum += s * s
			}
			want := sum / sinc(f/rate)
			if got := interpChain(iqShaperHz, rate)(f); math.Abs(got-want) > 1e-7 {
				t.Errorf("rate %g f %g: %.9f, images %.9f", rate, f, got, want)
			}
		}
	}
}
