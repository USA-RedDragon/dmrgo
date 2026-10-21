package trunk_test

import (
	"errors"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

type xorAuth uint32

func (k xorAuth) Response(addr, challenge uint32) (uint32, bool) {
	if k == 0 {
		return 0, false
	}
	return (challenge ^ uint32(k) ^ addr) & 0xFFFFFF, true
}

func countKind(log []trunk.Output, kind trunk.OutputKind) int {
	n := 0
	for _, o := range log {
		if o.Kind == kind {
			n++
		}
	}
	return n
}

func events(s *sim, kind trunk.TSCCOutputKind) []trunk.TSCCOutput {
	var out []trunk.TSCCOutput
	for _, e := range s.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

func registeredSim(t *testing.T, cfg *trunk.TSCCConfig, ms ...*trunk.MSConfig) *sim {
	t.Helper()
	s := newSim(t, cfg, ms...)
	s.until(5*time.Second, func() bool {
		for _, m := range s.ms {
			if !m.ms.Registered() && !m.ms.Off() {
				return false
			}
		}
		return true
	})
	s.events = nil
	for _, m := range s.ms {
		m.log = nil
	}
	return s
}

func threeMS() []*trunk.MSConfig {
	tg := [trunk.MaxTalkgroups]uint32{9}
	return []*trunk.MSConfig{
		{Address: 3120001, Talkgroups: tg, TalkgroupsLen: 1, Seed: 1},
		{Address: 3120002, Talkgroups: tg, TalkgroupsLen: 1, Seed: 2},
		{Address: 3120003, Seed: 3},
	}
}

func TestStatus_DeliveryAndPoll(t *testing.T) {
	t.Parallel()
	s := registeredSim(t, tsccConfig(), threeMS()...)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]

	if err := a.ms.SendStatus(s.now, false, 3120003, 42); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputAccepted); return ok })
	st, ok := has(c.log, trunk.OutputStatus)
	if !ok || st.Status != 42 || st.Grant.Source != 3120001 || st.Grant.Target != 3120003 || st.Grant.Group {
		t.Errorf("C status %+v %v", st, ok)
	}
	if acc, _ := has(a.log, trunk.OutputAccepted); acc.Ack != enums.ReasonMSAccepted {
		t.Errorf("A accepted %+v", acc)
	}
	if e, ok := s.event(trunk.TSCCStatus); !ok || e.Status != 42 || e.Ack != enums.ReasonMSAccepted || e.Call.Target != 3120003 || e.Call.Source != 3120001 {
		t.Errorf("TSCCStatus %+v %v", e, ok)
	}

	a.log, b.log, c.log, s.events = nil, nil, nil, nil
	if err := a.ms.SendStatus(s.now, true, 9, trunk.StatusEmergency); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputAccepted); return ok })
	s.run(time.Second)
	if n := countKind(b.log, trunk.OutputStatus); n != 1 {
		t.Errorf("B got %d talkgroup statuses, want 1", n)
	}
	if st, _ := has(b.log, trunk.OutputStatus); st.Status != trunk.StatusEmergency || !st.Grant.Group || st.Grant.Target != 9 {
		t.Errorf("B status %+v", st)
	}
	if _, ok := has(c.log, trunk.OutputStatus); ok {
		t.Error("C took a status of a talkgroup it is not in")
	}

	a.log = nil
	if err := c.ms.SetStatus(17); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.PollStatus(s.now, 3120003); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputStatus); return ok })
	if st, _ := has(a.log, trunk.OutputStatus); st.Status != 17 || st.Grant.Source != 3120003 {
		t.Errorf("poll answer %+v", st)
	}

	a.log, s.events = nil, nil
	if err := s.tscc.SendStatus(s.now, false, 3120002, 0xFFFECB, 5); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCStatus); return ok })
	if e, _ := s.event(trunk.TSCCStatus); e.Ack != enums.ReasonMSAccepted || e.Call.Source != 0xFFFECB || e.Status != 5 {
		t.Errorf("dispatcher status %+v", e)
	}
	if err := s.tscc.SendStatus(s.now, true, 9, 1, trunk.StatusPoll); err == nil {
		t.Error("TSCC.SendStatus: no error for a poll of a talkgroup")
	}
	if err := a.ms.SendStatus(s.now, false, 3120003, trunk.StatusPoll); err == nil {
		t.Error("SendStatus: no error for status 127")
	}
	if err := c.ms.SetStatus(127); err == nil {
		t.Error("SetStatus: no error for status 127")
	}
}

