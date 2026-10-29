package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRxTier0FirstBurstAtOffset(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	for _, seq := range []string{"ack", "data12", "voice"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		for _, lead := range []float64{0.2, 0.2131, 0.3377} {
			pad := int(lead * basebandRate)
			fm, env := append(make([]float64, pad), fm0...), append(make([]float64, pad), env0...)
			tx := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
			for _, off := range []float64{2500, -2500, 3000, -3000, 4000, -4000} {
				iq := rftx.Channel(tx, 2e6, 2.048e6, off, 25, 1)
				r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier0, Format: IQFormatCU8})
				if err != nil {
					t.Fatal(err)
				}
				first := false
				for _, b := range r.AppendFlush(r.AppendBursts(nil, iq)) {
					first = first || math.Abs(b.Seconds-lead-(slotSeconds-burstSeconds)/2) < 0.002
				}
				if !first {
					t.Errorf("%s after %.4f s of noise, %+.0f Hz: no first burst", seq, lead, off)
				}
			}
		}
	}
}
