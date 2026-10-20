package rf

import (
	"math/rand/v2"
	"testing"
)

// TestScanPair checks that the template of a data SYNC is the conjugate of
// that of the voice SYNC of its source, that scanPair gives the sums of
// the segment correlations of the plain loop bit for bit, and that it
// leaves a source only when neither share can reach the threshold.
func TestScanPair(t *testing.T) {
	t.Parallel()
	o := tierOptions(Tier2)
	s := newCohStageOf(&o)
	rng := rand.New(rand.NewPCG(1, 2))
	ref := func(x []complex64, tpl *[cohScanLen]complex64) float64 {
		var m float64
		for g := range cohScanSegs {
			var c complex64
			for j := g * cohScanSeg; j < (g+1)*cohScanSeg; j++ {
				c += x[j] * tpl[j]
			}
			m += float64(real(c)*real(c) + imag(c)*imag(c))
		}
		return m
	}
	for i := range 4 {
		var tv, td [cohScanLen]complex64
		voice, data, _ := sourceAt(i).SyncPatterns()
		s.scanTemplate(voice, &tv)
		s.scanTemplate(data, &td)
		for j := range tv {
			if td[j] != complex(real(tv[j]), -imag(tv[j])) {
				t.Fatalf("source %d sample %d: data %v, voice %v", i, j, td[j], tv[j])
			}
		}
		for trial := range 2000 {
			var x [cohScanLen]complex64
			sig := float32(trial%5) * 0.3
			var e, e3 float64
			for j := range x {
				if j == 3*cohScanSeg {
					e3 = e
				}
				v := complex(float32(rng.NormFloat64()), float32(rng.NormFloat64()))
				if trial%2 == 0 {
					v += complex(sig, 0) * complex(real(tv[j]), -imag(tv[j]))
				} else {
					v += complex(sig, 0) * tv[j]
				}
				x[j] = v
				e += float64(real(v)*real(v) + imag(v)*imag(v))
			}
			wv, wd := ref(x[:], &tv), ref(x[:], &td)
			mv, md, ok := scanPair(x[:], &tv, 0, -1)
			if !ok || mv != wv || md != wd {
				t.Fatalf("source %d trial %d: %v %v, want %v %v", i, trial, mv, md, wv, wd)
			}
			thr := 0.45 * e * cohScanLen / cohScanSegs
			if _, _, ok := scanPair(x[:], &tv, (e-e3)*cohScanSeg*cohScanBound, thr); !ok && max(wv, wd) >= thr {
				t.Fatalf("source %d trial %d: left at %v, %v over %v", i, trial, wv, wd, thr)
			}
		}
	}
}