func TestStatus_NoAnswer(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.Reg, cfg.AnswerTimeout = false, time.Second
	s := newSim(t, cfg, &trunk.MSConfig{Address: 3120001, Seed: 1})
	a := s.ms[0]
	s.run(time.Second)
	if err := a.ms.SendStatus(s.now, false, 7777, 3); err != nil {
		t.Fatal(err)
	}
	s.until(4*time.Second, func() bool { _, ok := has(a.log, trunk.OutputRefused); return ok })
	if r, _ := has(a.log, trunk.OutputRefused); r.Ack != enums.ReasonMSAwayRefused {
		t.Errorf("refused %+v", r)
	}
	if e, ok := s.event(trunk.TSCCStatus); !ok || e.Reason != trunk.ReasonTimeout {
		t.Errorf("TSCCStatus %+v %v", e, ok)
	}
}

func voiceSuperframe(t testing.TB) [6]layer2.RawBurst {
	t.Helper()
	var sf [6]layer2.RawBurst
	var frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	for i := range sf {
		raw, err := layer2.BuildVoiceBurst(frames, enums.VoiceBurstA+enums.VoiceBurstType(i), pdu.EmbeddedSignalling{ColorCode: testCC}, [4]byte{}, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		sf[i] = raw
	}
	return sf
}

func talk(s *sim, m *simMS, slot enums.Slot, d time.Duration, done func() bool) {
	s.t.Helper()
	sf := voiceSuperframe(s.t)
	for i, end := 0, s.now+d; s.now < end; i++ {
		s.inject(m, sf[i%6], slot)
		s.run(60 * time.Millisecond)
		if done != nil && done() {
			return
		}
	}
}

func groupCall(t *testing.T, s *sim, a, b *simMS) trunk.Grant {
	t.Helper()
	if err := a.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && b.ms.State() == trunk.StatePayload })
	g, _ := s.event(trunk.TSCCCallStart)
	s.inject(a, voiceLC(t, true, 9, 3120001, enums.DataTypeVoiceLCHeader), g.Call.Slot)
	s.run(60 * time.Millisecond)
	return g.Call
}

func TestInterrupt_PayloadInterrupt(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.TransmitInterrupt = true
	s := registeredSim(t, cfg, threeMS()...)
	a, b := s.ms[0], s.ms[1]
	g := groupCall(t, s, a, b)

	var ierr error
	talk(s, a, g.Slot, 600*time.Millisecond, nil)
	s.mout, ierr = b.ms.Interrupt(s.now, false, s.mout[:0])
	if ierr != nil {
		t.Fatal(ierr)
	}
	s.msOutputs(b, s.mout)
	if ce, ok := has(b.log, trunk.OutputCallEnd); !ok || ce.Reason != trunk.ReasonInterrupt || b.ms.State() != trunk.StateControl {
		t.Fatalf("B did not leave for the interrupt: %+v %v", ce, ok)
	}
	talk(s, a, g.Slot, 5*time.Second, func() bool { _, ok := has(a.log, trunk.OutputReverseChannel); return ok })
	if acc, ok := has(b.log, trunk.OutputAccepted); !ok || acc.Ack != enums.ReasonMessageAccepted || b.ms.State() != trunk.StatePayload || b.channel != g.Channel {
		t.Fatalf("B after the interrupt: %+v %v, state %v channel %d", acc, ok, b.ms.State(), b.channel)
	}
	rc, ok := has(a.log, trunk.OutputReverseChannel)
	if !ok || rc.RC != enums.RCCeaseTransmissionCommand {
		t.Fatalf("A reverse channel %+v %v", rc, ok)
	}
	s.inject(a, voiceLC(t, true, 9, 3120001, enums.DataTypeTerminatorWithLC), g.Slot)
	s.until(2*time.Second, func() bool { _, ok := has(b.log, trunk.OutputProtect); return ok })
	if e, ok := s.event(trunk.TSCCInterrupt); !ok || e.Address != 3120002 || e.Reason != trunk.ReasonNone {
		t.Errorf("TSCCInterrupt %+v %v", e, ok)
	}
	if p, _ := has(b.log, trunk.OutputProtect); p.Protect != enums.ProtectEnablePTTOneMS {
		t.Errorf("B protect %+v", p)
	}
	if p, ok := has(a.log, trunk.OutputProtect); !ok || p.Protect != enums.ProtectEnablePTTOneMS {
		t.Errorf("A protect %+v %v", p, ok)
	}
	if _, err := s.ms[2].ms.Interrupt(s.now, false, nil); !errors.Is(err, trunk.ErrNoCall) {
		t.Errorf("Interrupt on the TSCC: %v", err)
	}
}

