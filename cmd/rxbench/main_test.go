package main

import (
	"math"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestNoTierUnderFloor(t *testing.T) {
	t.Parallel()
	checkUnder(t, func(d data) map[string]curve {
		out := map[string]curve{}
		for k, f := range d.floor.Payloads {
			out[k] = f.DMR
		}
		return out
	}, 0)
}

const awareMargin = 0.25

func TestNoTierUnderAware(t *testing.T) {
	t.Parallel()
	checkUnder(t, func(d data) map[string]curve { return d.aware.Payloads }, awareMargin)
}

func checkUnder(t *testing.T, ref func(data) map[string]curve, margin float64) {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot find the rxbench source directory")
	}
	d, err := load(filepath.Join(filepath.Dir(thisFile), "..", "..", "v2/rf/testdata"))
	if err != nil {
		t.Fatal(err)
	}
	for k, c := range ref(d) {
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

const refConfidence = 1e-3

var errorLowers sync.Map

func errorLower(k, n int) float64 {
	if v, ok := errorLowers.Load([2]int{k, n}); ok {
		if p, ok := v.(float64); ok {
			return p
		}
	}
	p := errorLowerOf(k, n)
	errorLowers.Store([2]int{k, n}, p)
	return p
}

func errorLowerOf(k, n int) float64 {
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

func binomialAtMost(k, n int, p float64) float64 {
	if k >= n {
		return 1
	}
	lp, lq := math.Log(p), math.Log1p(-p)
	var sum float64
	lg, _ := math.Lgamma(float64(n + 1))
	for i := 0; i <= k; i++ {
		li, _ := math.Lgamma(float64(i + 1))
		lr, _ := math.Lgamma(float64(n - i + 1))
		sum += math.Exp(lg - li - lr + float64(i)*lp + float64(n-i)*lq)
	}
	return sum
}
