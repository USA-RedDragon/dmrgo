package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRxLateEntryOffsetCarrier(t *testing.T) {
	t.Parallel()

	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE"}
	built, err := rftx.Sequence("data12", cfg)
	if err != nil {
		t.Fatal(err)
	}
	freq, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	for _, off := range []float64{2000, 2500, 3000, -2000, -2500, -3000} {
		x := fmAtStart(freq, env, off, 30, 11)
		iq := make([]byte, 2*len(x))
		for i, v := range x {
			iq[2*i] = byte(max(0, min(255, math.Round(float64(real(v))*0.8*127.5+127.5))))
			iq[2*i+1] = byte(max(0, min(255, math.Round(float64(imag(v))*0.8*127.5+127.5))))
		}
		for _, tier := range []Tier{Tier1, Tier2} {
			r, err := NewReceiver(&RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCU8})
			if err != nil {
				t.Fatal(err)
			}
			got := r.AppendFlush(r.AppendBursts(nil, iq))
			if len(got) != len(built.Bursts) {
				t.Errorf("%+.0f Hz Tier%d: %d bursts, want %d", off, tier-Tier0, len(got), len(built.Bursts))
				continue
			}
			for i := range got {
				if data, _ := got[i].Decoded(); data != built.Bursts[i] {
					t.Errorf("%+.0f Hz Tier%d burst %d: decoded bits differ", off, tier-Tier0, i)
				}
			}
		}
	}
}
