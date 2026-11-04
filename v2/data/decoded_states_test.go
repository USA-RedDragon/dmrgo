package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

func TestReceiverDecodedValidCallStates(t *testing.T) {
	t.Parallel()

	u := setHeader(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
		SAP: enums.SAPIPBasedPacketData, LLIDDestination: dstID, LLIDSource: srcID, FullMessageFlag: true})
	pkt := build(t, u, payload(40), enums.DataTypeRate12)
	voiceA, err := layer2.BuildVoiceBurst([3]vocoder.AMBEFrame{}, enums.VoiceBurstA, pdu.EmbeddedSignalling{}, [4]byte{}, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	decode := func(raw layer2.RawBurst) *layer2.Burst {
		b := new(layer2.Burst)
		if err := b.DecodeFromBytes(raw); err != nil {
			t.Fatal(err)
		}
		return b
	}
	ev := make([]data.Event, 0, data.MaxEvents)
	var all []data.Event

	r := newReceiver(t, nil)
	for i := range pkt {
		all = append(all, r.Decoded(time.Duration(i)*60*time.Millisecond, decode(pkt[i]), layer2.Hint{}, ev[:0])...)
		want := data.StateReceiving
		if i == len(pkt)-1 {
			want = data.StateCall
		}
		if r.State() != want {
			t.Fatalf("burst %d: state %v, want %v (%s)", i, r.State(), want, kinds(all))
		}
	}
	if len(all) == 0 || all[0].Kind != data.EventCallStart || all[0].Call.Source != srcID {
		t.Errorf("packet: %s", kinds(all))
	}

	r = newReceiver(t, nil)
	all = append(all[:0], r.Decoded(0, decode(pkt[0]), layer2.Hint{}, ev[:0])...)
	all = append(all, r.Decoded(60*time.Millisecond, decode(voiceA), layer2.Hint{}, ev[:0])...)
	if got := kinds(all); got != "CallStart CallEnd(Voice)" || r.State() != data.StateIdle {
		t.Errorf("voice while receiving: %s, state %v", got, r.State())
	}
}
