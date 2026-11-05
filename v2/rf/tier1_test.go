package rf

import (
	"math"
	"sync"
	"testing"
)

const pairedZ = 4

func forSeeds(n int, f func(i int)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			f(i)
		}()
	}
	wg.Wait()
}

func meanSE(d []float64) (mean, se float64) {
	for _, v := range d {
		mean += v
	}
	mean /= float64(len(d))
	var ss float64
	for _, v := range d {
		ss += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(ss / float64(len(d)-1) / float64(len(d)))
}

// TestTier1Weak checks that Tier1 gains at least the stated rate over Tier0
// on weak bursts. Per seed it takes the rate of Tier1 minus the rate of
// Tier0, and fails when the mean gain over the seeds is more than pairedZ
// standard errors below the stated gain.
func TestTier1Weak(t *testing.T) {
	t.Parallel()
	const seeds = 32
	slots, truth := benchStream(t, 10)
	for _, c := range []struct {
		mode string
		cn   float64
	}{{benchIQ, 2}, {benchAudio, 3}} {
		gains := []struct {
			k    string
			gain float64
			d    []float64
		}{{"rate12", 0, make([]float64, seeds)}, {"ambe_c0c1", 0.05, make([]float64, seeds)}}
		forSeeds(seeds, func(i int) {
			x := noisyBaseband(t, slots, 1400, c.cn, 5+uint64(i))
			s0 := scoreBench(truth, benchReceive(t, x, benchEntry{c.mode, Tier0}))
			s1 := scoreBench(truth, benchReceive(t, x, benchEntry{c.mode, Tier1}))
			for _, p := range gains {
				p.d[i] = float64(s1[p.k].Exact-s0[p.k].Exact) / float64(max(s0[p.k].Truth, 1))
			}
		})
		for _, p := range gains {
			mean, se := meanSE(p.d)
			t.Logf("%s at C/N %v dB, %s: Tier1 gain %.3f, SE %.3f over %d seeds", c.mode, c.cn, p.k, mean, se, seeds)
			if mean+pairedZ*se < p.gain {
				t.Errorf("%s at C/N %v dB, %s: Tier1 gain %.3f, SE %.3f, below %.2f", c.mode, c.cn, p.k, mean, se, p.gain)
			}
		}
	}
}
