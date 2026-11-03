package fsktx

import (
	"math"
	"math/cmplx"
	"testing"
)

func TestPreEmphasisRange(t *testing.T) {
	t.Parallel()
	for _, c := range [][2]float64{{-1, 3000}, {3000, 300}, {300, 0}, {300, 30000}} {
		if _, err := NewAudio(AudioConfig{Tier: Tier1, PreEmphasisHz: c}); err == nil {
			t.Errorf("PreEmphasisHz %v: no error", c)
		}
	}
}

func TestPreEmphasisFit(t *testing.T) {
	t.Parallel()
	resp := func(d [5]float64, f, hz float64) complex128 {
		z := cmplx.Rect(1, -2*math.Pi*f/hz)
		return (complex(d[0], 0) + complex(d[1], 0)*z + complex(d[2], 0)*z*z) / (1 + complex(d[3], 0)*z + complex(d[4], 0)*z*z)
	}
	for _, hz := range []int{8000, 11025, 16000, 22050, 48000, 96000} {
		a, err := NewAudio(AudioConfig{Hz: hz, Level: 4000, Tier: Tier1, PreEmphasisHz: [2]float64{300, 3000}})
		if err != nil {
			t.Fatal(err)
		}
		r := float64(hz)
		tz, tp := math.Tan(math.Pi*300/r), math.Tan(math.Pi*3000/r)
		a0 := 1 + 1/tz
		bil := [5]float64{(1 + 1/tp) / a0, (1 - 1/tp) / a0, 0, (1 - 1/tz) / a0, 0}
		var fitDB, fitDeg, bilDB float64
		for f := 25.0; f <= min(preFitBandHz, 0.4*r); f += 25 {
			want := complex(1, f/3000) / complex(1, f/300)
			e := resp(a.de, f, r) / want
			fitDB = max(fitDB, math.Abs(20*math.Log10(cmplx.Abs(e))))
			fitDeg = max(fitDeg, math.Abs(cmplx.Phase(e))*180/math.Pi)
			bilDB = max(bilDB, math.Abs(20*math.Log10(cmplx.Abs(resp(bil, f, r)/want))))
		}
		t.Logf("%d Hz: fitted inverse within %.3f dB and %.2f degrees of the analog inverse, bilinear within %.3f dB", hz, fitDB, fitDeg, bilDB)
		if fitDB > 0.6 || fitDB > bilDB/4 || fitDeg > 3.5 {
			t.Errorf("%d Hz: fitted inverse off the analog inverse by %.3f dB and %.2f degrees, bilinear by %.3f dB", hz, fitDB, fitDeg, bilDB)
		}
	}
}
