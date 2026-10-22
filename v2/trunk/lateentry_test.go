package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func lateGrant(t *testing.T, tg uint32, ch uint16) layer2.RawBurst {
	t.Helper()
	return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: ch, LogicalChannel: true, LateEntry: true, TargetAddress: tg, SourceAddress: 3120001}), testCC)
}

func TestFollower_LateEntrySeenWhileHunting(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{SysCode: &testSys})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	late := lateGrant(t, 9, 5)
	h := layer2.Hint{Slot: enums.Slot1}
	out = f.Burst(0, &late, h, nil, out[:0])
	wantKinds(t, "late entry grant before the site", out)
	a := aloha(t)
	out = f.Burst(slot, &a, h, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)
	out = f.Burst(500*time.Millisecond, &late, h, nil, out[:0])
	wantKinds(t, "next late entry grant", out, trunk.OutputGrant, trunk.OutputTune)
}

func TestFollower_LateEntryAfterClearHold(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{SysCode: &testSys})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	h := layer2.Hint{Slot: enums.Slot2}
	a := aloha(t)
	out = f.Burst(0, &a, h, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)
	g := tvGrant(t, 9, 5, true)
	out = f.Burst(slot, &g, h, nil, out[:0])
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	cl := pClear(t, 9)
	cleared := 2 * slot
	out = f.Burst(cleared, &cl, h, nil, out[:0])
	wantKinds(t, "P_CLEAR", out, trunk.OutputCallEnd, trunk.OutputTune)

	late := lateGrant(t, 9, 5)
	for now := cleared + slot; now < cleared+3*time.Second; now += 400 * time.Millisecond {
		out = f.Burst(now, &late, h, nil, out[:0])
		if f.State() == trunk.StatePayload {
			if now-cleared < time.Second {
				t.Fatalf("followed a late entry grant %v after P_CLEAR", now-cleared)
			}
			return
		}
	}
	t.Fatal("a late entry grant every 400 ms was never followed after the P_CLEAR hold")
}
