package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func fmAtStart(freq, env []float64, offsetHz, cn float64, seed uint64) []complex64 {
	x := make([]complex64, len(freq))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for k := range x {
		ph += 2 * math.Pi * (fmMid(freq, k) + offsetHz) / basebandRate
		x[k] = complex(float32(env[k]*math.Cos(ph)+rng.NormFloat64()*sigma), float32(env[k]*math.Sin(ph)+rng.NormFloat64()*sigma))
	}
	return x
}

func TestRxBurstAtFirstSample(t *testing.T) {
	t.Parallel()

	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	for _, seq := range []string{"ack", "data12"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		freq, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		for _, cn := range []float64{30, 60} {
			for _, off := range []float64{0, 1500} {
				x := fmAtStart(freq, env, off, cn, 7)
				for _, tier := range []Tier{Tier1, Tier2, Tier3} {
					got := receiveBaseband(x, tier)
					if len(got) != len(built.Bursts) {
						t.Errorf("%s C/N %.0f dB %+.0f Hz Tier%d: %d bursts, want %d", seq, cn, off, tier-Tier0, len(got), len(built.Bursts))
						continue
					}
					for i := range got {
						if data, _ := got[i].Decoded(); data != built.Bursts[i] {
							t.Errorf("%s C/N %.0f dB %+.0f Hz Tier%d burst %d: decoded bits differ", seq, cn, off, tier-Tier0, i)
						}
						if want := float64(2*i)*slotSeconds + (slotSeconds-burstSeconds)/2; math.Abs(got[i].Seconds-want) > 20e-6 {
							t.Errorf("%s C/N %.0f dB %+.0f Hz Tier%d burst %d: at %.6f s, want %.6f", seq, cn, off, tier-Tier0, i, got[i].Seconds, want)
						}
					}
				}
			}
		}
	}
}
