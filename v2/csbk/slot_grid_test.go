package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// The answer to a request that starts at t0 on a logical channel goes in
// the next slot of that channel, a TDMA frame later: the next but one
// slot of a direct channel (ETSI TS 102 361-1 clause 5.2.1.7), and the
// inbound slot of the channel on the aligned timing of a BS, which starts
// with its outbound slot (ETSI TS 102 361-1 clause 5.1.1, Table 5.1 and
// Figure 5.23).
func onAnswerGrid(t0, due time.Duration) bool {
	return due >= t0+csbk.BurstInterval && (due-t0)%csbk.BurstInterval == 0
}

func requestAt(t *testing.T, t0 time.Duration) layer2.RawBurst {
	t.Helper()
	q := mustRequester(t, &csbk.RequesterConfig{ColorCode: 1, Source: layer2.SyncSourceMS})
	if err := q.RequestOACSU(t0, 1002, 1001, layer3.ServiceOptions{}, nil); err != nil {
		t.Fatal(err)
	}
	out := q.Tick(t0, nil)
	if len(out) != 1 || out[0].Due != t0 {
		t.Fatalf("request %+v", out)
	}
	return out[0].Burst
}

func TestResponder_AnswerSlotGrid(t *testing.T) {
	t.Parallel()
	t0 := time.Second
	for _, tt := range []struct {
		lead time.Duration
		want time.Duration
	}{
		{0, t0 + 60*ms},
		{10 * ms, t0 + 60*ms},
		{60 * ms, t0 + 60*ms},
		{70 * ms, t0 + 120*ms},
	} {
		r := mustResponder(t, &csbk.ResponderConfig{Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS, Lead: tt.lead})
		req := requestAt(t, t0)
		r.Burst(t0, &req, layer2.Hint{}, nil)
		at, ok := r.Deadline()
		if !ok || at != tt.want-tt.lead {
			t.Errorf("lead %v: Deadline %v %v, want %v", tt.lead, at, ok, tt.want-tt.lead)
			continue
		}
		if out := r.Tick(at-1, nil); len(out) != 0 {
			t.Errorf("lead %v: Tick before Deadline gave %+v", tt.lead, out)
		}
		out := r.Tick(at, nil)
		if len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != tt.want || !onAnswerGrid(t0, out[0].Due) {
			t.Errorf("lead %v: answer to the request at %v: %+v, want due %v", tt.lead, t0, out, tt.want)
		}
	}
}

func TestResponder_PoliteAnswerSlotGrid(t *testing.T) {
	t.Parallel()
	t0 := time.Second
	r := mustResponder(t, &csbk.ResponderConfig{Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS})
	r.Channel(0, true)
	req := requestAt(t, t0)
	r.Burst(t0, &req, layer2.Hint{}, nil)
	r.Channel(t0+75*ms, false)
	at, ok := r.Deadline()
	out := r.Tick(at, nil)
	if !ok || len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != t0+120*ms {
		t.Errorf("answer after the channel went idle at %v: %+v, want due %v", t0+75*ms, out, t0+120*ms)
	}

	for seed := range uint64(8) {
		r := mustResponder(t, &csbk.ResponderConfig{Address: 1002, ColorCode: 1, Source: layer2.SyncSourceMS, OACSU: csbk.OACSUNotSupported, Seed: seed})
		r.Channel(0, true)
		r.Burst(t0, &req, layer2.Hint{}, nil)
		var due time.Duration
		for range 8 {
			at, ok := r.Deadline()
			if !ok {
				break
			}
			r.Channel(at, false)
			for _, o := range r.Tick(at, nil) {
				if o.Kind == csbk.OutputBurst {
					due = o.Due
				}
			}
			if due != 0 {
				break
			}
		}
		if !onAnswerGrid(t0, due) {
			t.Errorf("seed %d: NACK_Rsp after a holdoff due %v, not a TDMA frame after the request at %v", seed, due, t0)
		}
	}
}
