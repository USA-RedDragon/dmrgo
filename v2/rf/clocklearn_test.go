package rf

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func clockRunErrors(t *testing.T, cn, ppm float64, seeds int) [8]int {
	t.Helper()
	cfg := testConfig()
	cfg.Superframes = 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env := append(make([]float64, pad), fm0...), append(make([]float64, pad), env0...)
	tx := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	var errs [8]int
	for seed := range seeds {
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
	return errs
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
