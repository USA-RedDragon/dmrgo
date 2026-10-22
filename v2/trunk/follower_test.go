package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestFollower_RealTSCC(t *testing.T) {
	t.Parallel()
	caps := readCapture(t, "testdata/t3cc.bits")
	if len(caps) < 40 {
		t.Fatalf("%d bursts in the capture, want at least 40", len(caps))
	}
	f, err := trunk.NewFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	var sites, others int
	for i := range caps {
		c := &caps[i]
		out = f.Burst(c.at, &c.raw, c.hint, c.cach, out[:0])
		for j := range out {
			switch out[j].Kind {
			case trunk.OutputSite:
				sites++
			default:
				others++
			}
		}
	}
	if sites != 1 || others != 0 {
		t.Errorf("%d OutputSite and %d other outputs, want 1 and 0", sites, others)
	}
	if f.State() != trunk.StateControl {
		t.Fatalf("State = %v, want Control", f.State())
	}
	s := f.Site()
	want := pdu.SysCode{Model: pdu.NetworkModelSmall, Net: 0, Site: 5, Par: 1}
	if !s.HasSysCode || s.SysCode != want || s.ColorCode != 0 || !s.Reg || s.Version != 3 || !s.ActiveConn {
		t.Errorf("Site = %+v", s)
	}
	if !s.HasAccess || s.Access.NRandWait.Frames() != 24 || s.Access.Backoff.Frames() != 1 || s.Access.Function != enums.ServiceFunctionAll || s.Access.Mask != 0 {
		t.Errorf("Access = %+v", s.Access)
	}
	if s.SlotCounter < 276 || s.SlotCounter > 285 {
		t.Errorf("SlotCounter = %d, want 276 to 285", s.SlotCounter)
	}

	end := caps[len(caps)-1].at
	if at, ok := f.Deadline(); !ok || at <= end || at > end+constants.TNoSignalDefault {
		t.Errorf("Deadline = %v, %v after the last burst at %v", at, ok, end)
	}
	out = f.Tick(end+constants.TNoSignalDefault, out[:0])
	wantKinds(t, "T_Nosig", out, trunk.OutputLost)
	if out[0].Reason != trunk.ReasonNoSignal || f.State() != trunk.StateHunting {
		t.Errorf("Tick after T_Nosig: %+v, state %v", out[0], f.State())
	}
}

func TestFollower_GrantClear(t *testing.T) {
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
	wantKinds(t, "start", out, trunk.OutputTune)
	if tn := out[0].Tune; tn.Channel != 1 || tn.RXHz != 451_000_000 || tn.TXHz != 456_000_000 {
		t.Errorf("hunt Tune = %+v", tn)
	}

	a := aloha(t)
	out = f.Burst(now, &a, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)

	now += slot
	g := tvGrant(t, 9, 5, true)
	out = f.Burst(now, &g, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	gr := out[0].Grant
	if gr.Target != 9 || gr.Source != 3120001 || !gr.Group || gr.Channel != 5 || gr.Slot != enums.Slot2 || gr.RXHz != 451_050_000 || gr.Service != enums.ServiceKindTalkgroupVoice {
		t.Errorf("Grant = %+v", gr)
	}
	if c := gr.Call(); !c.Group || c.Target != 9 || c.Source != 3120001 {
		t.Errorf("Grant.Call = %+v", c)
	}
	if tn := out[1].Tune; tn.Channel != 5 || tn.Slot != enums.Slot2 || tn.RXHz != 451_050_000 {
		t.Errorf("payload Tune = %+v", tn)
	}
	if f.State() != trunk.StatePayload {
		t.Fatalf("State = %v, want Payload", f.State())
	}

	now += slot
	other := pClear(t, 77)
	out = f.Burst(now, &other, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "P_CLEAR on the other slot", out)

	now += 2 * time.Second
	cl := pClear(t, 9)
	out = f.Burst(now, &cl, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "P_CLEAR", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonClear || out[0].Grant.Target != 9 || out[1].Tune.Channel != 1 || out[1].Tune.ColorCode != testCC {
		t.Errorf("P_CLEAR outputs %+v", out)
	}

	now += slot
	late := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 5, LogicalChannel: true, LateEntry: true, TargetAddress: 9, SourceAddress: 3120001}), testCC)
	out = f.Burst(now, &late, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "late entry TV_GRANT of the cleared call", out, trunk.OutputGrant)
	if f.State() != trunk.StateControl {
		t.Fatalf("State after a late entry grant of the cleared call = %v, want Control", f.State())
	}

	now += slot
	out = f.Burst(now, &g, layer2.Hint{Slot: enums.Slot2}, nil, out[:0])
	wantKinds(t, "new TV_GRANT of the same parties after P_CLEAR", out, trunk.OutputGrant, trunk.OutputTune)
	if f.State() != trunk.StatePayload || out[1].Tune.Channel != 5 {
		t.Errorf("new set-up grant after P_CLEAR: state %v, outputs %+v", f.State(), out)
	}
}

