//go:build !tinygo

package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsTSCC(t *testing.T) {
	pool := newInboundPool(t)
	ts, err := trunk.NewTSCC(tsccConfig())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	now := time.Duration(0)
	allocs := testing.AllocsPerRun(5, func() {
		for i := range pool {
			out = ts.Inbound(now, tsccChannel, &pool[i], layer2.Hint{Slot: enums.Slot1}, out[:0])
			out = ts.Inbound(now, 10, &pool[i], layer2.Hint{Slot: enums.Slot1}, out[:0])
			for range 8 {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
			}
		}
		for range 300 {
			now += 30 * time.Millisecond
			out = ts.Tick(now, out[:0])
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}
