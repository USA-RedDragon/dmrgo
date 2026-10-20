package rf

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestTunePhaseContinuous(t *testing.T) {
	t.Parallel()
	const hz = 1000.0
	c := newChannelizer(8192, false, 1, false, false)
	tone := func(i int) complex64 {
		p := 2 * math.Pi * hz * float64(i) / basebandRate
		return complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	n := 20333
	for i := range n {
		c.push(tone(i))
	}
	check := func(what string) {
		t.Helper()
		from := max(c.ch.first(), 0) + int64(len(c.taps))
		ref := cmplx.Phase(complex128(c.ch.at(from)))
		for i := from; i < c.ch.n; i++ {
			d := cmplx.Phase(complex128(c.ch.at(i)) * cmplx.Conj(cmplx.Rect(1, ref)))
			if math.Abs(d) > 0.05 {
				t.Fatalf("%s: channel sample %d turned %.3f rad from sample %d", what, i, d, from)
			}
		}
	}
	c.tune(hz)
	check("after tune")
	for i := n; i < n+2000; i++ {
		c.push(tone(i))
	}
	check("after more samples")
}
