package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRxVoiceSlowClock(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE", Superframes: 4}
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env = append(make([]float64, pad), fm...), append(make([]float64, pad), env...)
	tx := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	for _, off := range []float64{0, 3000} {
		for _, ppm := range []float64{500, -500} {
			iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, off, 25, 5)
			for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
				if tier == Tier0 && off != 0 {
					continue
				}
				r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: tier, Format: IQFormatCU8})
				if err != nil {
					t.Fatal(err)
				}
				got := r.AppendFlush(r.AppendBursts(nil, iq))
				if len(got) != len(built.Bursts) {
					t.Errorf("%+.0f Hz %+.0f ppm Tier%d: %d bursts, want %d", off, ppm, tier-Tier0, len(got), len(built.Bursts))
					continue
				}
				bad := 0
				for i := range got {
					if got[i].Data != built.Bursts[i] {
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("%+.0f Hz %+.0f ppm Tier%d: raw bits differ in %d of %d bursts", off, ppm, tier-Tier0, bad, len(got))
				}
			}
		}
	}
}
