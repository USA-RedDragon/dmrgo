package data_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
)

func TestSenderIgnoresStaleNACKOutOfSequence(t *testing.T) {
	t.Parallel()
	tx := newSender(t, &data.SenderConfig{Preambles: -1, Source: layer2.SyncSourceMS, Retries: 2})
	rx := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
	out := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	var queue []data.Event
	var done []data.Output
	var got [][]byte
	var stale []layer2.RawBurst
	injected := false
	now := time.Duration(0)
	for i := range 4 {
		m := message(t, 30+i)
		m.Header = sapHeader(t, enums.SAPShortData, 0, false)
		if err := tx.Send(now, m); err != nil {
			t.Fatal(err)
		}
		for end := now + time.Minute; tx.State() != data.SenderIdle && now < end; now += 10 * time.Millisecond {
			if i == 3 && !injected && tx.State() == data.SenderWaitResponse {
				for k := range stale {
					done = append(done, tx.Burst(now, &stale[k], layer2.Hint{}, out[:0])...)
				}
				injected = true
				continue
			}
			for _, e := range queue {
				done = append(done, tx.Burst(now, &e.Burst, layer2.Hint{}, out[:0])...)
			}
			queue = queue[:0]
			var bursts []data.Output
			for _, o := range tx.Tick(now, out[:0]) {
				if o.Kind == data.OutputDone {
					done = append(done, o)
				} else if i != 1 && (i != 3 || injected) {
					bursts = append(bursts, o)
				}
			}
			handle := func(evs []data.Event) {
				for _, e := range evs {
					switch e.Kind {
					case data.EventDelivered:
						got = append(got, bytes.Clone(e.Message.Payload))
					case data.EventRespond:
						if i == 2 && e.Response == packet.ResponseNACKOutOfSequence && len(stale) < 1 {
							stale = append(stale, e.Burst)
						}
						queue = append(queue, e)
					case data.EventCallStart, data.EventResponse, data.EventCallEnd, data.EventReassembled:
					}
				}
			}
			for _, o := range bursts {
				handle(rx.Burst(o.Due, &o.Burst, layer2.Hint{}, ev[:0]))
			}
			handle(rx.Tick(now, ev[:0]))
		}
	}
	rs := make([]data.Result, 0, len(done))
	for _, o := range done {
		rs = append(rs, o.Result)
	}
	if len(stale) != 1 || !injected {
		t.Fatalf("stale NACKs %d, injected %v", len(stale), injected)
	}
	if len(done) != 4 || done[3].Result != data.ResultDelivered || len(got) != 3 || !bytes.Equal(got[2], payload(33)) {
		t.Fatalf("results %v, %d delivered", rs, len(got))
	}
}
