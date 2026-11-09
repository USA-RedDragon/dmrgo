//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

func shapedBaseband(s benchSignal, cn float64, seed uint64, gain func(i int) float64) []complex64 {
	x := make([]complex64, len(s.re))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	for i := range x {
		a := gain(i)
		x[i] = complex(float32(a*s.re[i]+rng.NormFloat64()*sigma), float32(a*s.im[i]+rng.NormFloat64()*sigma))
	}
	return x
}

func TestRxStreamEndNoise(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	end := benchPad + 40*slotSeconds
	n := 0
	for _, e := range []benchEntry{{benchIQ, Tier0}, {benchIQ, Tier1}, {benchIQ, Tier2}, {benchIQ, Tier3}, {benchAudio, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier2}, {benchAudio, Tier3}} {
		for seed := uint64(1); seed <= 8; seed++ {
			for _, b := range benchReceive(t, noisyBaseband(t, slots[:40], 1400, 10, seed), e) {
				if b.Seconds > end+0.005 {
					n++
				}
			}
		}
	}
	if n > 4 {
		t.Errorf("%d bursts from the noise after 64 streams end, want at most 4", n)
	}
}

const keepsFadesDraws = 560

// TestRxStreamEndKeepsFades checks that the stream-end gate drops no faded
// voice: in a 12 dB C/N step and a 5 Hz Rayleigh fade, a receiver must not
// miss significantly more C0 and C1 frames than the same receiver with the
// gate off, whose misses over keepsFadesDraws payload draws give the mean
// and variance of each case.
func TestRxStreamEndKeepsFades(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	clean := cleanBaseband(slots, 1400)
	fm, env := rftx.ModulateBS(slots)
	cases := []struct {
		name string
		x    func(seed uint64) []complex64
		ref  map[string][2]float64
	}{
		{"12 dB step", func(seed uint64) []complex64 {
			return shapedBaseband(clean, 15, seed, func(i int) float64 {
				if i/(basebandRate/2)%2 == 1 {
					return math.Pow(10, -12.0/20)
				}
				return 1
			})
		}, map[string][2]float64{"iq0": {36.0, 27.7}, "iq3": {0.1, 0.1}, "audio0": {93.5, 49.6}, "audio2": {8.1, 8.2}}},
		{"Rayleigh 5 Hz", func(seed uint64) []complex64 { return fadedBaseband(fm, env, 5, 10, seed) }, map[string][2]float64{"iq0": {67.3, 12.9}, "iq3": {40.2, 6.3}, "audio0": {85.3, 14.9}, "audio2": {55.4, 9.1}}},
	}
	for _, c := range cases {
		for _, e := range []benchEntry{{benchIQ, Tier0}, {benchIQ, Tier3}, {benchAudio, Tier0}, {benchAudio, Tier2}} {
			var sc benchScore
			for seed := uint64(9000); seed < 9004; seed++ {
				s := scoreBench(truth, benchReceive(t, c.x(seed), e))["ambe_c0c1"]
				sc.Exact, sc.Truth = sc.Exact+s.Exact, sc.Truth+s.Truth
			}
			if ref := c.ref[e.name()]; testseed.CountAbove(sc.Truth-sc.Exact, ref[0], ref[1]) {
				t.Errorf("%s, %s: %d of %d frames with C0 and C1 exact; without the stream-end gate the receiver misses %.1f on average (variance %.1f) over %d payload draws, so %d misses or more has probability below %g",
					c.name, e.name(), sc.Exact, sc.Truth, ref[0], ref[1], keepsFadesDraws, testseed.CountBar(ref[0], ref[1])+1, testseed.BarAlpha)
			}
		}
	}
}
