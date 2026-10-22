package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
)

// ETSI TS 102 361-1 clause 5.2.2.3 and ETSI TS 102 361-2 clause 5.1.2.2
func TestPoliteBusyHoldsOff(t *testing.T) {
	t.Parallel()
	for seed := range uint64(20) {
		q := mustRequester(t, &csbk.RequesterConfig{Seed: seed})
		q.Channel(0, true)
		if err := q.RequestCallAlert(0, 2, 1, nil); err != nil {
			t.Fatal(err)
		}
		end, ok := q.Deadline()
		if !ok || end <= 0 || end > csbk.DefaultHoldoffMax+1 {
			t.Fatalf("seed %d: holdoff Deadline %v %v", seed, end, ok)
		}
		idle := end / 2
		q.Channel(idle, false)
		if out := q.Tick(idle, nil); len(out) != 0 {
			t.Fatalf("seed %d: sent at %v when the busy channel went idle, before the holdoff ends at %v", seed, idle, end)
		}
		if out := q.Tick(end, nil); len(out) != 1 || out[0].Kind != csbk.OutputBurst || out[0].Due != end {
			t.Errorf("seed %d: after the holdoff %+v", seed, out)
		}
	}
}

func TestRequesterBusyDeadlineAdvances(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name         string
		idle, hold   time.Duration
		timeCritical bool
	}{
		{name: "no wait, no holdoff", idle: -1, hold: -1},
		{name: "1 ns holdoff", idle: -1, hold: 1},
		{name: "default", idle: 0, hold: 0},
		{name: "no wait, time critical", idle: -1, hold: -1, timeCritical: true},
	} {
		q := mustRequester(t, &csbk.RequesterConfig{IdleSearch: c.idle, HoldoffMax: c.hold, Seed: 3})
		q.Channel(0, true)
		if err := q.RequestCallAlert(time.Second, 2, 1, &csbk.RequestOptions{TimeCritical: c.timeCritical}); err != nil {
			t.Fatal(err)
		}
		for _, now := range []time.Duration{2 * time.Second, time.Hour} {
			q.Tick(now, nil)
			if at, ok := q.Deadline(); ok && at <= now {
				t.Errorf("%s: Deadline %v after a Tick at %v", c.name, at, now)
			}
		}
	}
}
