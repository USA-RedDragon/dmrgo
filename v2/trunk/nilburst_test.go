package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestNilBurst(t *testing.T) {
	t.Parallel()

	plan := trunk.ChannelPlan{}
	if err := plan.AddRule(trunk.ChannelRule{First: 1, Last: 100, RXHz: 451_000_000, StepHz: 12_500, TXOffsetHz: 5_000_000}); err != nil {
		t.Fatal(err)
	}
	f, err := trunk.NewFollower(&trunk.FollowerConfig{SysCode: &testSys, Plan: &plan, Hunt: huntList(t, 1)})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	now := time.Duration(0)
	out = f.Tick(now, out[:0])
	a := aloha(t)
	out = f.Burst(now, &a, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	now += slot
	g := tvGrant(t, 9, 5, true)
	out = f.Burst(now, &g, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	now += slot
	out = f.Burst(now, nil, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "nil burst", out)
	if f.State() != trunk.StatePayload {
		t.Fatalf("State = %v after a nil burst, want Payload", f.State())
	}
	out = f.Burst(now, nil, layer2.Hint{Slot: enums.Slot2, SyncLost: true}, nil, out[:0])
	wantKinds(t, "nil burst with SyncLost", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonInactive {
		t.Errorf("Reason = %v, want Inactive", out[0].Reason)
	}

	cp, err := trunk.NewCapacityPlusFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := trunk.NewMS(&trunk.MSConfig{Address: msAddr})
	if err != nil {
		t.Fatal(err)
	}
	tscc, err := trunk.NewTSCC(tsccConfig())
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range []layer2.Hint{{}, {SyncLost: true}} {
		_ = cp.Burst(0, nil, h, nil, out[:0])
		_ = ms.Burst(0, nil, h, nil, out[:0])
		var cach [3]byte
		_ = cp.Decoded(0, nil, h, &cach, out[:0])
		_ = ms.Decoded(0, nil, h, &cach, out[:0])
		_ = f.Decoded(0, nil, h, &cach, out[:0])
		var zf trunk.Follower
		var zm trunk.MS
		var zc trunk.CapacityPlusFollower
		_ = zf.Decoded(0, nil, h, &cach, out[:0])
		_ = zm.Decoded(0, nil, h, &cach, out[:0])
		_ = zc.Decoded(0, nil, h, &cach, out[:0])
		h.Slot = enums.Slot1
		if o := tscc.Inbound(0, 1, nil, h, nil); len(o) != 0 {
			t.Errorf("TSCC.Inbound of nil: %d outputs", len(o))
		}
	}
}
