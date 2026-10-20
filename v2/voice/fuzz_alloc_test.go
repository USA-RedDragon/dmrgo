//go:build !tinygo

package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

func TestAllocsReceiverCorpus(t *testing.T) {
	p := newPool(t)
	events := make([]voice.Event, 0, voice.MaxEvents)
	events2 := make([]voice.Event, 0, voice.MaxEvents)
	outs := make([]voice.Output, 0, voice.MaxOutputs)
	scratch := layer2.NewBurst()
	for i, in := range append(seeds(p), corpusFiles(t, "FuzzReceiver")...) {
		s := parseScript(in, p)
		if s.badCfg {
			continue
		}
		rx := newReceiver(t, &s.cfg)
		rx2 := newReceiver(t, nil)
		tx, err := voice.NewTransmitter(&s.txCfg)
		if err != nil {
			t.Fatal(err)
		}
		allocs := testing.AllocsPerRun(3, func() {
			rx.Reset()
			rx2.Reset()
			tx.Reset()
			for j := range s.ops {
				op := &s.ops[j]
				events = runOp(rx, scratch, op, events[:0])
				for k := range events {
					_ = tx.Relay(op.at, &events[k])
				}
				for {
					at, ok := tx.Deadline()
					if !ok || at > op.at {
						break
					}
					outs = tx.Tick(at, outs[:0])
					for k := range outs {
						if outs[k].Kind == voice.OutputBurst {
							events2 = rx2.Burst(outs[k].Due, &outs[k].Burst, layer2.Hint{}, events2[:0])
						}
					}
				}
			}
		})
		if allocs != 0 {
			t.Errorf("input %d: %v allocations", i, allocs)
		}
	}
}