func TestInterrupt_Refused(t *testing.T) {
	t.Parallel()
	s := registeredSim(t, tsccConfig(), threeMS()...)
	a, b := s.ms[0], s.ms[1]
	g := groupCall(t, s, a, b)
	var err error
	s.mout, err = b.ms.Interrupt(s.now, false, s.mout[:0])
	if err != nil {
		t.Fatal(err)
	}
	s.msOutputs(b, s.mout)
	talk(s, a, g.Slot, 3*time.Second, func() bool { _, ok := has(b.log, trunk.OutputRefused); return ok && b.ms.State() == trunk.StatePayload })
	if r, _ := has(b.log, trunk.OutputRefused); r.Ack != enums.ReasonNotSupported || b.channel != g.Channel {
		t.Errorf("refused %+v, channel %d", r, b.channel)
	}
	if _, ok := has(a.log, trunk.OutputReverseChannel); ok {
		t.Error("A got a reverse channel command")
	}
}

func TestInterrupt_TerminateAndCallTimer(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.TransmitInterrupt = true
	s := registeredSim(t, cfg, threeMS()...)
	a, b := s.ms[0], s.ms[1]
	g := groupCall(t, s, a, b)
	var err error
	s.mout, err = b.ms.Interrupt(s.now, true, s.mout[:0])
	if err != nil {
		t.Fatal(err)
	}
	s.msOutputs(b, s.mout)
	talk(s, a, g.Slot, 5*time.Second, func() bool { _, ok := has(a.log, trunk.OutputReverseChannel); return ok })
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCCallEnd); return ok })
	if e, _ := s.event(trunk.TSCCCallEnd); e.Reason != trunk.ReasonInterrupt {
		t.Errorf("TSCCCallEnd %+v", e)
	}
	if b.ms.State() != trunk.StateControl {
		t.Errorf("B state %v after a terminate", b.ms.State())
	}

	cfg = tsccConfig()
	cfg.MaxCall = 2 * time.Second
	s = registeredSim(t, cfg, threeMS()...)
	a, b = s.ms[0], s.ms[1]
	g = groupCall(t, s, a, b)
	talk(s, a, g.Slot, 6*time.Second, func() bool { _, ok := has(a.log, trunk.OutputReverseChannel); return ok })
	if rc, ok := has(a.log, trunk.OutputReverseChannel); !ok || rc.RC != enums.RCCeaseTransmissionCommand {
		t.Fatalf("call timer RC %+v %v", rc, ok)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCCallEnd); return ok })
	if e, _ := s.event(trunk.TSCCCallEnd); e.Reason != trunk.ReasonCallTimer {
		t.Errorf("TSCCCallEnd %+v", e)
	}
	if e, ok := s.event(trunk.TSCCInterrupt); !ok || e.Address != 0 {
		t.Errorf("TSCCInterrupt %+v %v", e, ok)
	}

	s.run(1500 * time.Millisecond)
	s.events = nil
	g = groupCall(t, s, a, b)
	s.until(4*time.Second, func() bool { _, ok := s.event(trunk.TSCCCallEnd); return ok })
	if e, _ := s.event(trunk.TSCCCallEnd); e.Reason != trunk.ReasonCallTimer {
		t.Errorf("silent call: TSCCCallEnd %+v", e)
	}
	_ = g
}

