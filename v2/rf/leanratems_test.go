//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

// TestLeanClockRateNullMS checks that a Tier 2 MS receiver learns no clock
// rate on a 0 ppm signal: a re-entry refit moves the timing by more than the
// loop's own corrections do, so its shift must count in the spread of the
// rate estimate as well as in its sum. The seeds are the ones of 768 that
// learned a false rate before. It fails when a stream applies a rate over
// the bound of TestLeanClockRateNull.
func TestLeanClockRateNullMS(t *testing.T) {
	t.Parallel()
	d := directShared(t)
	const sigmaS2 = 0.0062
	a := 1 - leanClockGain*slotSymbols
	bound := 5 * leanClockGain * math.Sqrt(sigmaS2/(1-a*a)) / leanMMSPS
	for _, c := range []struct {
		cn   float64
		seed uint64
	}{{0, 20012}, {2, 20079}, {8, 20108}} {
		r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier2, Format: IQFormatCU8})
		dd := r.host().iqd.d
		x := noisyFM(d.fm, d.env, 1400, c.cn, c.seed)
		worst := 0.0
		for k := 0; k < len(x); k += 2048 {
			must(r.writeBB(x[k:min(k+2048, len(x))]))
			if v := math.Abs(float64(dd.tm.rate / (2 * dd.tm.step))); dd.locked {
				worst = max(worst, v)
			}
		}
		if worst > bound {
			t.Errorf("%v dB seed %d: clock rate %.1e, bound %.1e, true rate 0", c.cn, c.seed, worst, bound)
		}
	}
}

// TestRateLineNullMS checks that the Tier 1 timing line of an MS stream steps
// no clock rate from the noise of its SYNC fits on a 0 ppm signal. The line
// weighs each fit by its measured variance, leanRefitVar*(1-c2)/c2; with a
// fixed fit spread under that variance, two fits one frame apart passed the
// 5 sigma test. The cases are ones that stepped a false rate before. It fails
// when a stream applies a rate over the bound of TestLeanClockRateNull.
func TestRateLineNullMS(t *testing.T) {
	t.Parallel()
	const sigmaS2 = 0.0062
	a := 1 - leanClockGain*slotSymbols
	bound := 5 * leanClockGain * math.Sqrt(sigmaS2/(1-a*a)) / leanMMSPS
	fm, env, _ := msStream(t, 3, 0)
	for _, c := range []struct {
		cn    float64
		seed  uint64
		audio bool
	}{{6, 105, true}, {8, 119, true}, {8, 108, false}, {10, 123, false}} {
		x := noisyFM(fm, env, 1400, c.cn, c.seed)
		var core *leanCore
		var write func(k int) int
		if c.audio {
			s := discAudio(x, benchAudioHz)
			r, err := NewAudioReceiver(&AudioRxConfig{Rate: benchAudioHz, Tier: Tier1, OnBursts: each(func(*Burst) {})})
			if err != nil {
				t.Fatal(err)
			}
			core = &r.leanCore
			write = func(k int) int {
				if k >= len(s) {
					return 0
				}
				must(r.writeAll(s[k:min(k+1024, len(s))]))
				return 1024
			}
		} else {
			r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier1, Format: IQFormatCU8})
			core = &r.host().iqd.d.leanCore
			write = func(k int) int {
				if k >= len(x) {
					return 0
				}
				must(r.writeBB(x[k:min(k+2048, len(x))]))
				return 2048
			}
		}
		worst := 0.0
		for k, n := 0, write(0); n > 0; n = write(k) {
			k += n
			if v := math.Abs(float64(core.tm.rate / (2 * core.tm.step))); core.locked {
				worst = max(worst, v)
			}
		}
		if worst > bound {
			t.Errorf("%v dB seed %d audio %v: clock rate %.1e, bound %.1e, true rate 0", c.cn, c.seed, c.audio, worst, bound)
		}
	}
}
