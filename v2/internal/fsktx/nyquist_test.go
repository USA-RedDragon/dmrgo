package fsktx

import (
	"math"
	"runtime"
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
	tol := 1e-12
	if runtime.GOARCH == "amd64" {
		tol = 0
	}
	near := func(a, b float64) bool { return math.Abs(a-b) <= tol*(1+math.Abs(b)) }
	for i, p := range cases {
		want := p
		want.solveNyquist()
		p.nyquist()
		for k := range p.echo {
			if !near(p.echo[k], want.echo[k]) {
				t.Errorf("case %d (span %d, tab %v, flat %v): echo %d is %v, a solve gives %v", i, p.span, p.tab != nil, p.flat, k, p.echo[k], want.echo[k])
				break
			}
		}
	}
	p := tierPulses()[0]
	p.nyquist()
	p.inv = [2]float64{0.01, -0.002}
	if !p.iqBase(iqBaseL, 1) {
		t.Fatal("the I/Q tier pulse does not take iqBaseTab")
	}
	other := p
	other.beta++
	echo := *p.echo
	echo[3] += 1e-9
	moved := p
	moved.echo = &echo
	if other.iqBase(iqBaseL, 1) || moved.iqBase(iqBaseL, 1) || p.iqBase(iqBaseL+1, 1) {
		t.Error("a pulse other than the I/Q tier design takes iqBaseTab")
	}
	n := 2 * p.span
	d := 1 / float64(iqBaseL)
	for ph := range iqBaseL {
		for i := range n {
			tm := float64(i-p.span) + float64(ph)/float64(iqBaseL)
			for j, v := range iqBaseTab[ph*n+i] {
				if w := p.base(tm + float64(j-2)*d); !near(v, w) {
					t.Errorf("iqBaseTab phase %d tap %d offset %d: %v, the pulse gives %v", ph, i, j-2, v, w)
				}
			}
		}
	}
}
