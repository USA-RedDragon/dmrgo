package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

var table615Key = [auth.KeyLen]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

func challengeAhoy(t *testing.T, group, flag bool, target, source uint32) pdu.CSBK {
	t.Helper()
	return *withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
		ServiceOptsMirror: 1, ServiceKindFlag: flag, Group: group, ServiceKind: uint8(enums.ServiceKindRegistration), TargetAddress: target, SourceAddress: source})
}

func TestMS_AuthDuringRegistration(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, Seed: 7})
	a := aloha(t)
	r.feed(a, &idle)
	wantKinds(t, "registration C_RAND", r.feed(a, &idle), trunk.OutputBurst)
	ahoy := challengeAhoy(t, false, false, msAddr, 0x7A17C0)
	out := r.feed(bsBurst(t, &ahoy, testCC), &busy)
	wantKinds(t, "challenge", out, trunk.OutputAuthChallenge)
	if out[0].Challenge != 0x7A17C0 {
		t.Fatalf("Challenge %06X", out[0].Challenge)
	}
	at := r.now
	wantKinds(t, "no random access retry", r.feed(a, &idle))
	out = r.ms.AuthResponse(r.now, auth.Response(out[0].Challenge, table615Key, 0), r.out[:0])
	wantKinds(t, "response", out, trunk.OutputBurst)
	ackIn, ok := sent(t, &out[0]).AsAckInbound()
	if !ok || enums.AckReason(ackIn.ReasonCode) != enums.AckReasonAuthResponseMS || ackIn.TargetAddress != 0xE8CA1D ||
		ackIn.AdditionalInfo != msAddr || ackIn.ResponseInfo != 0 || out[0].Due != at+60*time.Millisecond || out[0].Slot != enums.Slot1 {
		t.Fatalf("C_ACKU %v %+v due %v slot %v", ok, ackIn, out[0].Due, out[0].Slot)
	}
	wantKinds(t, "second response", r.ms.AuthResponse(r.now, 1, r.out[:0]))
	out = r.feed(ack(t, enums.AckReasonRegAccepted), &idle)
	wantKinds(t, "Reg_Accepted", out, trunk.OutputRegistered)
}

func TestMS_AuthTimeoutAndLate(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, RegTimeout: 2 * time.Second})
	a := aloha(t)
	r.feed(a, &idle)
	r.feed(a, &idle)
	ahoy := challengeAhoy(t, false, false, msAddr, 1)
	wantKinds(t, "challenge", r.feed(bsBurst(t, &ahoy, testCC), &busy), trunk.OutputAuthChallenge)
	r.now += 61 * time.Millisecond
	wantKinds(t, "late response", r.ms.AuthResponse(r.now, 5, r.out[:0]))
	at, ok := r.ms.Deadline()
	if !ok || at > r.now+2*time.Second {
		t.Fatalf("Deadline %v %v at %v", at, ok, r.now)
	}
	out := r.ms.Tick(at, r.out[:0])
	wantKinds(t, "TNP_Timer", out, trunk.OutputRefused, trunk.OutputLost)
	if out[0].Reason != trunk.ReasonTimeout || r.ms.Registered() {
		t.Errorf("timeout %+v, registered %v", out, r.ms.Registered())
	}
}

func TestMS_AuthNotChallenges(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1})
	r.feed(aloha(t), &idle)
	wantKinds(t, "no challenge", r.ms.AuthResponse(r.now, 1, r.out[:0]))
	for _, tt := range []struct {
		name         string
		group, flag  bool
		target, from uint32
		want         []trunk.OutputKind
	}{
		{"talkgroup", true, false, 9, 0x10, []trunk.OutputKind{trunk.OutputAhoy, trunk.OutputBurst}},
		{"subscription", false, true, msAddr, 0x10, []trunk.OutputKind{trunk.OutputAhoy, trunk.OutputBurst}},
		{"radio check from TSI", false, false, msAddr, constants.GatewayTSI, []trunk.OutputKind{trunk.OutputAhoy, trunk.OutputBurst}},
		{"another MS", false, false, msAddr + 1, 0x10, nil},
		{"poll", false, false, msAddr, 0xFFFCDF, []trunk.OutputKind{trunk.OutputAuthChallenge}},
	} {
		ahoy := challengeAhoy(t, tt.group, tt.flag, tt.target, tt.from)
		wantKinds(t, tt.name, r.feed(bsBurst(t, &ahoy, testCC), &busy), tt.want...)
	}
	out := r.ms.AuthResponse(r.now, 0x123456, r.out[:0])
	wantKinds(t, "poll response", out, trunk.OutputBurst)
	if ackIn, ok := sent(t, &out[0]).AsAckInbound(); !ok || ackIn.TargetAddress != 0x123456 {
		t.Errorf("C_ACKU %v %+v", ok, ackIn)
	}
}

