package main

import (
	"math"
	"path/filepath"
	"runtime"
	"testing"
)

// TestNoTierUnderFloor checks that at every Eb/N0 of the sweep data no tier
// makes fewer errors than its floor allows: the chance of seeing so few
// errors from a tier as good as the floor, by the binomial tail, must stay
// above 1e-4.
func TestNoTierUnderFloor(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the rxbench source directory")
	}
	d, err := load(filepath.Join(filepath.Dir(thisFile), "..", "..", "v2/cmd/dmrrf/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	for k, c := range d.floor.Payloads {
		for _, tr := range d.bench.Tiers {
			for _, p := range tr.Full {
				s := p.Payloads[k]
				if s == nil || s.Truth == 0 {
					continue
				}
				pf, ok := floorAt(c, cnToEbN0(p.CN))
				if !ok {
					continue
				}
				if q := binomialAtMost(s.Truth-s.Exact, s.Truth, pf); q < 1e-4 {
					t.Errorf("tier %d, %s at C/N %.1f dB: %d errors in %d, floor error rate %.3g (chance %.2g)",
						tr.Tier, k, p.CN, s.Truth-s.Exact, s.Truth, pf, q)
				}
			}
		}
	}
}

// floorAt returns the error rate of floor curve c at Eb/N0 x, by linear
// interpolation of its logarithm between the points on either side, and
// false outside the curve or where a side has no errors.
func floorAt(c curve, x float64) (float64, bool) {
	for i := 1; i < len(c.EbN0); i++ {
		x0, x1 := c.EbN0[i-1], c.EbN0[i]
		if x < x0-1e-9 || x > x1+1e-9 || c.Truth[i-1] == 0 || c.Truth[i] == 0 {
			continue
		}
		p0 := 1 - float64(c.Exact[i-1])/float64(c.Truth[i-1])
		p1 := 1 - float64(c.Exact[i])/float64(c.Truth[i])
		if p0 <= 0 || p1 <= 0 {
			return 0, false
		}
		w := (x - x0) / (x1 - x0)
		return math.Exp((1-w)*math.Log(p0) + w*math.Log(p1)), true
	}
	return 0, false
}

// binomialAtMost returns the chance of at most k successes in n trials of
// chance p.
func binomialAtMost(k, n int, p float64) float64 {
	if k >= n {
		return 1
	}
	lp, lq := math.Log(p), math.Log1p(-p)
	var sum float64
	for i := 0; i <= k; i++ {
		lg, _ := math.Lgamma(float64(n + 1))
		li, _ := math.Lgamma(float64(i + 1))
		lr, _ := math.Lgamma(float64(n - i + 1))
		sum += math.Exp(lg - li - lr + float64(i)*lp + float64(n-i)*lq)
	}
	return sum
}
