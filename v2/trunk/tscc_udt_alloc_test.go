//go:build !tinygo

package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsTSCCUDT(t *testing.T) {
	cfg := tsccConfig()
	cfg.Reg = false
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	pool := newTSCCUDTPool(t)
	var a trunk.Appended
	if err := a.SetAddresses(true, []uint32{501}); err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs)
	now := time.Duration(0)
	run := func() {
		for i := range pool {
			out = ts.Inbound(now, tsccChannel, enums.Slot1, &pool[i], layer2.Hint{}, out[:0])
			for range 4 {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
			}
		}
		_ = ts.SendShortData(now, false, 3120002, &a)
		_ = ts.PollShortData(now, 3120002, 1)
		_ = ts.PollUSBD(now, 3120002, pdu.ServiceTypeLIP, 0)
		for range 400 {
			now += 30 * time.Millisecond
			out = ts.Tick(now, out[:0])
		}
	}
	run()
	if allocs := testing.AllocsPerRun(5, run); allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}
