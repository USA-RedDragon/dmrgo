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
	checkUnder(t, func(d data) floor { return d.floor }, 0)
}

// awareMargin is the Eb/N0, in dB, by which TestNoTierUnderAware shifts
// the code-aware curve to lower error rates: the code-aware receiver is
// measured, not a bound, on as few seeds as the tiers.
const awareMargin = 0.25

// TestNoTierUnderAware checks, as TestNoTierUnderFloor, that no tier makes
// fewer errors than the code-aware receiver at awareMargin dB more Eb/N0.
func TestNoTierUnderAware(t *testing.T) {
	t.Parallel()
	checkUnder(t, func(d data) floor { return d.aware }, awareMargin)
}

// checkUnder fails when a tier makes fewer errors at some point of the
// sweep data than the reference curves of ref allow at margin dB more
// Eb/N0, with a binomial tail chance under 1e-4.
func checkUnder(t *testing.T, ref func(data) floor, margin float64) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the rxbench source directory")
	}
	d, err := load(filepath.Join(filepath.Dir(thisFile), "..", "..", "v2/rf/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	for k, c := range ref(d).Payloads {
		for _, tr := range d.bench.Tiers {
			for _, p := range tr.Full {
				s := p.Payloads[k]
				if s == nil || s.Truth == 0 {
					continue
				}
				pf, ok := floorAt(c, cnToEbN0(p.CN)+margin)
				if !ok {
					continue
				}
				if q := binomialAtMost(s.Truth-s.Exact, s.Truth, pf); q < 1e-4 {
					t.Errorf("tier %d, %s at C/N %.1f dB: %d errors in %d, reference error rate %.3g (chance %.2g)",
						tr.Tier, k, p.CN, s.Truth-s.Exact, s.Truth, pf, q)
				}
			}
		}
	}
}

// floorAt returns the error rate of reference curve c at Eb/N0 x, by
// linear interpolation of its logarithm between the points on either side,
// and false outside the curve or where a side has no errors. The curves
// are Monte Carlo counts, so each point gives its lower confidence bound at
// refConfidence in place of its error rate: a floor of 56 frames all in
// error does not show an error rate of 1.
func floorAt(c curve, x float64) (float64, bool) {
	for i := 1; i < len(c.EbN0); i++ {
		x0, x1 := c.EbN0[i-1], c.EbN0[i]
		if x < x0-1e-9 || x > x1+1e-9 || c.Truth[i-1] == 0 || c.Truth[i] == 0 {
			continue
		}
		p0 := errorLower(c.Truth[i-1]-c.Exact[i-1], c.Truth[i-1])
		p1 := errorLower(c.Truth[i]-c.Exact[i], c.Truth[i])
		if p0 <= 0 || p1 <= 0 {
			return 0, false
		}
		w := (x - x0) / (x1 - x0)
		return math.Exp((1-w)*math.Log(p0) + w*math.Log(p1)), true
	}
	return 0, false
}

// refConfidence is the chance that errorLower is above the true error rate.
const refConfidence = 1e-3

// errorLower returns the Clopper-Pearson lower bound, at refConfidence, of
// the error rate of k errors in n trials: the p at which k or more errors
// have that chance. Above 1000 errors it takes the normal approximation.
func errorLower(k, n int) float64 {
	if k == 0 {
		return 0
	}
	ph := float64(k) / float64(n)
	if k > 1000 {
		return max(0, ph-3.09*math.Sqrt(ph*(1-ph)/float64(n)))
	}
	lo, hi := 0.0, ph
	for range 60 {
		m := (lo + hi) / 2
		if 1-binomialAtMost(k-1, n, m) < refConfidence {
			lo = m
		} else {
			hi = m
		}
	}
	return lo
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
