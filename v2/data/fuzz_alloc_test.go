//go:build !tinygo

package data_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/data"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
)

func rxCorpus(t *testing.T) []rxInput {
	t.Helper()
	var out []rxInput
	var in rxInput
	fuzztest.Corpus(t, "FuzzReceiver", func() { out = append(out, in) }, &in.c0, &in.c1, &in.step, &in.ops)
	return out
}

func txCorpus(t *testing.T) []txInput {
	t.Helper()
	var out []txInput
	var in txInput
	fuzztest.Corpus(t, "FuzzSender", func() { out = append(out, in) }, &in.c0, &in.c1, &in.c2, &in.step, &in.ops)
	return out
}

func TestAllocsReceiverCorpus(t *testing.T) {
	p := pool(t)
	ev := make([]data.Event, 0, data.MaxEvents)
	for i, in := range append(rxSeeds(), rxCorpus(t)...) {
		s := parseRxScript(in, p)
		r, err := data.NewReceiver(&s.cfg)
		if err != nil {
			continue
		}
		allocs := testing.AllocsPerRun(3, func() {
			r.Reset()
			for j := range s.ops {
				ev = runRxOp(r, &s.ops[j], ev[:0])
			}
		})
		if allocs != 0 {
			t.Errorf("input %d: %v allocations", i, allocs)
		}
	}
}
