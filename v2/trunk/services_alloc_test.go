//go:build !tinygo

package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestAllocsTSCCServices(t *testing.T) {
	pool := newTSCCSvcPool(t)
	cfg := tsccConfig()
	cfg.PowerSave, cfg.IP, cfg.Supplementary, cfg.TransmitInterrupt, cfg.MaxCall, cfg.Auth = true, true, true, true, time.Second, xorAuth(1)
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.TSCCOutput, 0, ts.MaxOutputs())
	now := time.Duration(0)
	allocs := testing.AllocsPerRun(5, func() {
		_ = ts.Stun(now, 3120002, false)
		_ = ts.Kill(now, 3120003)
		_ = ts.SendStatus(now, false, 3120002, 0xFFFECB, 3)
		_ = ts.MassRegister(2, 0, 0)
		for i := range pool {
			out = ts.Inbound(now, tsccChannel, enums.Slot1, &pool[i], layer2.Hint{}, out[:0])
			out = ts.Inbound(now, 10, enums.Slot1, &pool[i], layer2.Hint{}, out[:0])
			_ = ts.Interrupt(now, 10, enums.Slot1, enums.RCCeaseTransmissionRequest)
			for range 8 {
				now += 30 * time.Millisecond
				out = ts.Tick(now, out[:0])
			}
		}
		for range 400 {
			now += 30 * time.Millisecond
			out = ts.Tick(now, out[:0])
		}
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}

func TestAllocsMSServices(t *testing.T) {
	pool := newMSSvcPool(t)
	ms, err := trunk.NewMS(&trunk.MSConfig{Address: msAddr, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1, PowerSave: 2, Auth: xorAuth(1)})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	data := []byte("supplementary")
	now := time.Duration(0)
	var cach [3]byte
	allocs := testing.AllocsPerRun(5, func() {
		ms.Reset()
		for i := range pool {
			now += 30 * time.Millisecond
			out = ms.Burst(now, &pool[i], layer2.Hint{Slot: enums.Slot1}, &cach, out[:0])
			_ = ms.SendStatus(now, false, 3120002, 3)
			_ = ms.PollStatus(now, 3120002)
			_ = ms.SetSupplementary(1, data)
			_ = ms.SetStatus(4)
			_ = ms.AdviseIP(now, [16]byte{1}, false, false)
			out, _ = ms.Interrupt(now, false, out[:0])
			out = ms.Tick(now, out[:0])
		}
		out = ms.Tick(now+time.Minute, out[:0])
	})
	if allocs != 0 {
		t.Errorf("%v allocations per run, want 0", allocs)
	}
}
