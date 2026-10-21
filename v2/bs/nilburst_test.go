package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestNilBurst(t *testing.T) {
	t.Parallel()

	for _, from := range []bs.Source{rf, {Peer: 7}} {
		r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
		in := call(t, 1, 9, 3191868, 2, enums.Slot1, from, 0)
		last := in[len(in)-2].at
		got := run(t, r, in[:len(in)-1], last)
		if len(filter(got, bs.OutputCallStart)) != 1 {
			t.Fatalf("%+v: call did not start", from)
		}
		out := r.Inbound(last+10*time.Millisecond, nil, layer2.Hint{Slot: enums.Slot1}, from, nil)
		if len(filter(out, bs.OutputCallEnd)) != 0 {
			t.Fatalf("%+v: nil burst without a hint: %d outputs", from, len(out))
		}
		out = r.Inbound(last+20*time.Millisecond, nil, layer2.Hint{Slot: enums.Slot1, SyncLost: true}, from, nil)
		if len(filter(out, bs.OutputCallEnd)) != 1 {
			t.Errorf("%+v: nil burst with SyncLost: %d call ends", from, len(filter(out, bs.OutputCallEnd)))
		}
	}
}
