package voice_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
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

func TestTransmitter_NilBurst(t *testing.T) {
	t.Parallel()

	tx := authTransmitter(t, voice.TransmitterConfig{})
	if out := tx.Burst(time.Second+time.Millisecond, nil, layer2.Hint{Slot: enums.Slot1}, nil); len(out) != 0 || tx.State() != voice.TransmitterChanAuth {
		t.Fatalf("nil burst: %+v, state %v", out, tx.State())
	}
	out := tx.Burst(time.Second+constants.TMSChanAuth, nil, layer2.Hint{}, nil)
	if len(out) != 1 || out[0].Kind != voice.OutputEnded || out[0].Reason != voice.EndNoOutbound {
		t.Errorf("nil bursts count as a busy channel: %+v", out)
	}
}

func TestInterrupter_NilBurst(t *testing.T) {
	t.Parallel()

	i, err := voice.NewInterrupter(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := i.Interrupt(0, enums.RCCeaseTransmissionRequest); err != nil {
		t.Fatal(err)
	}
	if out := i.Burst(0, nil, layer2.Hint{}, nil); len(out) != 1 || out[0].Kind != voice.OutputBurst {
		t.Errorf("nil burst does not run the timers: %+v", out)
	}
}
