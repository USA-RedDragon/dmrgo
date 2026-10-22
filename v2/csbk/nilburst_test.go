package csbk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestNilBurst(t *testing.T) {
	t.Parallel()

	q := mustRequester(t, &csbk.RequesterConfig{ColorCode: 1, Source: layer2.SyncSourceMS})
	r := mustResponder(t, &csbk.ResponderConfig{Address: anytone, ColorCode: 1})
	p, err := csbk.NewPreambles(nil)
	if err != nil {
		t.Fatal(err)
	}
	w, err := csbk.NewWakeup(&csbk.WakeupConfig{ColorCode: 7})
	if err != nil {
		t.Fatal(err)
	}
	m, err := csbk.NewTiming(&csbk.TimingConfig{ID: 1, Slot: enums.Slot1})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Join(0); err != nil {
		t.Fatal(err)
	}
	for _, h := range []layer2.Hint{{}, {SyncLost: true}} {
		for name, out := range map[string][]csbk.Output{
			"Requester": q.Burst(0, nil, h, nil),
			"Responder": r.Burst(0, nil, h, nil),
			"Preambles": p.Burst(0, nil, h, nil),
			"Wakeup":    w.Burst(0, nil, h, nil),
			"Timing":    m.Burst(0, nil, h, nil),
		} {
			if len(out) != 0 {
				t.Errorf("%s %+v: %d outputs in Idle", name, h, len(out))
			}
		}
	}
}
