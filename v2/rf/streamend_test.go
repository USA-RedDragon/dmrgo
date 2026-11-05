//go:build !tinygo

package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
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

func TestRxStreamEndKeepsFades(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	clean := cleanBaseband(slots, 1400)
	fm, env := rftx.ModulateBS(slots)
	cases := []struct {
		name string
		x    func(seed uint64) []complex64
		want map[string]int
	}{
		{"12 dB step", func(seed uint64) []complex64 {
			return shapedBaseband(clean, 15, seed, func(i int) float64 {
				if i/(basebandRate/2)%2 == 1 {
					return math.Pow(10, -12.0/20)
				}
				return 1
			})
		}, map[string]int{"iq0": 395, "iq3": 432, "audio0": 323, "audio2": 421}},
		{"Rayleigh 5 Hz", func(seed uint64) []complex64 { return fadedBaseband(fm, env, 5, 10, seed) }, map[string]int{"iq0": 358, "iq3": 387, "audio0": 346, "audio2": 375}},
	}
	for _, c := range cases {
		for _, e := range []benchEntry{{benchIQ, Tier0}, {benchIQ, Tier3}, {benchAudio, Tier0}, {benchAudio, Tier2}} {
			got := 0
			for seed := uint64(9000); seed < 9004; seed++ {
				got += scoreBench(truth, benchReceive(t, c.x(seed), e))["ambe_c0c1"].Exact
			}
			if want, ok := c.want[e.name()]; ok && got < want {
				t.Errorf("%s, %s: %d frames with C0 and C1 exact, want at least %d", c.name, e.name(), got, want)
			}
		}
	}
}