func TestMS_AuthOnPayload(t *testing.T) {
	t.Parallel()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true, Talkgroups: [trunk.MaxTalkgroups]uint32{9}, TalkgroupsLen: 1})
	r.feed(aloha(t), &idle)
	wantKinds(t, "TV_GRANT", r.feed(tvGrant(t, 9, 5, false), &idle), trunk.OutputGrant, trunk.OutputTune)
	ahoy := challengeAhoy(t, false, false, msAddr, 0x7A17C0)
	out := r.feed(bsBurst(t, &ahoy, testCC), nil)
	wantKinds(t, "P_AHOY challenge", out, trunk.OutputAuthChallenge)
	out = r.ms.AuthResponse(r.now, auth.Response(out[0].Challenge, table615Key, 0x010203), r.out[:0])
	wantKinds(t, "P_ACKU", out, trunk.OutputBurst)
	ackP, ok := sent(t, &out[0]).AsAckInboundPayload()
	if !ok || ackP.TargetAddress != 0xE9C81E || enums.AckReason(ackP.ReasonCode) != enums.AckReasonAuthResponseMS ||
		out[0].Slot != enums.Slot1 {
		t.Errorf("P_ACKU %v %+v slot %v", ok, ackP, out[0].Slot)
	}
}

func TestTSCC_Authentication(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.Authenticate, cfg.Seed, cfg.AnswerTimeout = true, 9, time.Second
	good := [auth.KeyLen]byte{0xA5, 2}
	s := newSim(t, cfg,
		&trunk.MSConfig{Address: 3120001, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Seed: 2},
		&trunk.MSConfig{Address: 3120003, Seed: 3},
		&trunk.MSConfig{Address: 3120004, Seed: 4, DeniedLife: time.Hour},
		&trunk.MSConfig{Address: 3120005, Seed: 5, RegTimeout: 10 * time.Second})
	s.keys = map[uint32]simKey{3120001: {good, 0}, 3120002: {good, 0x0A0B0C}, 3120004: {good, 0}}
	s.ms[0].key = &simKey{good, 0}
	s.ms[1].key = &simKey{good, 0x0A0B0C}
	s.ms[2].key = &simKey{good, 0}
	s.ms[3].key = &simKey{[auth.KeyLen]byte{0xA5, 3}, 0}

	s.until(5*time.Second, func() bool { return s.ms[0].ms.Registered() && s.ms[1].ms.Registered() })
	for _, addr := range []uint32{3120001, 3120002} {
		if !s.tscc.Registered(addr) {
			t.Errorf("TSCC has no registration of %d", addr)
		}
	}
	s.until(5*time.Second, func() bool {
		n := 0
		for _, e := range s.events {
			if e.Kind == trunk.TSCCRefused {
				n++
			}
		}
		return n >= 3
	})
	refused := map[uint32]enums.AckReason{}
	challenges := map[uint32]int{}
	for _, e := range s.events {
		switch e.Kind {
		case trunk.TSCCRefused:
			if _, ok := refused[e.Address]; !ok {
				refused[e.Address] = e.Ack
			}
		case trunk.TSCCAuthChallenge:
			challenges[e.Address]++
			if e.Challenge > auth.MaxChallenge {
				t.Errorf("challenge %06X over the maximum", e.Challenge)
			}
		default:
		}
	}
	want := map[uint32]enums.AckReason{3120003: enums.AckReasonRegDenied, 3120004: enums.AckReasonRegDenied, 3120005: enums.AckReasonRegRefused}
	for addr, r := range want {
		if refused[addr] != r || s.tscc.Registered(addr) {
			t.Errorf("MS %d: refused %v, want %v; registered %v", addr, refused[addr], r, s.tscc.Registered(addr))
		}
	}
	for _, addr := range []uint32{3120001, 3120002, 3120003, 3120004, 3120005} {
		if challenges[addr] == 0 {
			t.Errorf("MS %d got no challenge", addr)
		}
	}
	if o, ok := has(s.ms[3].log, trunk.OutputRefused); !ok || o.Ack != enums.AckReasonRegDenied {
		t.Errorf("wrong key: %+v %v", o, ok)
	}
	if s.ms[2].ms.Registered() || s.ms[3].ms.Registered() || s.ms[4].ms.Registered() {
		t.Error("an MS without the right answer registered")
	}
	if trunk.TSCCAuthChallenge.String() != "AuthChallenge" || trunk.OutputAuthChallenge.String() != "AuthChallenge" {
		t.Error("names")
	}
}

func TestTSCC_AuthPSNPerMS(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.Authenticate, cfg.Seed, cfg.AnswerTimeout = true, 11, time.Second
	key := [auth.KeyLen]byte{0x5A, 7}
	const psn = 0x123456
	s := newSim(t, cfg,
		&trunk.MSConfig{Address: 3120001, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Seed: 2, DeniedLife: time.Hour},
		&trunk.MSConfig{Address: 3120003, Seed: 3, DeniedLife: time.Hour})
	s.keys = map[uint32]simKey{3120001: {key, psn}, 3120002: {key, 0}, 3120003: {key, psn}}
	s.ms[0].key = &simKey{key, psn}
	s.ms[1].key = &simKey{key, psn}
	s.ms[2].key = &simKey{key, 0}
	s.until(5*time.Second, func() bool {
		_, d1 := has(s.ms[1].log, trunk.OutputRefused)
		_, d2 := has(s.ms[2].log, trunk.OutputRefused)
		return s.ms[0].ms.Registered() && d1 && d2
	})
	for i, m := range s.ms[1:] {
		addr := uint32(3120002 + i)
		if o, _ := has(m.log, trunk.OutputRefused); o.Ack != enums.AckReasonRegDenied || m.ms.Registered() || s.tscc.Registered(addr) {
			t.Errorf("MS %d with another PSN than the TSCC: %+v, registered %v", addr, o, m.ms.Registered())
		}
	}
	if !s.tscc.Registered(3120001) {
		t.Error("the TSCC did not register the MS with its PSN")
	}
}
