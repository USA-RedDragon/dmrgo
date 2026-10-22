//go:build !tinygo

package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsFollower(t *testing.T) {
	caps := readCapture(t, "testdata/t3cc.bits")
	pool := newPool(t)
	f, err := trunk.NewFollower(&trunk.FollowerConfig{Hunt: huntList(t, 1)})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	allocs := testing.AllocsPerRun(5, func() {
		f.Reset()
		out = f.Tick(0, out[:0])
		for i := range caps {
			c := &caps[i]
			out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		}
		now := 2 * time.Second
		for i := range pool {
			now += 30 * time.Millisecond
			out = f.Burst(now, &pool[0], layer2.Hint{}, nil, out[:0])
			out = f.Burst(now, &pool[i], layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
		}
		out = f.Tick(now+time.Minute, out[:0])
	})
	if allocs != 0 {
		t.Errorf("%v allocations per replay, want 0", allocs)
	}
}
