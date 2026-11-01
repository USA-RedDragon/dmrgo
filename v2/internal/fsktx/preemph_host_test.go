//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestPreEmphasisInverts(t *testing.T) {
	t.Parallel()
	const hz = 48000
	flat, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier2})
	if err != nil {
		t.Fatal(err)
	}
	pre, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier2, PreEmphasisHz: [2]float64{preZeroHz, prePoleHz}})
	if err != nil {
		t.Fatal(err)
	}
	if flat.shape == 0 || flat.shape != pre.shape || flat.fb != pre.fb || flat.poles || pre.poles || flat.sync != pre.sync {
		t.Fatal("the flat and pre-emphasized Audio do not shape the rounding alike")
	}
	var a, b []int16
	syms := [...]int8{3, 3, -3, -3, 1, -1, 3, -1, -3, 1, 1, 3}
	for i := range 2000 {
		s := syms[i%len(syms)]
		a = flat.AppendSymbol(a, s)
		b = pre.AppendSymbol(b, s)
	}
	x := make([]float64, len(b))
	for i, v := range b {
		x[i] = float64(v)
	}
	y := preEmph(x, hz)
	model, pk, rms := preInverseBounds(a, pre.de, flat.fb[:2*flat.shape], hz)
	var worst, worstModel, ss, ssModel float64
	lo, hi := len(a)/10, len(a)*9/10
	for i := lo; i < hi; i++ {
		d := y[i] - float64(a[i])
		worst, worstModel = max(worst, math.Abs(d)), max(worstModel, math.Abs(model[i]))
		ss, ssModel = ss+d*d, ssModel+model[i]*model[i]
		if r := math.Abs(d - model[i]); r > pk {
			t.Fatalf("sample %d: the rounding noise through the analog pre-emphasis is %.2f codes, over its bound %.2f", i, r, pk)
		}
	}
	n := float64(hi - lo)
	if top := worstModel + pk; worst > top {
		t.Errorf("pre-emphasized output through the analog pre-emphasis differs from flat by %.2f codes, over the bound %.2f", worst, top)
	}
	if got, top := math.Sqrt(ss/n), math.Sqrt(ssModel/n)+rms; got > top {
		t.Errorf("pre-emphasized output through the analog pre-emphasis differs from flat by %.2f codes RMS, over the bound %.2f", got, top)
	}
}

func preInverseBounds(a []int16, d [5]float64, fb []float64, rate float64) (model []float64, pk, rms float64) {
	x := make([]complex128, len(a))
	for i, v := range a {
		x[i] = complex(float64(v), 0)
	}
	af := spectrumOf(x)
	m := len(af)
	gp, gd := make([]complex128, m), make([]complex128, m)
	for i := range af {
		k := i
		if k >= m/2 {
			k -= m
		}
		f := float64(k) * rate / float64(m)
		z := cmplx.Rect(1, -2*math.Pi*f/rate)
		p := (1 + complex(0, f/preZeroHz)) / (1 + complex(0, f/prePoleHz))
		inv := (complex(d[0], 0) + complex(d[1], 0)*z + complex(d[2], 0)*z*z) / (1 + complex(d[3], 0)*z + complex(d[4], 0)*z*z)
		s := complex(1, 0)
		for j, c := range fb {
			s += complex(c, 0) * cmplx.Pow(z, complex(float64(j+1), 0))
		}
		af[i] *= p*inv - 1
		gp[i], gd[i] = p*s, p*inv*s
		rms = max(rms, 0.5*(cmplx.Abs(gp[i])+cmplx.Abs(gd[i])))
	}
	ifft(af)
	ifft(gp)
	ifft(gd)
	model = make([]float64, len(a))
	for i := range model {
		model[i] = real(af[i])
	}
	for i := range gp {
		pk += 0.5 * (math.Abs(real(gp[i])) + math.Abs(real(gd[i])))
	}
	return model, pk, rms
}
