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
		x := cleanBaseband(slots, off).noisy(2, 5000)
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

func TestRxFilteredIQ(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	cases := []struct {
		off, cn float64
		tier    Tier
	}{{-2000, 4, Tier1}, {1400, 2, Tier3}, {-2000, 2, Tier3}}
	for _, c := range cases {
		x := cleanBaseband(slots, c.off).noisy(c.cn, 5000)
		raw := scoreBench(truth, receiveBaseband(x, c.tier))["ambe"].Exact
		filt := scoreBench(truth, receiveBaseband(channelFiltered(x, 6250), c.tier))["ambe"].Exact
		if filt < raw*9/10 {
			t.Errorf("%+.0f Hz at %.0f dB, Tier%d: %d voice frames through a 6.25 kHz channel filter, %d without it", c.off, c.cn, c.tier-Tier0, filt, raw)
		}
	}
}
