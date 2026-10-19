//go:build !tinygo

package fsktx

import (
	"errors"
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"
)

// predMargin is the least margin in dB to Emission Mask D from 8 to 12.5
// kHz of the peak-hold power in 100 Hz of the I/Q of cfg on random
// symbols.
func predMargin(t *testing.T, cfg IQConfig) float64 {
	q, err := NewIQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	q.Key(true)
	r := rand.New(rand.NewPCG(3, 4))
	var out []int16
	for range 6000 {
		out = q.AppendS16(out, int8(2*r.IntN(4)-3))
	}
	rate := float64(cfg.Rate)
	x := make([]complex128, len(out)/2)
	var pw float64
	for i := range x {
		x[i] = complex(float64(out[2*i]), float64(out[2*i+1]))
		pw += real(x[i])*real(x[i]) + imag(x[i])*imag(x[i])
	}
	pw /= float64(len(x))
	x = x[int(rate*0.1):]
	const n = 2048
	var wsum float64
	w := make([]float64, n)
	for i := range w {
		a := 2 * math.Pi * float64(i) / n
		w[i] = 0.35875 - 0.48829*math.Cos(a) + 0.14128*math.Cos(2*a) - 0.01168*math.Cos(3*a)
		wsum += w[i] * w[i]
	}
	peak := make([]float64, n)
	buf := make([]complex128, n)
	for st := 0; st+n <= len(x); st += n / 8 {
		for i := range buf {
			buf[i] = x[st+i] * complex(w[i], 0)
		}
		fftInPlace(buf)
		for i, v := range buf {
			peak[i] = max(peak[i], real(v*cmplx.Conj(v))/(n*wsum)/pw)
		}
	}
	bin := rate / n
	per := int(math.Round(100 / bin))
	m := math.Inf(1)
	for i := range n {
		f := float64(i) * bin
		if i >= n/2 {
			f -= rate
		}
		if math.Abs(f) < 8000 || math.Abs(f) > 12500 {
			continue
		}
		var p float64
		for j := range per {
			p += peak[(i+j-per/2+n)%n]
		}
		m = min(m, -10*math.Log10(p)-optMaskAttn(f))
	}
	return m
}

// fftInPlace is a radix-2 FFT of x.
func fftInPlace(x []complex128) {
	n := len(x)
	for i, j := 1, 0; i < n; i++ {
		b := n >> 1
		for ; j&b != 0; b >>= 1 {
			j ^= b
		}
		j |= b
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		w := cmplx.Rect(1, -2*math.Pi/float64(size))
		for st := 0; st < n; st += size {
			tw := complex(1, 0)
			for k := range size / 2 {
				u, v := x[st+k], x[st+k+size/2]*tw
				x[st+k], x[st+k+size/2] = u+v, u-v
				tw *= w
			}
		}
	}
}

// TestShapePredMask checks that the predictor lowers the peak power from
// 8 to 12.5 kHz on both tiers. The kernels fitted to a pulse of 8 symbols
// each side raised it by 4 dB instead, as they start predLo samples after
// their symbol, past the peak of that pulse.
func TestShapePredMask(t *testing.T) {
	t.Parallel()
	for _, tier := range []Tier{Tier1, Tier2} {
		cfg := IQConfig{Rate: 96000, Tier: tier, Exact: true, Pulse: predPulse}
		base := predMargin(t, cfg)
		cfg.ShapePred = true
		if got := predMargin(t, cfg); got < base+3 {
			t.Errorf("%v: margin %.1f dB with the predictor, %.1f dB without", tier, got, base)
		}
	}
	_, err := NewIQ(IQConfig{Rate: 96000, Tier: Tier1, Exact: true, Pulse: PulseConfig{Window: 3, Beta: 17, Flat: true}, ShapePred: true})
	if !errors.Is(err, errPred) {
		t.Errorf("Tier1 with a pulse of 8 symbols: %v, want errPred", err)
	}
}
