package trunk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func registered(t *testing.T, cfg *trunk.MSConfig) *msRig {
	t.Helper()
	r := newRig(t, cfg)
	a := aloha(t)
	r.feed(a, &idle)
	wantKinds(t, "registration C_RAND", r.feed(a, &idle), trunk.OutputBurst)
	wantKinds(t, "Reg_Accepted", r.feed(ack(t, enums.AckReasonRegAccepted), &idle), trunk.OutputRegistered)
	return r
}

func TestMS_PowerOff(t *testing.T) {
	t.Parallel()
	r := registered(t, &trunk.MSConfig{Address: msAddr})
	r.out = r.ms.PowerOff(r.now, r.out[:0])
	wantKinds(t, "PowerOff", r.out)
	out := r.feed(aloha(t), &idle)
	wantKinds(t, "de-registration C_RAND", out, trunk.OutputBurst)
	if ra, ok := sent(t, &out[0]).AsRandomAccess(); !ok || ra.ServiceKind != uint8(enums.ServiceKindRegistration) || ra.ServiceOptions != 0 {
		t.Fatalf("de-registration %v %+v", ok, ra)
	}
	wantKinds(t, "Reg_Accepted", r.feed(ack(t, enums.AckReasonRegAccepted), &idle), trunk.OutputPowerOff)
	if !r.ms.Off() || r.ms.Registered() {
		t.Errorf("Off %v Registered %v", r.ms.Off(), r.ms.Registered())
	}
	wantKinds(t, "burst after PowerOff", r.feed(aloha(t), &idle))

	r2 := registered(t, &trunk.MSConfig{Address: msAddr})
	r2.out = r2.ms.PowerOff(r2.now, r2.out[:0])
	at, ok := r2.ms.Deadline()
	if !ok || at != r2.now+constants.TDeregDefault {
		t.Fatalf("Deadline %v %v", at, ok)
	}
	out = r2.ms.Tick(at, r2.out[:0])
	wantKinds(t, "T_Dereg", out, trunk.OutputPowerOff)
	if out[0].Reason != trunk.ReasonTimeout {
		t.Errorf("reason %v", out[0].Reason)
	}

	r3 := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	r3.feed(aloha(t), &idle)
	wantKinds(t, "PowerOff unregistered", r3.ms.PowerOff(r3.now, r3.out[:0]), trunk.OutputPowerOff)
}

func TestMS_RegDeniedAndRefused(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, DeniedLife: time.Minute})
	a := aloha(t)
	r.feed(a, &idle)
	if err := r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{}); !errors.Is(err, trunk.ErrBusy) {
		t.Errorf("Call while registering = %v", err)
	}
	r.feed(a, &idle)
	out := r.feed(ack(t, enums.AckReasonRegDenied), &idle)
	wantKinds(t, "Reg_Denied", out, trunk.OutputRefused, trunk.OutputLost)
	if out[0].Ack != enums.AckReasonRegDenied || out[1].Reason != trunk.ReasonDenied || r.ms.State() != trunk.StateHunting {
		t.Errorf("Reg_Denied %+v, state %v", out, r.ms.State())
	}
	out = r.feed(a, &idle)
	wantKinds(t, "denied TSCC", out, trunk.OutputSite, trunk.OutputLost)
	if out[1].Reason != trunk.ReasonDenied {
		t.Errorf("reason %v", out[1].Reason)
	}
	r.now += time.Minute
	wantKinds(t, "after T_DENREG", r.feed(a, &idle), trunk.OutputSite)

	r2 := newRig(t, &trunk.MSConfig{Address: msAddr})
	r2.feed(a, &idle)
	r2.feed(a, &idle)
	out = r2.feed(ack(t, enums.AckReasonRegRefused), &idle)
	wantKinds(t, "Reg_Refused", out, trunk.OutputRefused, trunk.OutputLost)
	wantKinds(t, "TSCC again", r2.feed(a, &idle), trunk.OutputSite)
	wantKinds(t, "registration again", r2.feed(a, &idle), trunk.OutputBurst)
}