func TestFollower_HangAndInactive(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{Hangtime: 2 * time.Second, Inactive: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	a := aloha(t)
	out = f.Burst(0, &a, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)

	g := tvGrant(t, 9, 5, false)
	out = f.Burst(slot, &g, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)

	term := terminator(t, 9)
	out = f.Burst(time.Second, &term, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "terminator", out)
	if at, ok := f.Deadline(); !ok || at != 3*time.Second {
		t.Errorf("Deadline = %v, %v, want 3s", at, ok)
	}
	out = f.Tick(3*time.Second, out[:0])
	wantKinds(t, "hang time", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonHangtime || out[1].Tune.Channel != trunk.ChannelNull {
		t.Errorf("hang time outputs %+v", out)
	}

	g2 := tvGrant(t, 10, 6, false)
	out = f.Burst(4*time.Second, &g2, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "second TV_GRANT", out, trunk.OutputGrant, trunk.OutputTune)
	out = f.Tick(7*time.Second, out[:0])
	wantKinds(t, "TV_Inactive", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonInactive {
		t.Errorf("TV_Inactive reason %v", out[0].Reason)
	}
}

func TestFollower_Filters(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	a := aloha(t)
	out = f.Burst(0, &a, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)

	g := tvGrant(t, 10, 5, false)
	out = f.Burst(slot, &g, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "TV_GRANT to another talkgroup", out, trunk.OutputGrant)

	pv := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetPrivateVoiceGrant, pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 4, TargetAddress: 3120002, SourceAddress: 3120001}), testCC)
	out = f.Burst(2*slot, &pv, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "PV_GRANT", out, trunk.OutputGrant)
	if out[0].Grant.Group || out[0].Grant.Service != enums.ServiceKindIndividualVoice {
		t.Errorf("PV_GRANT %+v", out[0].Grant)
	}

	wrongCC := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetTalkgroupVoiceGrant, pdu.TalkgroupVoiceGrantPDU{
		PhysicalChannel: 5, TargetAddress: 9, SourceAddress: 1}), testCC+1)
	out = f.Burst(3*slot, &wrongCC, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "TV_GRANT with another color code", out)

	g9 := tvGrant(t, 9, 5, false)
	out = f.Burst(4*slot, &g9, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "TV_GRANT to talkgroup 9", out, trunk.OutputGrant, trunk.OutputTune)
}

func TestFollower_MBCGrant(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(nil)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	a := aloha(t)
	out = f.Burst(0, &a, layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)

	cdef := layer3.CdefParms{Channel: 42, TXMHz: 456, TXKHz: 100, RXMHz: 451, RXKHz: 100}
	blocks := mbcGrant(t, 9, cdef, 5)
	out = f.Burst(slot, &blocks[0], layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "MBC header", out)
	out = f.Burst(3*slot, &blocks[1], layer2.Hint{Slot: enums.Slot1}, nil, out[:0])
	wantKinds(t, "CG_AP", out, trunk.OutputGrant, trunk.OutputTune)
	tn := out[1].Tune
	if tn.Channel != 42 || tn.Slot != enums.Slot2 || tn.RXHz != 451_012_500 || tn.TXHz != 456_012_500 || !tn.HasColorCode || tn.ColorCode != 5 {
		t.Errorf("Tune = %+v", tn)
	}
	plan := f.Plan()
	if ch, ok := plan.Lookup(42); !ok || ch.RXHz != 451_012_500 {
		t.Errorf("plan channel 42 = %+v, %v", ch, ok)
	}
	if ch, ok := f.Lookup(42); !ok || ch.RXHz != 451_012_500 {
		t.Errorf("Lookup(42) = %+v, %v", ch, ok)
	}
	if _, ok := f.Lookup(43); ok {
		t.Error("Lookup(43) found a channel")
	}
}

func TestFollower_Hunt(t *testing.T) {
	t.Parallel()
	f, err := trunk.NewFollower(&trunk.FollowerConfig{Hunt: huntList(t, 1, 2), NoSignal: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	out := make([]trunk.Output, 0, trunk.MaxOutputs)
	out = f.Tick(0, out[:0])
	wantKinds(t, "start", out, trunk.OutputTune)
	out = f.Tick(time.Second, out[:0])
	wantKinds(t, "dwell", out, trunk.OutputTune)
	if out[0].Tune.Channel != 2 {
		t.Errorf("second hunt Tune %+v", out[0].Tune)
	}
	a := aloha(t)
	out = f.Burst(1500*time.Millisecond, &a, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "C_ALOHA", out, trunk.OutputSite)
	if s := f.Site(); s.Channel != 2 {
		t.Errorf("Site.Channel = %d, want 2", s.Channel)
	}

	ann := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetBroadcast, pdu.BroadcastPDU{
		AnnouncementType: enums.AnnouncementAnnWDTSCC, BroadcastParms1: 0b0000_0111_0000_01,
		SysIdentCode: sysIdent(t), BroadcastParms2: 7<<12 | 1}), testCC)
	out = f.Burst(1600*time.Millisecond, &ann, layer2.Hint{}, nil, out[:0])
	wantKinds(t, "Ann_WD_TSCC", out)
	h := f.Hunt()
	if h.Len() != 2 || h.At(0).Channel != 2 || h.At(1) != (trunk.HuntEntry{Channel: 7, ColorCode: 7, HasColorCode: true}) {
		t.Errorf("hunt list after Ann_WD_TSCC: %+v %+v", h.At(0), h.At(1))
	}
}
