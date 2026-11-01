package rf

import (
	"fmt"
	"testing"
)

func TestTxDecodesEveryBit(t *testing.T) {
	t.Parallel()

	for _, src := range []struct {
		name   string
		bursts []Burst
	}{
		{"ms", msBursts(t, "voice")},
		{"bs", bsBursts(t, "voice")},
	} {
		for _, tier := range []Tier{Tier0, Tier1, Tier2} {
			tx, err := newTransmitter(usIQ(tier, 96000, IQFormatCS16), openGate)
			if err != nil {
				t.Fatal(err)
			}
			iq := tx.AppendFlush(tx.AppendSamples(nil, src.bursts))
			rx, err := NewReceiver(&RxConfig{Tier: Tier1, Rate: 96000, Format: IQFormatCS16})
			if err != nil {
				t.Fatal(err)
			}
			got := rx.AppendFlush(rx.AppendBursts(nil, iq))
			checkDecoded(t, fmt.Sprintf("%s I/Q Tier%d", src.name, tier-1), got, src.bursts)
		}
	}
}

func checkDecoded(t *testing.T, name string, got, want []Burst) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: %d bursts, want %d", name, len(got), len(want))
		return
	}
	for i := range want {
		if got[i].Data != want[i].Data {
			t.Errorf("%s: burst %d of %d: bits differ", name, i, len(want))
		}
	}
}
