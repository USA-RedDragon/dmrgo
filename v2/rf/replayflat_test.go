package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestRxNoBurstsFromCarrier(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Message: "DMRGO TEST MESSAGE", Superframes: 1, LateEntry: true}
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	const lead = 2.0
	n := int(lead * basebandRate)
	fm = append(make([]float64, n), fm...)
	env = append(make([]float64, n), env...)
	ramp := int((slotSeconds - burstSeconds) / 2 * basebandRate)
	for i := range n {
		env[i] = 1
		if i < ramp {
			env[i] = 0.5 - 0.5*math.Cos(math.Pi*float64(i)/float64(ramp))
		}
	}
	for i := n; i < len(env) && env[i] < 1; i++ {
		env[i] = 1
	}
	fm, env = fm[ramp/2:], env[ramp/2:]
	at := lead - float64(ramp/2)/basebandRate
	for _, off := range []float64{-2500, 2500} {
		iq := rftx.Channel(rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, basebandRate, off, 60, 5)
		for _, tier := range []Tier{Tier1, Tier2} {
			r, err := NewReceiver(&RxConfig{Rate: basebandRate, Tier: tier, Format: IQFormatCU8})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range r.AppendFlush(r.AppendBursts(nil, iq)) {
				if b.Seconds < at-0.001 {
					t.Errorf("%+.0f Hz Tier%d: burst at %.4f s inside the carrier", off, tier-Tier0, b.Seconds)
				}
			}
		}
	}
}
