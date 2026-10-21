package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func inFrame(counter uint16, rq, offset uint8) bool {
	mask := uint16(1)<<rq - 1
	return (counter>>2)&mask == uint16(offset) || ((counter+1)%512>>2)&mask == uint16(offset)
}

func TestPowerSave_SleepAndHeldAhoy(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.PowerSave = true
	cfgs := threeMS()
	cfgs[2].PowerSave = 4
	s := registeredSim(t, cfg, cfgs...)
	a, c := s.ms[0], s.ms[2]
	off := c.ms.PowerSaveOffset()
	if want := uint8(1 + 3120003%15); off != want {
		t.Fatalf("PowerSave_Offset %d, want %d", off, want)
	}
	if a.ms.PowerSaveOffset() != 0 {
		t.Errorf("A has offset %d without power save", a.ms.PowerSaveOffset())
	}
	s.until(5*time.Second, func() bool { return c.ms.Asleep() })
	sl, _ := has(c.log, trunk.OutputSleep)
	if at, ok := c.ms.Deadline(); !ok || at != sl.Due || sl.Due < s.now {
		t.Fatalf("asleep: Deadline %v %v, sleep %+v at %v", at, ok, sl, s.now)
	}
	s.until(10*time.Second, func() bool { return !c.ms.Asleep() })
	if !inFrame(c.ms.Site().SlotCounter, 4, off) {
		t.Errorf("woke at counter %d, offset %d", c.ms.Site().SlotCounter, off)
	}
	s.until(20*time.Second, func() bool { return c.ms.Asleep() })
	s.run(10 * time.Second)
	if c.ms.State() != trunk.StateControl {
		t.Fatalf("C lost the TSCC in power save: %v", c.ms.State())
	}

	c.log = nil
	s.until(10*time.Second, func() bool { return c.ms.Asleep() })
	if err := a.ms.Call(s.now, false, 3120003, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(10*time.Second, func() bool { _, ok := has(c.log, trunk.OutputAhoy); return ok })
	if !inFrame(c.ms.Site().SlotCounter, 4, off) {
		t.Errorf("C_AHOY at counter %d, offset %d", c.ms.Site().SlotCounter, off)
	}
	s.until(3*time.Second, func() bool { return c.ms.State() == trunk.StatePayload })
}

func TestMassRegistration(t *testing.T) {
	t.Parallel()
	s := registeredSim(t, tsccConfig(), threeMS()...)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	if err := s.tscc.MassRegister(2, 0, 0); err != nil {
		t.Fatal(err)
	}
	s.run(1500 * time.Millisecond)
	for i, m := range s.ms {
		if n := countKind(m.log, trunk.OutputRegistered); n != 1 {
			t.Errorf("MS %d registered %d times in the window", i, n)
		}
	}

	a.log, b.log, c.log = nil, nil, nil
	if err := s.tscc.MassRegister(1, 24, 3120002); err != nil {
		t.Fatal(err)
	}
	s.run(time.Second)
	if countKind(a.log, trunk.OutputRegistered) != 0 || countKind(b.log, trunk.OutputRegistered) != 1 || countKind(c.log, trunk.OutputRegistered) != 0 {
		t.Errorf("Mask 24: %d %d %d registrations", countKind(a.log, trunk.OutputRegistered), countKind(b.log, trunk.OutputRegistered), countKind(c.log, trunk.OutputRegistered))
	}

	b.log = nil
	if err := s.tscc.MassRegister(9, 24, 3120002); err != nil {
		t.Fatal(err)
	}
	s.run(200 * time.Millisecond)
	if err := s.tscc.MassRegister(0, 24, 3120002); err != nil {
		t.Fatal(err)
	}
	s.run(time.Second)
	if at, ok := b.ms.Deadline(); ok && at > s.now+10*time.Second {
		t.Errorf("cancelled mass registration still has a deadline at %v", at)
	}
	if err := s.tscc.MassRegister(16, 0, 0); err == nil {
		t.Error("MassRegister window 16: no error")
	}
}

func TestIPAdvice(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.IP = true
	s := registeredSim(t, cfg, threeMS()...)
	a := s.ms[0]
	ip := [16]byte{10, 1, 2, 3}
	if err := a.ms.AdviseIP(s.now, ip, false, false); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.AdviseIP(s.now, ip, false, false); err == nil {
		t.Error("second AdviseIP: no error")
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputAccepted); return ok })
	e, ok := s.event(trunk.TSCCIPAdvice)
	if !ok || e.Address != 3120001 || e.IP != ip || e.IPv6 || e.Remove {
		t.Errorf("TSCCIPAdvice %+v %v", e, ok)
	}
	if acc, _ := has(a.log, trunk.OutputAccepted); acc.Ack != enums.ReasonRegAccepted || !a.ms.Registered() {
		t.Errorf("accepted %+v, registered %v", acc, a.ms.Registered())
	}

	a.log, s.events = nil, nil
	ip6 := [16]byte{0x20, 0x01, 0x0d, 0xb8, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	if err := a.ms.AdviseIP(s.now, ip6, true, true); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputAccepted); return ok })
	if e, ok := s.event(trunk.TSCCIPAdvice); !ok || e.IP != ip6 || !e.IPv6 || !e.Remove || !s.tscc.Registered(3120001) {
		t.Errorf("TSCCIPAdvice %+v %v, registered %v", e, ok, s.tscc.Registered(3120001))
	}

	s = registeredSim(t, tsccConfig(), threeMS()...)
	a = s.ms[0]
	if err := a.ms.AdviseIP(s.now, ip, false, false); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputRefused); return ok })
	if r, _ := has(a.log, trunk.OutputRefused); r.Ack != enums.ReasonIPConnectionFailed || !a.ms.Registered() {
		t.Errorf("no IP bearer: %+v, registered %v", r, a.ms.Registered())
	}
}

