package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func decodedInput(t *testing.T, in input) input {
	t.Helper()
	b := new(layer2.Burst)
	if err := b.DecodeFromBytes(in.raw); err != nil {
		t.Fatal(err)
	}
	in.decoded = b
	return in
}

func TestReceiver_DecodedValidCallStates(t *testing.T) {
	t.Parallel()

	bursts := transmission(t, groupLC(t, 91, 1001), 1, nil)
	term := decodedInput(t, input{raw: bursts[len(bursts)-1]})
	rx := newReceiver(t, nil)
	got := feed(t, rx, []input{term}, -1)
	if k := kinds(got); k != "s" || rx.State() != voice.StateHang || got[0].Call.Target != 91 {
		t.Errorf("terminator in idle: events %s, state %v", k, rx.State())
	}

	idle, err := layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, 1, layer2.SyncSourceBS)
	if err != nil {
		t.Fatal(err)
	}
	in := append(timed(bursts[:4], 0, layer2.Hint{}), decodedInput(t, input{at: 4 * slot, raw: idle}))
	rx = newReceiver(t, nil)
	got = feed(t, rx, in, -1)
	ends := find(got, voice.EventCallEnd)
	if len(ends) != 1 || ends[0].Reason != voice.EndIdle || ends[0].Call.Target != 91 || rx.State() != voice.StateIdle {
		t.Errorf("idle in a call: events %s, state %v", kinds(got), rx.State())
	}
}
