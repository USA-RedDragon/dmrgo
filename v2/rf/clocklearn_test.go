package rf

import (
	"math"
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func clockRunErrors(t *testing.T, cn, ppm float64, seeds int) [8]int {
	t.Helper()
	var errs [8]int
	for _, e := range clockRuns(t, testConfig(), cn, ppm, seeds) {
		for i := range errs {
			errs[i] += e[i]
		}
	}
	return errs
}

func clockRuns(t *testing.T, cfg rftx.SeqConfig, cn, ppm float64, seeds int) [][8]int {
	t.Helper()
	cfg.Superframes = 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env := append(make([]float64, pad), fm0...), append(make([]float64, pad), env0...)
	tx := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	runs := make([][8]int, seeds)
	for seed := range seeds {
		errs := &runs[seed]
		iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, 0, cn-22.1, uint64(seed+1))
		r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier1, Format: IQFormatCU8})
		if err != nil {
			t.Fatal(err)
		}
		got := r.AppendFlush(r.AppendBursts(nil, iq))
		for i := 0; i < len(got) && i < len(errs); i++ {
			best := 1 << 30
			for _, w := range built.Bursts {
				n := 0
				for k := range w {
					n += bits.OnesCount8(got[i].Data[k] ^ w[k])
				}
				best = min(best, n)
			}
			errs[i] += best
		}
	}
	return runs
}

func TestRxClockLearnLowCN(t *testing.T) {
	t.Parallel()
	for _, ppm := range []float64{500, -500} {
		errs := clockRunErrors(t, 12, ppm, 16)
		late := 0
		for _, e := range errs[5:] {
			late += e
		}
		if late > 25 {
			t.Errorf("12 dB %+.0f ppm: %d raw bit errors in bursts 5 to 7 over 16 seeds, want at most 25 (%v)", ppm, late, errs)
		}
	}
}

// TestRxClockLearnPayloads checks that an MS call at 12 dB with a -500 ppm
// clock decodes its late bursts for other source IDs than the one of
// TestRxClockLearnLowCN: how fast the clock rate is learned depends on the
// bits of the call, and a call whose rate is not learned in time drifts off
// its symbols. It fails when more than 1 of the 32 runs has more than 10 raw
// bit errors in bursts 5 to 7.
func TestRxClockLearnPayloads(t *testing.T) {
	t.Parallel()
	bad := 0
	for _, src := range []uint32{3009104, 3009105} {
		cfg := testConfig()
		cfg.Source = src
		for _, e := range clockRuns(t, cfg, 12, -500, 16) {
			if e[5]+e[6]+e[7] > 10 {
				bad++
			}
		}
	}
	if bad > 1 {
		t.Errorf("%d of 32 runs with more than 10 raw bit errors in bursts 5 to 7", bad)
	}
}

// TestRxClockFirstStep checks that the first clock rate step after an MS
// or a BS lock reads the true clock error at +-500 ppm.
// It fails when the mean ratio of the step to the true rate over the seeds
// is more than pairedZ standard errors outside 0.85 to 1.15.
func TestRxClockFirstStep(t *testing.T) {
	t.Parallel()
	const seeds = 8
	cfg := testConfig()
	cfg.Superframes = 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	slots, _ := benchStream(t, 1)
	pad := int(0.2 * basebandRate)
	for _, bs := range []bool{false, true} {
		fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		if bs {
			fm, env = rftx.ModulateBS(slots)
		}
		n := min(len(fm), int(0.3*basebandRate))
		tx := rftx.ToIQ(append(make([]float64, pad), fm[:n]...), append(make([]float64, pad), env[:n]...), 2e6, 0, 0.9)
		for _, tier := range []Tier{Tier0, Tier2} {
			for _, ppm := range []float64{500, -500} {
				ratio := make([]float64, seeds)
				forSeeds(seeds, func(i int) {
					iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, 1500, 30-22.1, uint64(i+1))
					r := mustNew(RxConfig{Rate: 2.048e6, Tier: tier, Format: IQFormatCU8})
					d := r.host().iqd.d
					for k := 0; k < len(iq) && ratio[i] == 0; k += 4096 {
						r.AppendBursts(nil, iq[k:min(k+4096, len(iq))])
						ratio[i] = float64(d.tm.rate/(2*d.tm.step)) / (ppm * 1e-6)
					}
				})
				mean, se := meanSE(ratio)
				if math.Abs(mean-1)-pairedZ*se > 0.15 {
					t.Errorf("%s, Tier%d, %+.0f ppm: first rate step %.3f of the clock error, SE %.3f", map[bool]string{false: "MS", true: "BS"}[bs], tier-Tier0, ppm, mean, se)
				}
			}
		}
	}
}
