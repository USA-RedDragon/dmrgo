package data_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestSenderResyncsAfterLostMessage(t *testing.T) {
	t.Parallel()
	tx := newSender(t, &data.SenderConfig{Preambles: -1, Source: layer2.SyncSourceMS, Retries: 2})
	rx := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
	out := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	var queue []data.Event
	var done []data.Output
	var got [][]byte
	now := time.Duration(0)
	for i := range 3 {
		m := message(t, 30+i)
		m.Header = sapHeader(t, enums.SAPShortData, 0, false)
		if err := tx.Send(now, m); err != nil {
			t.Fatal(err)
		}
		for end := now + time.Minute; tx.State() != data.SenderIdle && now < end; now += 10 * time.Millisecond {
			for _, e := range queue {
				done = append(done, tx.Burst(now, &e.Burst, layer2.Hint{}, out[:0])...)
			}
			queue = queue[:0]
			var bursts []data.Output
			for _, o := range tx.Tick(now, out[:0]) {
				if o.Kind == data.OutputDone {
					done = append(done, o)
				} else if i != 1 {
					bursts = append(bursts, o)
				}
			}
			handle := func(evs []data.Event) {
				for _, e := range evs {
					switch e.Kind {
					case data.EventDelivered:
						got = append(got, bytes.Clone(e.Message.Payload))
					case data.EventRespond:
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
	if len(done) != 3 || done[0].Result != data.ResultDelivered || done[1].Result != data.ResultRetryLimit || done[2].Result != data.ResultDelivered {
		rs := make([]data.Result, 0, len(done))
		for _, o := range done {
			rs = append(rs, o.Result)
		}
		t.Fatalf("results %v", rs)
	}
	if len(got) != 2 || !bytes.Equal(got[1], payload(32)) {
		t.Fatalf("delivered %x", got)
	}
}
