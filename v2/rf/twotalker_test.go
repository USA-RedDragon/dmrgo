package rf

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type ttTalker struct {
	bursts []layer2.RawBurst
	at     []float64
}

func twoTalkers(t testing.TB, ppm [2]float64, sf int) (f, e [2][]float64, tk [2]ttTalker) {
	t.Helper()
	ramp := (slotSeconds - burstSeconds) / 2
	for k := range 2 {
		cfg := testConfig(t)
		cfg.Source, cfg.Superframes = []uint32{3009104, 3009105}[k], sf
		b, err := rftx.Sequence("voice", cfg)
		if err != nil {
			t.Fatal(err)
		}
		f0, e0 := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: k})
		f[k], e[k] = resampleFM(f0, e0, ppm[k])
		tk[k].bursts = b.Bursts
		for i := range b.Bursts {
			tk[k].at = append(tk[k].at, benchPad+(float64(2*i+k)*slotSeconds+ramp)/(1+ppm[k]*1e-6))
		}
	}
	return f, e, tk
}

func twoTalkerIQ(t testing.TB, f, e [2][]float64, offsetHz, cn float64, seed uint64) []complex64 {
	seed = noiseSeed(t, seed)
	pad := int(benchPad * basebandRate)
	n := max(len(f[0]), len(f[1])) + 2*pad
	x := make([]complex64, n)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph [2]float64
	for i := range x {
		var re, im float64
		for j := range 2 {
			if k := i - pad; k >= 0 && k < len(f[j]) {
				ph[j] += 2 * math.Pi * (fmMid(f[j], k) + offsetHz) / basebandRate
				re += e[j][k] * math.Cos(ph[j])
				im += e[j][k] * math.Sin(ph[j])
			}
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	return x
}

func twoTalkerErrors(tk [2]ttTalker, got []Burst) [2][]int {
	var errs [2][]int
	for k := range 2 {
		errs[k] = make([]int, len(tk[k].bursts))
		for i := range errs[k] {
			errs[k][i] = 264
		}
	}
	for _, b := range got {
		for k := range 2 {
			for i, at := range tk[k].at {
				if math.Abs(b.Seconds-at) > 0.005 {
					continue
				}
				n := 0
				for j, w := range tk[k].bursts[i] {
					n += bits.OnesCount8(b.Data[j] ^ w)
				}
				errs[k][i] = min(errs[k][i], n)
			}
		}
	}
	return errs
}

func ttAudio(x []complex64, hz int, tier Tier) []Burst {
	a := discAudio(x, hz)
	r := mustAudio(AudioRxConfig{Rate: hz, Tier: tier})
	return r.AppendFlush(r.AppendBursts(nil, a))
}

// TestRxTwoTalkerClocks checks that two MS talkers, one on each TDMA slot,
// each keep their own symbol clock: the slot 1 talker at +400 ppm and the
// slot 2 talker at -400 ppm, and the other way round, at 12 dB, over I/Q and
// discriminator audio at Tiers 0 to 2. A single clock for both slots walks
// the bursts of one talker off their symbols. It fails when a burst is lost
// or has more than 10 raw bit errors; the first burst of each talker comes
// before any clock evidence of its own and may have 20.
func TestRxTwoTalkerClocks(t *testing.T) {
	t.Parallel()
	const seeds = 4
	for _, ppm := range [][2]float64{{400, -400}, {-400, 400}} {
		f, e, tk := twoTalkers(t, ppm, 3)
		for _, audio := range []bool{false, true} {
			for _, tier := range []Tier{Tier0, Tier1, Tier2} {
				forSeeds(seeds, func(i int) {
					x := twoTalkerIQ(t, f, e, 1400, 12, uint64(i+1))
					var got []Burst
					if audio {
						got = ttAudio(x, 48000, tier)
					} else {
						got = receiveBaseband(x, tier)
					}
					for k, errs := range twoTalkerErrors(tk, got) {
						for j, n := range errs {
							if n > 10 && (j > 0 || n > 20) {
								t.Errorf("%+.0f/%+.0f ppm, audio %v, Tier%d, seed %d: talker %d burst %d has %d raw bit errors", ppm[0], ppm[1], audio, tier-Tier0, i+1, k+1, j, n)
							}
						}
					}
				})
			}
		}
	}
}