func TestInterrupt_TSCCPowerControl(t *testing.T) {
	t.Parallel()
	s := registeredSim(t, tsccConfig(), threeMS()...)
	a, b := s.ms[0], s.ms[1]
	g := groupCall(t, s, a, b)
	if err := s.tscc.Interrupt(s.now, g.Channel, g.Slot, enums.RCIncreasePower); err != nil {
		t.Fatal(err)
	}
	talk(s, a, g.Slot, 2*time.Second, nil)
	if n := countKind(a.log, trunk.OutputReverseChannel); n != 1 {
		t.Errorf("%d RC outputs, want 1", n)
	}
	if rc, _ := has(a.log, trunk.OutputReverseChannel); rc.RC != enums.RCIncreasePower {
		t.Errorf("RC %+v", rc)
	}
	if err := s.tscc.Interrupt(s.now, g.Channel, g.Slot.Other(), enums.RCCeaseTransmissionRequest); !errors.Is(err, trunk.ErrNoCall) {
		t.Errorf("Interrupt of an idle slot: %v", err)
	}
	if err := s.tscc.Interrupt(s.now, g.Channel, g.Slot, 6); err == nil {
		t.Error("Interrupt: no error for a reserved command")
	}
	if e, ok := s.event(trunk.TSCCInterrupt); ok {
		t.Errorf("power control reported %+v", e)
	}
}

func TestStun_WithoutAuthentication(t *testing.T) {
	t.Parallel()
	cfgs := threeMS()
	cfgs[2].NoStun = true
	s := registeredSim(t, tsccConfig(), cfgs...)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]

	if err := s.tscc.Stun(s.now, 3120001, false); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.Stun(s.now, 3120001, false); !errors.Is(err, trunk.ErrBusy) {
		t.Errorf("second Stun: %v", err)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCStun); return ok })
	if e, _ := s.event(trunk.TSCCStun); e.Address != 3120001 || e.Stun != trunk.StunStun || e.Ack != enums.ReasonMSAccepted || e.Reason != trunk.ReasonNone {
		t.Errorf("TSCCStun %+v", e)
	}
	if o, ok := has(a.log, trunk.OutputStun); !ok || o.Stun != trunk.StunStun || !a.ms.Stunned() {
		t.Errorf("A stun %+v %v %v", o, ok, a.ms.Stunned())
	}
	if err := a.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{}); !errors.Is(err, trunk.ErrStunned) {
		t.Errorf("Call of a stunned MS: %v", err)
	}
	if err := a.ms.SendStatus(s.now, true, 9, 1); !errors.Is(err, trunk.ErrStunned) {
		t.Errorf("SendStatus of a stunned MS: %v", err)
	}
	if err := b.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return b.ms.State() == trunk.StatePayload })
	s.run(300 * time.Millisecond)
	if a.ms.State() != trunk.StateControl {
		t.Errorf("stunned A followed the call: %v", a.ms.State())
	}
	s.run(5 * time.Second)

	s.events = nil
	a.log = nil
	if err := s.tscc.Stun(s.now, 3120001, true); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCStun); return ok })
	if o, ok := has(a.log, trunk.OutputStun); !ok || o.Stun != trunk.StunRevive || a.ms.Stunned() {
		t.Errorf("A revive %+v %v %v", o, ok, a.ms.Stunned())
	}

	s.events = nil
	if err := s.tscc.Stun(s.now, 3120003, false); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCStun); return ok })
	if e, _ := s.event(trunk.TSCCStun); e.Ack != enums.ReasonMSNotSupported || c.ms.Stunned() {
		t.Errorf("NoStun: %+v, stunned %v", e, c.ms.Stunned())
	}

	s.events = nil
	if err := s.tscc.Stun(s.now, 3129999, false); err != nil {
		t.Fatal(err)
	}
	s.until(15*time.Second, func() bool { _, ok := s.event(trunk.TSCCStun); return ok })
	if e, _ := s.event(trunk.TSCCStun); e.Reason != trunk.ReasonTimeout {
		t.Errorf("absent MS: %+v", e)
	}
	if err := s.tscc.Stun(s.now, 0, false); err == nil {
		t.Error("Stun 0: no error")
	}
}

