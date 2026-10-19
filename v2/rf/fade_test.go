package rf

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// fadedBaseband is the transmission fm, env with 0.3 s of noise on each
// side, 1200 Hz off, through a Rayleigh fader of Doppler fd Hz with a Jakes
// spectrum and unit mean power, at C/N cn.
func fadedBaseband(fm, env []float64, fd, cn float64, seed uint64) []complex64 {
	pad := int(0.3 * basebandRate)
	x := make([]complex64, len(fm)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	const m = 64
	var w, ph0 [m]float64
	for i := range m {
		a := 2 * math.Pi * (float64(i) + rng.Float64()) / m
		w[i] = 2 * math.Pi * fd * math.Cos(a) / basebandRate
		ph0[i] = 2 * math.Pi * rng.Float64()
	}
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var v complex128
		if k := i - pad; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + 1200) / basebandRate
			var g complex128
			for j := range m {
				s, c := math.Sincos(ph0[j] + w[j]*float64(i))
				g += complex(c, s)
			}
			g /= complex(math.Sqrt(m), 0)
			v = g * complex(env[k]*math.Cos(ph), env[k]*math.Sin(ph))
		}
		x[i] = complex64(v + complex(rng.NormFloat64()*sigma, rng.NormFloat64()*sigma))
	}
	return x
}

// TestReceiverFading receives a voice call through Rayleigh fading at a
// C/N where only the deepest fades take bursts, from a BS and from an MS,
// and checks that the coherent tiers recover C0 and C1 of nearly every
// AMBE+2 frame. The level of the receiver follows only the anchors, so a
// voice burst in a fade must not count as off.
func TestReceiverFading(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 20, 1
	bs, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	var bsWant []uint64
	for i, s := range bs.Slots {
		if f, res := s.Burst.AMBEFrames(); i%2 == 1 && !res.Uncorrectable {
			for _, v := range f {
				bsWant = append(bsWant, rftx.AMBEValue(v))
			}
		}
	}
	bsFM, bsEnv := rftx.ModulateBS(bs.Slots)
	cfg.BSSourced, cfg.Wakeups = false, 0
	ms, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	msFM, msEnv := rftx.Modulate(ms.Bursts, rftx.Schedule{Slot: 1})
	msWant := make([]uint64, 0, 18*cfg.Superframes)
	for i := range 18 * cfg.Superframes {
		msWant = append(msWant, rftx.AMBEValue(rftx.TestAMBEFrame(i)))
	}
	for _, c := range []struct {
		name    string
		fm, env []float64
		want    []uint64
		cn      float64
	}{{"bs", bsFM, bsEnv, bsWant, 25}, {"ms", msFM, msEnv, msWant, 20}} {
		for _, fd := range []float64{5, 50} {
			for _, tier := range []Tier{Tier2, Tier3} {
				var exact int
				for seed := range uint64(4) {
					x := fadedBaseband(c.fm, c.env, fd, c.cn, 10+seed)
					exact += c0c1InOrder(c.want, voiceFrames(receiveBaseband(x, tier)))
				}
				total := 4 * len(c.want)
				t.Logf("%s fade %g Hz tier %d: C0 and C1 of %d of %d frames exact", c.name, fd, int(tier-Tier0), exact, total)
				if float64(exact) < 0.98*float64(total) {
					t.Errorf("%s fade %g Hz tier %d: C0 and C1 of %d of %d frames exact, want 98%%", c.name, fd, int(tier-Tier0), exact, total)
				}
			}
		}
	}
}
