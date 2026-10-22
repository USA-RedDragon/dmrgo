package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestTSCC_SlotClock(t *testing.T) {
	t.Parallel()
	ms := time.Millisecond
	for _, lead := range []time.Duration{0, 10 * ms} {
		cfg := tsccConfig()
		cfg.Lead = lead
		ts, err := trunk.NewTSCC(cfg)
		if err != nil {
			t.Fatal(err)
		}
		out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
		check := func(now time.Duration, wantDue time.Duration, wantSlot enums.Slot, wantSkipped int64) {
			t.Helper()
			out = ts.Tick(now, out[:0])
			n := 0
			for i := range out {
				if out[i].Kind != trunk.TSCCBurst {
					continue
				}
				n++
				if out[i].Due != wantDue || out[i].Slot != wantSlot {
					t.Errorf("lead %v Tick(%v): due %v slot %v, want %v %v", lead, now, out[i].Due, out[i].Slot, wantDue, wantSlot)
				}
			}
			if wantDue < 0 && n != 0 || wantDue >= 0 && n != 1 {
				t.Errorf("lead %v Tick(%v): %d bursts", lead, now, n)
			}
			if ts.Skipped() != wantSkipped {
				t.Errorf("lead %v Tick(%v): Skipped %d, want %d", lead, now, ts.Skipped(), wantSkipped)
			}
		}
		check(5*time.Second, 5*time.Second, enums.Slot1, 0)
		check(5*time.Second, -1, 0, 0)
		if at, ok := ts.Deadline(); !ok || at != 5*time.Second+30*ms-lead {
			t.Errorf("lead %v: Deadline %v %v", lead, at, ok)
		}
		check(5*time.Second+30*ms-lead, 5*time.Second+30*ms, enums.Slot2, 0)
		check(5*time.Second+90*ms-lead, 5*time.Second+90*ms, enums.Slot2, 1)
		check(5*time.Second+90*ms, -1, 0, 1)
		check(time.Hour+5*time.Second-lead, time.Hour+5*time.Second, enums.Slot1, 119997)
	}
}
