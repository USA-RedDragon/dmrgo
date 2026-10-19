package fsktx

import (
	"math"
	"testing"
)

// TestNyquistPulse checks that nyquist leaves a pulse that, after the
// receive filter of nyquist, has the gain of a run of one symbol at its
// centre and little intersymbol interference, where the truncated pulse
// has neither.
func TestNyquistPulse(t *testing.T) {
	t.Parallel()
	for _, span := range []int{4, 6, 8, 16} {
		p := pulse{span: span}
		g0, i0 := nyquistResponse(p)
		p.nyquist()
		g, isi := nyquistResponse(p)
		if math.Abs(g) > 2e-4 || isi > i0/4 {
			t.Errorf("span %d: gain error %.2e (truncated %.2e), ISI %.2e (truncated %.2e)", span, g, g0, isi, i0)
		}
	}
}

// nyquistResponse returns the gain error at the centre of p after the
// receive filter of nyquist, over the area of p, and the sum of squares of
// the response at the other symbol times.
func nyquistResponse(p pulse) (gain, isi float64) {
	const g = 128
	var area float64
	for j := -nyquistRxSpan * g; j < nyquistRxSpan*g; j++ {
		area += rrc((float64(j)+0.5)/g) / g
	}
	var dc float64
	for j := -p.span * g; j < p.span*g; j++ {
		dc += p.base((float64(j)+0.5)/g) / g
	}
	for m := -3 * p.span; m <= 3*p.span; m++ {
		var v float64
		for j := -p.span * g; j < p.span*g; j++ {
			x := (float64(j) + 0.5) / g
			if r := float64(m) - x; math.Abs(r) < nyquistRxSpan {
				v += p.base(x) * rrc(r) / area / g
			}
		}
		v /= dc
		if m == 0 {
			gain = v - 1
		} else {
			isi += v * v
		}
	}
	return gain, isi
}
