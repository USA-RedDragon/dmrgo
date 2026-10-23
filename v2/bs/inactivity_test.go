package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestInactivityDeadlineMoves(t *testing.T) {
	t.Parallel()
	in := call(t, 1, 9, 3191868, 2, enums.Slot1, rf, 0)
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true, Inactivity: 20 * time.Millisecond})
	out := make([]bs.Output, 0, bs.MaxOutputs)
	dekeyed := false
	for now := time.Duration(0); now < 6*time.Second; now += time.Millisecond {
		for len(in) > 0 && in[0].at <= now {
			out = r.Inbound(now, &in[0].raw, layer2.Hint{Slot: in[0].slot}, in[0].from, out[:0])
			in = in[1:]
		}
		for n := 0; ; n++ {
			at, ok := r.Deadline()
			if !ok || at > now {
				break
			}
			if n > 100 {
				t.Fatalf("Deadline %v at %v does not move with Tick", at, now)
			}
			out = r.Tick(now, out[:0])
			dekeyed = dekeyed || len(filter(out, bs.OutputDekey)) > 0
		}
	}
	if !dekeyed {
		t.Fatal("no Dekey after the call hang time")
	}
}