func TestStun_Authenticated(t *testing.T) {
	t.Parallel()
	cfgs := threeMS()
	cfgs[0].Auth = xorAuth(0x5A5A5A)
	cfgs[1].Auth = xorAuth(0x123456)
	s := registeredSim(t, tsccConfig(), cfgs...)
	a, b := s.ms[0], s.ms[1]

	if err := s.tscc.Stun(s.now, 3120001, false); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCStun); return ok })
	if e, _ := s.event(trunk.TSCCStun); e.Reason != trunk.ReasonAuthentication || a.ms.Stunned() {
		t.Errorf("TSCC without a key: %+v, stunned %v", e, a.ms.Stunned())
	}

	cfg := tsccConfig()
	cfg.Auth = xorAuth(0x5A5A5A)
	s = registeredSim(t, cfg, cfgs...)
	a, b = s.ms[0], s.ms[1]
	if err := s.tscc.Stun(s.now, 3120001, false); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.Stun(s.now, 3120002, false); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return len(events(s, trunk.TSCCStun)) == 2 })
	for _, e := range events(s, trunk.TSCCStun) {
		switch e.Address {
		case 3120001:
			if e.Ack != enums.ReasonMSAccepted || e.Reason != trunk.ReasonNone || !a.ms.Stunned() {
				t.Errorf("A: %+v, stunned %v", e, a.ms.Stunned())
			}
		case 3120002:
			if e.Ack != enums.ReasonRecipientRefused || e.Reason != trunk.ReasonAuthentication || b.ms.Stunned() {
				t.Errorf("B with another key: %+v, stunned %v", e, b.ms.Stunned())
			}
		}
	}

	s.events = nil
	if err := s.tscc.Kill(s.now, 3120001); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.Kill(s.now, 3120003); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return len(events(s, trunk.TSCCStun)) == 2 })
	for _, e := range events(s, trunk.TSCCStun) {
		switch e.Address {
		case 3120001:
			if e.Stun != trunk.StunKill || e.Ack != enums.ReasonMSAccepted || !a.ms.Killed() || !a.ms.Off() {
				t.Errorf("kill A: %+v, killed %v", e, a.ms.Killed())
			}
		case 3120003:
			if e.Ack != enums.ReasonMSNotSupported || s.ms[2].ms.Killed() {
				t.Errorf("kill without a key: %+v", e)
			}
		}
	}
	a.ms.Reset()
	if !a.ms.Off() || !a.ms.Killed() {
		t.Error("Reset revived a killed MS")
	}
	if _, ok := a.ms.Deadline(); ok {
		t.Error("killed MS has a deadline")
	}
}

func TestStun_ConfigState(t *testing.T) {
	t.Parallel()
	m, err := trunk.NewMS(&trunk.MSConfig{Address: 3120001, Stunned: true})
	if err != nil || !m.Stunned() || m.Killed() || m.Off() {
		t.Fatalf("Stunned config: %v %v %v %v", err, m.Stunned(), m.Killed(), m.Off())
	}
	m, err = trunk.NewMS(&trunk.MSConfig{Address: 3120001, Killed: true})
	if err != nil || !m.Killed() || !m.Off() {
		t.Fatalf("Killed config: %v %v %v", err, m.Killed(), m.Off())
	}
	if _, err := trunk.NewMS(&trunk.MSConfig{Address: 3120001, PowerSave: 8}); err == nil {
		t.Error("PowerSave 8: no error")
	}
}
