package fsktx

import (
	"math"
	"slices"
	"testing"
)

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

func TestNyquistTables(t *testing.T) {
	t.Parallel()
	var cases []pulse
	for _, p := range tierPulses() {
		cases = append(cases, p)
		q := p
		q.ridge = 2e-6
		cases = append(cases, q)
		q = p
		q.flatGain = 0.001
		q.flat = true
		cases = append(cases, q)
		if p.tab != nil {
			q = p
			q.tab = slices.Clone(p.tab)
			q.tab[len(q.tab)/2] += 1e-3
			cases = append(cases, q)
			q = p
			q.edge = 0.5
			cases = append(cases, q)
		} else {
			q = p
			q.beta++
			cases = append(cases, q)
		}
	}
	for i, p := range cases {
		want := p
		want.solveNyquist()
		p.nyquist()
		if *p.echo != *want.echo {
			t.Errorf("case %d (span %d, tab %v, flat %v): the echoes of the design differ from a solve", i, p.span, p.tab != nil, p.flat)
		}
	}
}
