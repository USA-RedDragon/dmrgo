package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestReceiver_NilBurst(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		hint layer2.Hint
		want voice.EndReason
	}{
		{layer2.Hint{SyncLost: true}, voice.EndSyncLost},
		{layer2.Hint{End: layer2.EndStrong}, voice.EndTransport},
	} {
		rx := newReceiver(t, nil)
		in := timed(transmission(t, groupLC(t, 91, 3191868), 2, nil), 0, layer2.Hint{})
		in = in[:len(in)-1]
		feed(t, rx, in, -1)
		at := in[len(in)-1].at + slot
		if ev := rx.Burst(at, nil, layer2.Hint{}, nil); len(ev) != 0 || rx.State() != voice.StateInCall {
			t.Fatalf("nil burst without a hint: %s, state %v", kinds(ev), rx.State())
		}
		ev := rx.Burst(at, nil, c.hint, nil)
		if kinds(ev) != "E" || ev[0].Reason != c.want {
			t.Errorf("%+v: %s, want an end with %v", c.hint, kinds(ev), c.want)
		}
	}
	rx := newReceiver(t, nil)
	if ev := rx.Decoded(0, nil, layer2.Hint{SyncLost: true}, nil); len(ev) != 0 {
		t.Errorf("Decoded of nil in Idle: %s", kinds(ev))
	}
}
