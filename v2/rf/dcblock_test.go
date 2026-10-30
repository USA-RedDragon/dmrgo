package rf

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

func TestDCBlockCarrierLead(t *testing.T) {
	t.Parallel()
	const rate = basebandRate
	for _, c := range []struct {
		name     string
		offset   complex64
		cw       bool
		fsk      float64
		noise    float64
		maxAfter float64
	}{
		{"offset in noise", complex(-0.004, -0.004), false, 0, 0.05, 0.1},
		{"large offset, quiet input", complex(0.02, -0.01), false, 0, 0.002, 0.1},
		{"large offset, then 4FSK at 0 Hz", complex(0.02, -0.01), false, 0.01, 0.002, 0.1},
		{"cw lead at 0 Hz", 0, true, 0.9, 0.002, 0.1},
	} {
		var d dcBlock
		d.init(rate)
		rng := rand.New(rand.NewPCG(1, 2))
		var ph, dev float64
		var dc complex64
		for n := range rate {
			if n%(rate/4800) == 0 {
				dev = []float64{-1944, -648, 648, 1944}[rng.IntN(4)]
			}
			v := c.offset + complex(float32(rng.NormFloat64()*c.noise), float32(rng.NormFloat64()*c.noise))
			switch {
			case c.cw && n < rate/2:
				v += complex(float32(c.fsk), 0)
			case n >= rate/2:
				ph += 2 * math.Pi * dev / rate
				v += complex64(cmplx.Rect(c.fsk, ph))
			}
			d.remove(v)
			if n == rate/2+rate/50 {
				dc = d.dc
			}
		}
		if c.cw {
			if e := cmplx.Abs(complex128(dc)); e > c.maxAfter*0.9 {
				t.Errorf("%s: DC estimate %.3f 20 ms after the CW ended, want under %.3f", c.name, e, c.maxAfter*0.9)
			}
			continue
		}
		if e := cmplx.Abs(complex128(d.dc-c.offset)) / cmplx.Abs(complex128(c.offset)); e > c.maxAfter {
			t.Errorf("%s: DC estimate %v after 1 s, want %v within %.0f%%", c.name, d.dc, c.offset, 100*c.maxAfter)
		}
	}
}
