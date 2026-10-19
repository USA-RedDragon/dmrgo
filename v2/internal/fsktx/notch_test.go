package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

// TestNoisePoleNotch checks that NoisePoleR lowers the 8-bit rounding
// noise in its notch and keeps it near that of plain rounding far out.
func TestNoisePoleNotch(t *testing.T) {
	const rate = 2400000
	base := IQConfig{Rate: rate, Tier: Tier2, Exact: true}
	notch := base
	notch.NoiseShape, notch.NoiseZerosHz, notch.NoisePoleR = 3, [4]float64{12500, 22000, 28000}, 0.99
	noise := func(cfg IQConfig) func(f float64) float64 {
		q, err := NewIQ(cfg)
		if err != nil {
			t.Fatal(err)
		}
		r, err := NewIQ(IQConfig{Rate: rate, Tier: Tier2, Exact: true})
		if err != nil {
			t.Fatal(err)
		}
		q.Key(true)
		r.Key(true)
		var e []complex128
		var b []byte
		var w []int16
		x := uint32(1)
		for i := range 4800 {
			x = x*1664525 + 1013904223
			s := int8(2*(x>>30) - 3)
			b = q.AppendS8(b[:0], s)
			w = r.AppendS16(w[:0], s)
			if i < 480 {
				continue
			}
			for j := 0; j < len(b); j += 2 {
				e = append(e, complex(float64(int8(b[j]))-float64(w[j])/256, float64(int8(b[j+1]))-float64(w[j+1])/256))
			}
		}
		return func(f float64) float64 {
			const m = 1200
			var p float64
			for k := 0; k+m <= len(e); k += m {
				var s complex128
				for n, v := range e[k : k+m] {
					h := 1 - math.Cos(2*math.Pi*float64(n)/m)
					s += v * cmplx.Rect(h, -2*math.Pi*f*float64(k+n)/rate)
				}
				p += real(s * cmplx.Conj(s))
			}
			return p
		}
	}
	plain, shaped := noise(base), noise(notch)
	for _, c := range []struct{ hz, db float64 }{{22000, -10}, {25000, -5}} {
		if db := 10 * math.Log10(shaped(c.hz)/plain(c.hz)); db > c.db {
			t.Errorf("noise at %.0f Hz %.1f dB from plain rounding, want at most %.0f", c.hz, db, c.db)
		}
	}
	if db := 10 * math.Log10(shaped(600000)/plain(600000)); math.Abs(db) > 3 {
		t.Errorf("noise at 600 kHz %.1f dB from plain rounding, want within 3", db)
	}
	if _, err := NewIQ(IQConfig{Rate: rate, NoiseShape: 1, NoisePoleR: 1}); err == nil {
		t.Error("NoisePoleR 1: no error")
	}
}
