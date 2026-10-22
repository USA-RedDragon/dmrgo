package bs_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/bs"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestRejectedCall(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{Mode: bs.ModeNetwork, ColorCode: 1})
	a := call(t, 1, 9, 3191868, 2, enums.Slot1, bs.Source{Peer: 1}, 0)
	b := call(t, 1, 91, 3120000, 1, enums.Slot1, bs.Source{Peer: 2}, 10*time.Millisecond)
	c := call(t, 1, 92, 3120001, 1, enums.Slot1, bs.Source{Peer: 3}, 3*time.Second)
	rej := filter(run(t, r, merge(merge(a, b), c), 5*time.Second), bs.OutputRejected)
	want := []struct {
		peer   uint32
		reason bs.Reason
		target uint32
		source uint32
	}{{2, bs.ReasonSlotBusy, 91, 3120000}, {3, bs.ReasonHangTime, 92, 3120001}}
	if len(rej) != len(want) {
		t.Fatalf("rejections %v", rej)
	}
	for i, w := range want {
		o := rej[i]
		if o.From.Peer != w.peer || o.Reason != w.reason {
			t.Fatalf("rejection %d: %v", i, o)
		}
		if !o.Call.Group || o.Call.Target != w.target || o.Call.Source != w.source || o.Call.Origin != voice.OriginHeader || o.Call.FID != enums.FIDStandardized {
			t.Errorf("rejection %d: call %+v", i, o.Call)
		}
	}
}

func TestHangTimeRF(t *testing.T) {
	t.Parallel()
	r := newRepeater(t, &bs.Config{ColorCode: 1, CACH: true})
	a := call(t, 1, 9, 3191868, 1, enums.Slot1, rf, 0)
	b := call(t, 1, 92, 3120001, 1, enums.Slot1, rf, time.Second)
	got := run(t, r, merge(a, b), 4*time.Second)
	starts := filter(got, bs.OutputCallStart)
	if len(starts) != 2 || starts[1].Call.Target != 92 {
		t.Fatalf("call starts %v", starts)
	}
	if rej := filter(got, bs.OutputRejected); len(rej) != 0 {
		t.Fatalf("rejections %v", rej)
	}
}
