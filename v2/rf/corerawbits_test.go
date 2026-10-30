//go:build !tinygo

package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestCoreTier3RawBitsAtLock(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	for _, seq := range []string{"ack", "data12", "voice"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		for _, off := range []float64{1500, 3000, -3000, 4000} {
			iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, off, 25, 5)
			r, err := newReceiver(RxConfig{Rate: 2.048e6, Tier: Tier3, Format: IQFormatCU8}, ptr(coreIQ3Options()))
			if err != nil {
				t.Fatal(err)
			}
			got := r.AppendFlush(r.AppendBursts(nil, iq))
			if len(got) != len(built.Bursts) {
				t.Errorf("%s %+.0f Hz: %d bursts, want %d", seq, off, len(got), len(built.Bursts))
				continue
			}
			for i := range got {
				if got[i].Data != built.Bursts[i] {
					t.Errorf("%s %+.0f Hz burst %d: raw bits differ", seq, off, i)
				}
			}
		}
	}
}
