package rf

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// specriseWakeup is the spacing of the wakeup bursts of the sequences.
const specriseWakeup = 2 * slotSeconds

// TestRxTier0FirstBurstAtOffset checks that Tier 0 hands out a first burst
// of a call that starts 2.5 to 4 kHz off after a noise lead, and that this
// burst is one its own coding vouches for. The tune needs at least 3 FFT
// frames and Tier 0 keeps no samples to replay, so the first wakeup can be
// lost; the second must then come out.
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
				got := r.AppendFlush(r.AppendBursts(nil, iq))
				start := lead + (slotSeconds-burstSeconds)/2
				var c leanCore
				switch {
				case len(got) == 0:
					t.Errorf("%s after %.4f s of noise, %+.0f Hz: no burst", seq, lead, off)
				case !c.vouched(&got[0]):
					t.Errorf("%s after %.4f s of noise, %+.0f Hz: the first burst, at %.4f s, is not vouched for by its coding", seq, lead, off, got[0].Seconds)
				case got[0].Seconds > start+specriseWakeup+0.002:
					t.Errorf("%s after %.4f s of noise, %+.0f Hz: first burst at %.4f s, want by %.4f s", seq, lead, off, got[0].Seconds, start+specriseWakeup)
				}
			}
		}
	}
}
