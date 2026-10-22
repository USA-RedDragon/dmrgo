package csbk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/csbk"
)

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