func TestMS_RadioCheck(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1})
	r.feed(aloha(t), &idle)
	for _, tt := range []struct {
		group  bool
		target uint32
		answer bool
	}{{false, msAddr, true}, {true, 9, true}, {true, 10, false}, {false, msAddr + 1, false}} {
		ahoy := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
			ServiceKind: uint8(enums.ServiceKindRegistration), Group: tt.group, TargetAddress: tt.target, SourceAddress: constants.GatewayALLMSI}), testCC)
		out := r.feed(ahoy, &busy)
		if !tt.answer {
			wantKinds(t, "radio check for another", out)
			continue
		}
		wantKinds(t, "radio check", out, trunk.OutputAhoy, trunk.OutputBurst)
		ackIn, ok := sent(t, &out[1]).AsAckInbound()
		if !ok || ackIn.TargetAddress != constants.GatewayALLMSI || ackIn.AdditionalInfo != msAddr {
			t.Errorf("C_ACKU %v %+v", ok, ackIn)
		}
	}
}

func TestMS_Move(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	a := aloha(t)
	r.feed(a, &idle)
	other := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetMove, pdu.MovePDU{Mask: 24, PhysicalChannel: 40, MSAddress: msAddr + 1}), testCC)
	wantKinds(t, "C_MOVE for another MS", r.feed(other, &idle))
	mv := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetMove, pdu.MovePDU{Mask: 24, PhysicalChannel: 40, MSAddress: msAddr}), testCC)
	out := r.feed(mv, &idle)
	wantKinds(t, "C_MOVE", out, trunk.OutputLost, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonMove || out[1].Tune.Channel != 40 || r.ms.State() != trunk.StateHunting {
		t.Errorf("C_MOVE %+v, state %v", out, r.ms.State())
	}
	wantKinds(t, "TSCC on channel 40", r.feed(a, &idle), trunk.OutputSite)
	if s := r.ms.Site(); s.Channel != 40 {
		t.Errorf("Site.Channel %d", s.Channel)
	}
}

func TestMS_PayloadProtectAndAhoy(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1})
	r.feed(aloha(t), &idle)
	wantKinds(t, "TV_GRANT", r.feed(tvGrant(t, 9, 5, false), &idle), trunk.OutputGrant, trunk.OutputTune)
	dis := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetProtect, pdu.ProtectPDU{
		ProtectKind: uint8(enums.ProtectDisablePTT), Group: true, TargetAddress: 9, SourceAddress: 3120001}), testCC)
	out := r.feed(dis, nil)
	wantKinds(t, "DIS_PTT", out, trunk.OutputProtect)
	if out[0].Protect != enums.ProtectDisablePTT {
		t.Errorf("Protect %v", out[0].Protect)
	}
	pa := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
		ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: msAddr, SourceAddress: constants.GatewayALLMSI}), testCC)
	out = r.feed(pa, nil)
	wantKinds(t, "P_AHOY", out, trunk.OutputAhoy, trunk.OutputBurst)
	if ackP, ok := sent(t, &out[1]).AsAckInboundPayload(); !ok || out[1].Slot != enums.Slot1 {
		t.Errorf("P_ACKU %v %+v slot %v", ok, ackP, out[1].Slot)
	}
	parked := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetProtect, pdu.ProtectPDU{
		ProtectKind: uint8(enums.ProtectIllegallyParked), Group: true, TargetAddress: 9, SourceAddress: 3120005}), testCC)
	out = r.feed(parked, nil)
	wantKinds(t, "ILLEGALLY_PARKED", out, trunk.OutputCallEnd, trunk.OutputTune)
	if out[0].Reason != trunk.ReasonParked {
		t.Errorf("reason %v", out[0].Reason)
	}
}

