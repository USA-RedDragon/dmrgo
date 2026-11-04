package rf

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/resample"
)

// TestDCBlockAdjacentStart feeds the DC blocker an adjacent channel that is
// on from the first sample and checks the part of the DC estimate inside
// 1 kHz of 0 Hz. EN 300 113 asks for 60 dB of adjacent channel selectivity,
// so the estimate must stay 80 dB under the adjacent channel to keep the
// step it leaves 20 dB under a wanted signal 60 dB weaker. The rates are the
// 48 kHz baseband and the rate the 2048 kHz front end runs the blocker at.
func TestDCBlockAdjacentStart(t *testing.T) {
	t.Parallel()
	const bar = 1e-4
	wide := float64(2048000 / newFrontEnd(2048000, IQFormatCS16, basebandRate).dec)
	for _, rate := range []float64{basebandRate, wide} {
		taps := int(rate*0.006) | 1
		h := make([]float32, taps)
		resample.Design(h, taps, 1, 1000/rate, 12)
		var sum float64
		for _, v := range h {
			sum += float64(v)
		}
		for _, hz := range []float64{-14000, -12500, -11000, 11000, 12500, 14000} {
			var d dcBlock
			d.init(rate)
			n := int(rate * 0.06)
			est := make([]complex128, n)
			for i := range n {
				d.remove(complex64(cmplx.Rect(1, 2*math.Pi*hz*float64(i)/rate)))
				est[i] = complex128(d.dc)
			}
			var worst float64
			for k := taps; k < n; k++ {
				var acc complex128
				for j, v := range h {
					acc += complex(float64(v), 0) * est[k-j]
				}
				worst = max(worst, cmplx.Abs(acc)/sum)
			}
			if worst > bar {
				t.Errorf("%.0f Hz rate, adjacent channel at %+.0f Hz: DC estimate within 1 kHz of 0 Hz reaches %.2g of the adjacent channel, want at most %.0g", rate, hz, worst, bar)
			}
		}
	}
}

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

// TestDCBlockFitStart starts the DC blocker on a weak 4FSK carrier 2.5 kHz
// off with an I/Q offset as strong as the carrier, as an RTL-SDR gives on a
// weak signal. The circle fit must find the offset within the first 10 ms;
// the running mean alone still misses about half of it then.
func TestDCBlockFitStart(t *testing.T) {
	t.Parallel()
	for _, rate := range []float64{basebandRate, 204800} {
		const amp = 0.01
		offset := complex64(complex(-0.7123*amp, -0.7019*amp))
		var d dcBlock
		d.init(rate)
		rng := rand.New(rand.NewPCG(3, 4))
		sps := int(rate / 4800)
		var ph, dev float64
		for n := range int(rate / 100) {
			if n%sps == 0 {
				dev = []float64{-1944, -648, 648, 1944}[rng.IntN(4)]
			}
			ph += 2 * math.Pi * (2500 + dev) / rate
			v := offset + complex64(cmplx.Rect(amp, ph)) + complex(float32(rng.NormFloat64()*amp/100), float32(rng.NormFloat64()*amp/100))
			d.remove(v)
		}
		if e := cmplx.Abs(complex128(d.dc-offset)) / amp; e > 0.05 {
			t.Errorf("%.0f Hz rate: DC estimate off by %.3f of the carrier 10 ms after the start, want at most 0.05", rate, e)
		}
	}
}
