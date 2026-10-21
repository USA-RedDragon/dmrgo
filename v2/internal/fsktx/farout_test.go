package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestNoiseShapeFarOut(t *testing.T) {
	cases := []struct {
		name  string
		shape int
		zs    [4]float64
		r     float64
	}{
		{"plain", 0, [4]float64{}, 0},
		{"two zeros", 2, [4]float64{12500, 25000}, 0},
		{"three zeros", 3, [4]float64{12500, 22000, 28000}, 0},
		{"notch", 3, [4]float64{12500, 22000, 28000}, 0.99},
	}
	for _, rate := range []int{2400000, 1000000, 192000} {
		var plain float64
		for _, c := range cases {
			cfg := IQConfig{Rate: rate, Tier: Tier2, Exact: true, NoiseShape: c.shape, NoiseZerosHz: c.zs, NoisePoleR: c.r}
			db := farNoise(t, cfg)
			if c.shape == 0 {
				plain = db
			}
			if db > plain+1 {
				t.Errorf("%d Hz %s: rounding noise %.1f dBc in 100 kHz beyond ntfFarHz, want at most 1 dB over plain rounding (%.1f)", rate, c.name, db, plain)
			}
			if rate == 2400000 && db > -60 {
				t.Errorf("%d Hz %s: rounding noise %.1f dBc in 100 kHz beyond 56.25 kHz, want at most -60", rate, c.name, db)
			}
		}
	}
}

func farNoise(t *testing.T, cfg IQConfig) float64 {
	t.Helper()
	q, err := NewIQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ref := cfg
	ref.NoiseShape, ref.NoiseZerosHz, ref.NoisePoleR = 0, [4]float64{}, 0
	r, err := NewIQ(ref)
	if err != nil {
		t.Fatal(err)
	}
	q.Key(true)
	r.Key(true)
	var e []complex128
	var b []byte
	var w []int16
	var pc float64
	x := uint32(1)
	for i := range 1200 {
		x = x*1664525 + 1013904223
		s := int8(2*(x>>30) - 3)
		b = q.AppendS8(b[:0], s)
		w = r.AppendS16(w[:0], s)
		if i < 120 {
			continue
		}
		for j := 0; j < len(b); j += 2 {
			u := complex(float64(w[j])/256, float64(w[j+1])/256)
			e = append(e, complex(float64(int8(b[j])), float64(int8(b[j+1])))-u)
			pc += real(u * cmplx.Conj(u))
		}
	}
	pc /= float64(len(e))
	const n = 4096
	win := make([]float64, n)
	var ww float64
	for i := range win {
		win[i] = 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/n)
		ww += win[i] * win[i]
	}
	psd := make([]float64, n)
	blk := make([]complex128, n)
	k := 0
	for o := 0; o+n <= len(e); o += n {
		for i := range blk {
			blk[i] = e[o+i] * complex(win[i], 0)
		}
		fft(blk)
		for i, v := range blk {
			psd[i] += real(v * cmplx.Conj(v))
		}
		k++
	}
	df := float64(cfg.Rate) / n
	worst := math.Inf(-1)
	for _, sign := range []float64{1, -1} {
		for lo := 56250.0; lo == 56250 || lo+100000 <= float64(cfg.Rate)/2; lo += 10000 {
			var p float64
			for i := range psd {
				f := float64(i) * df
				if i > n/2 {
					f -= float64(cfg.Rate)
				}
				if f *= sign; f >= lo && f < lo+100000 {
					p += psd[i]
				}
			}
			worst = max(worst, 10*math.Log10(p/(float64(k)*n*ww)/pc))
		}
	}
	return worst
}