func TestMS_DataCallAndEmergency(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	regOnly := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAloha, pdu.AlohaPDU{
		ServiceFunc: uint8(enums.ServiceFunctionRegistration), NRandWait: 1, Backoff: 2, SysIdentCode: sysIdent(t)}), testCC)
	r.feed(regOnly, &idle)
	if err := r.ms.CallData(r.now, false, 3120002, layer3.DataServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		wantKinds(t, "data request while registration only", r.feed(regOnly, &idle))
	}
	var out []trunk.Output
	for range 10 {
		out = r.feed(aloha(t), &idle)
		if len(out) > 0 && out[len(out)-1].Kind == trunk.OutputBurst {
			out = out[len(out)-1:]
			break
		}
	}
	wantKinds(t, "data C_RAND after an inviting C_ALOHA", out, trunk.OutputBurst)
	if ra, ok := sent(t, &out[0]).AsRandomAccess(); !ok || ra.ServiceKind != uint8(enums.ServiceKindIndividualPacketData) {
		t.Fatalf("C_RAND %v %+v", ok, ra)
	}
	pd := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetPrivateDataGrant, pdu.PrivateDataGrantPDU{
		PhysicalChannel: 7, TargetAddress: 3120002, SourceAddress: msAddr}), testCC)
	wantKinds(t, "PD_GRANT", r.feed(pd, &idle), trunk.OutputGrant, trunk.OutputTune)
	if r.ms.Busy() {
		t.Error("busy after the grant")
	}

	e := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	e.feed(regOnly, &idle)
	if err := e.ms.Call(e.now, true, 9, layer3.VoiceServiceOptions{IsEmergency: true}); err != nil {
		t.Fatal(err)
	}
	out = e.feed(regOnly, &idle)
	wantKinds(t, "emergency C_RAND ignores Service_Function", out, trunk.OutputBurst)
	if ra, ok := sent(t, &out[0]).AsRandomAccess(); !ok || ra.ServiceOptions&0x40 == 0 {
		t.Errorf("emergency options %v %07b", ok, ra.ServiceOptions)
	}
}

func TestMS_Quality(t *testing.T) {
	t.Parallel()
	hunt, idleLevel := -100, -110
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, MinLevelHunt: &hunt, MinLevelIdle: &idleLevel})
	r.out = r.ms.Quality(r.now, -105, r.out[:0])
	a := aloha(t)
	out := r.feed(a, &idle)
	wantKinds(t, "weak TSCC", out, trunk.OutputSite, trunk.OutputLost)
	r.out = r.ms.Quality(r.now, -90, r.out[:0])
	wantKinds(t, "strong TSCC", r.feed(a, &idle), trunk.OutputSite)
	out = r.ms.Quality(r.now, -105, r.out[:0])
	wantKinds(t, "above the idle threshold", out)
	out = r.ms.Quality(r.now, -115, r.out[:0])
	wantKinds(t, "below the idle threshold", out, trunk.OutputLost)
	if out[0].Reason != trunk.ReasonQuality {
		t.Errorf("reason %v", out[0].Reason)
	}

	b := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, MaxBERPermille: 1})
	b.feed(a, &idle)
	noisy := a
	noisy[5] ^= 0x81
	noisy[20] ^= 0x10
	var lost bool
	for range 40 {
		for _, o := range b.feed(noisy, &idle) {
			lost = lost || o.Kind == trunk.OutputLost && o.Reason == trunk.ReasonQuality
		}
	}
	if !lost {
		t.Error("no OutputLost for a TSCC with a high bit error rate")
	}
}

func TestMS_RandomTimeout(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, RandomTimeout: 100 * time.Millisecond})
	a := aloha(t)
	r.feed(a, &idle)
	_ = r.ms.Call(r.now, true, 9, layer3.VoiceServiceOptions{})
	wantKinds(t, "C_RAND", r.feed(a, &idle), trunk.OutputBurst)
	wantKinds(t, "NWait frame 1, after TRand_TC", r.feed(a, &idle))
	wantKinds(t, "NWait frame 2", r.feed(a, &idle))
	out := r.feed(a, &idle)
	wantKinds(t, "end of NWait", out, trunk.OutputRefused, trunk.OutputLost)
	if out[0].Reason != trunk.ReasonNoAnswer {
		t.Errorf("reason %v", out[0].Reason)
	}

	q := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, RandomTimeout: 100 * time.Millisecond})
	q.feed(a, &idle)
	_ = q.ms.Call(q.now, true, 9, layer3.VoiceServiceOptions{})
	at, ok := q.ms.Deadline()
	if !ok || at != q.now+100*time.Millisecond {
		t.Fatalf("Deadline %v %v", at, ok)
	}
	wantKinds(t, "TRand_TC before any C_RAND", q.ms.Tick(at, q.out[:0]), trunk.OutputRefused)
	if q.ms.State() != trunk.StateControl {
		t.Errorf("state %v", q.ms.State())
	}
}
