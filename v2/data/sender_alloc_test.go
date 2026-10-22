//go:build !tinygo

package data_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestAllocsSenderLoopback(t *testing.T) {
	tx := newSender(t, &data.SenderConfig{Preambles: 2, Source: layer2.SyncSourceMS, ColorCode: 1})
	rx := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
	m := message(t, 60)
	out := make([]data.Output, 0, data.MaxOutputs)
	done := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	var queue [8]data.Event
	tries := 0
	allocs := testing.AllocsPerRun(5, func() {
		tx.Reset()
		rx.Reset()
		if tx.Send(0, m) != nil {
			t.Fatal("Send")
		}
		n := 0
		for now := time.Duration(0); now < 2*time.Second && tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
			for i := range n {
				done = tx.Burst(queue[i].Due, &queue[i].Burst, layer2.Hint{}, done[:0])
				if len(done) > 0 {
					tries = int(done[0].Try)
				}
			}
			n = 0
			ev = rx.Tick(now, ev[:0])
			out = tx.Tick(now, out[:0])
			for i := range out {
				if out[i].Kind != data.OutputBurst || out[i].Try == 1 && out[i].Due == 5*60*time.Millisecond {
					continue
				}
				ev = rx.Burst(out[i].Due, &out[i].Burst, layer2.Hint{}, ev[:0])
				for j := range ev {
					if ev[j].Kind == data.EventRespond && n < len(queue) {
						queue[n] = ev[j]
						n++
					}
				}
			}
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations", allocs)
	}
	if tx.State() != data.SenderIdle || tries != 2 {
		t.Errorf("state %v after %d tries", tx.State(), tries)
	}
}

func TestAllocsSenderWindow(t *testing.T) {
	tx := newSender(t, &data.SenderConfig{Window: 3})
	rx := newReceiver(t, &data.ReceiverConfig{Respond: true, Address: dstID})
	msgs := windowMessages(t, 3, 60)
	out := make([]data.Output, 0, data.MaxOutputs)
	done := make([]data.Output, 0, data.MaxOutputs)
	ev := make([]data.Event, 0, data.MaxEvents)
	var queue [8]data.Event
	delivered := 0
	allocs := testing.AllocsPerRun(5, func() {
		tx.Reset()
		rx.Reset()
		for _, m := range msgs {
			if tx.Send(0, m) != nil {
				t.Fatal("Send")
			}
		}
		n, sent := 0, 0
		delivered = 0
		for now := time.Duration(0); now < 4*time.Second && tx.State() != data.SenderIdle; now += 10 * time.Millisecond {
			for i := range n {
				done = tx.Burst(queue[i].Due, &queue[i].Burst, layer2.Hint{}, done[:0])
				delivered += len(done)
			}
			n = 0
			ev = rx.Tick(now, ev[:0])
			out = tx.Tick(now, out[:0])
			for i := range out {
				if out[i].Kind != data.OutputBurst {
					continue
				}
				sent++
				if sent == 12 {
					continue
				}
				ev = rx.Burst(out[i].Due, &out[i].Burst, layer2.Hint{}, ev[:0])
				for j := range ev {
					if ev[j].Kind == data.EventRespond && n < len(queue) {
						queue[n] = ev[j]
						n++
					}
				}
			}
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations", allocs)
	}
	if tx.State() != data.SenderIdle || delivered != 3 {
		t.Errorf("state %v after %d messages", tx.State(), delivered)
	}
}

func TestAllocsSenderCorpus(t *testing.T) {
	p, msgs := txPool(t), txMessages(t)
	for i, in := range append(txSeeds(), corpusFiles(t, "FuzzSender")...) {
		base := testing.AllocsPerRun(3, func() { checkTxScript(t, in[:min(len(in), 3)], p, msgs) })
		allocs := testing.AllocsPerRun(3, func() { checkTxScript(t, in, p, msgs) })
		if allocs > base {
			t.Errorf("input %d: %v allocations, %v of them in NewSender", i, allocs, base)
		}
	}
}
