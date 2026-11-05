package rf

import (
	"math"
	"testing"
)

func channelFiltered(x []complex64, cutHz float64) []complex64 {
	taps := windowTaps(31, cutHz/basebandRate, 0)
	var sum float64
	for _, v := range taps {
		sum += v
	}
	y := make([]complex64, len(x))
	for i := range x {
		var acc complex64
		for k, v := range taps {
			if j := i - k + len(taps)/2; j >= 0 && j < len(x) {
				acc += x[j] * complex(float32(v/sum), 0)
			}
		}
		y[i] = acc
	}
	return y
}

func TestSpectrumFloorFilteredNoise(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	for _, off := range []float64{1400, -2000} {
		x := cleanBaseband(slots, off).noisy(t, 2, 5000)
		for _, filt := range []bool{false, true} {
			y := x
			if filt {
				y = channelFiltered(x, 6250)
			}
			s := newSpectrum(basebandRate)
			got := math.NaN()
			for _, v := range y[int(0.35*basebandRate):] {
				if s.push(v) && s.ready && s.OK {
					got = s.Hz
					break
				}
			}
			if !(math.Abs(got-off) < 300) {
				t.Errorf("%+.0f Hz carrier at 2 dB, filtered %v: spectrum at %.0f Hz", off, filt, got)
			}
		}
	}
}

// TestRxFilteredIQ checks that a 6.25 kHz channel filter keeps 90% of the
// voice frames of each case. Per seed it takes the margin filtered minus 0.9
// times raw, and fails when the mean margin over the seeds is more than
// pairedZ standard errors below zero.
func TestRxFilteredIQ(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		off, cn float64
		tier    Tier
		n       int
		seeds   int
	}{{-2000, 4, Tier1, 6, 64}, {1400, 2, Tier3, 2, 4}, {-2000, 2, Tier3, 2, 4}} {
		slots, truth := benchStream(t, c.n)
		sig := cleanBaseband(slots, c.off)
		d := make([]float64, c.seeds)
		forSeeds(c.seeds, func(i int) {
			x := sig.noisy(t, c.cn, 5000+uint64(i))
			r := float64(scoreBench(truth, receiveBaseband(x, c.tier))["ambe"].Exact)
			f := float64(scoreBench(truth, receiveBaseband(channelFiltered(x, 6250), c.tier))["ambe"].Exact)
			d[i] = f - 0.9*r
		})
		mean, se := meanSE(d)
		t.Logf("%+.0f Hz at %.0f dB, Tier%d: margin %.2f, SE %.2f over %d seeds", c.off, c.cn, c.tier-Tier0, mean, se, c.seeds)
		if mean+pairedZ*se < 0 {
			t.Errorf("%+.0f Hz at %.0f dB, Tier%d: filtered voice frames %.2f per seed below 90%% of raw, SE %.2f", c.off, c.cn, c.tier-Tier0, -mean, se)
		}
	}
}
