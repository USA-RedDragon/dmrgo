package csbk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestTypedNilData(t *testing.T) {
	t.Parallel()

	good, nils := fuzztest.TypedNilBursts(t)
	for _, r := range good {
		for _, n := range nils {
			p, err := csbk.NewPreambles(nil)
			if err != nil {
				t.Fatal(err)
			}
			q, err := csbk.NewRequester(&csbk.RequesterConfig{Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			s, err := csbk.NewResponder(&csbk.ResponderConfig{Slot: enums.Slot1, Address: 1})
			if err != nil {
				t.Fatal(err)
			}
			w, err := csbk.NewWakeup(&csbk.WakeupConfig{Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			m, err := csbk.NewTiming(&csbk.TimingConfig{ID: 1, Slot: enums.Slot1})
			if err != nil {
				t.Fatal(err)
			}
			for _, b := range []*layer2.Burst{r, n} {
				p.Decoded(0, b, layer2.Hint{}, nil)
				q.Decoded(0, b, layer2.Hint{}, nil)
				s.Decoded(0, b, layer2.Hint{}, nil)
				w.Decoded(0, b, layer2.Hint{}, nil)
				m.Decoded(0, b, layer2.Hint{}, nil)
			}
		}
	}
}