func TestSupplementary_VoiceCall(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.Supplementary = true
	s := registeredSim(t, cfg, threeMS()...)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	data := []byte("call me back on 4711")
	if err := a.ms.SetSupplementary(4, data); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.Call(s.now, false, 3120003, layer3.VoiceServiceOptions{IsSuppData: true}); err != nil {
		t.Fatal(err)
	}
	s.until(5*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && c.ms.State() == trunk.StatePayload })
	if _, ok := has(c.log, trunk.OutputUDT); !ok {
		t.Fatal("C got no supplementary data")
	}
	if u := c.ms.UDT(); string(u.Data[:len(data)]) != string(data) || !u.Header.SupplementaryFlag || u.Header.LLIDDestination != 3120003 || u.Header.UDTFormat != 4 {
		t.Errorf("C UDT %+v", u)
	}
	if _, ok := s.event(trunk.TSCCCallStart); !ok {
		t.Error("no call after the supplementary data")
	}
	_ = b

	s = registeredSim(t, cfg, threeMS()...)
	a, b = s.ms[0], s.ms[1]
	if err := a.ms.SetSupplementary(4, data[:5]); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{IsSuppData: true}); err != nil {
		t.Fatal(err)
	}
	s.until(5*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && b.ms.State() == trunk.StatePayload })
	if _, ok := has(b.log, trunk.OutputUDT); !ok {
		t.Error("B got no supplementary data of the talkgroup call")
	}
	if err := a.ms.SetSupplementary(16, nil); err == nil {
		t.Error("SetSupplementary format 16: no error")
	}
	if err := a.ms.SetSupplementary(0, make([]byte, trunk.MaxSupplementary+1)); err == nil {
		t.Error("SetSupplementary: no error for too much data")
	}
}

func TestSupplementary_NotOffered(t *testing.T) {
	t.Parallel()
	s := registeredSim(t, tsccConfig(), threeMS()...)
	a, c := s.ms[0], s.ms[2]
	if err := a.ms.SetSupplementary(4, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.Call(s.now, false, 3120003, layer3.VoiceServiceOptions{IsSuppData: true}); err != nil {
		t.Fatal(err)
	}
	s.until(5*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && c.ms.State() == trunk.StatePayload })
	if _, ok := has(c.log, trunk.OutputUDT); ok {
		t.Error("C got supplementary data from a TSCC without the service")
	}
}

func TestMS_RegistrationResponseInfo(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		ps     uint8
		reason enums.AckReason
		info   uint8
		offset uint8
	}{
		{"Reg_Accepted 0x7F without power save", 0, enums.ReasonRegAccepted, 0x7F, 0},
		{"Reg_Denied 9 without power save", 0, enums.ReasonRegDenied, 9, 0},
		{"Reg_Denied 9 with power save", 2, enums.ReasonRegDenied, 9, 0},
		{"Reg_Accepted 3 with power save", 2, enums.ReasonRegAccepted, 3, 3},
	} {
		r := newRig(t, &trunk.MSConfig{Address: msAddr, Seed: 7, PowerSave: tt.ps})
		a := aloha(t)
		r.feed(a, &idle)
		wantKinds(t, tt.name+" C_RAND", r.feed(a, &idle), trunk.OutputBurst)
		out := r.feed(svcAck(t, tt.reason, tt.info, constants.GatewayREGI), &idle)
		accepted := tt.reason == enums.ReasonRegAccepted
		if accepted {
			wantKinds(t, tt.name, out, trunk.OutputRegistered)
		} else if len(out) == 0 || out[0].Kind != trunk.OutputRefused || out[0].Ack != enums.ReasonRegDenied {
			t.Errorf("%s: %+v", tt.name, out)
		}
		if r.ms.Registered() != accepted || r.ms.PowerSaveOffset() != tt.offset || r.ms.Asleep() {
			t.Errorf("%s: registered %v, offset %d, asleep %v", tt.name, r.ms.Registered(), r.ms.PowerSaveOffset(), r.ms.Asleep())
		}
	}
}
