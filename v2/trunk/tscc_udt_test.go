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

func udtSim(t *testing.T, extra ...*trunk.MSConfig) *sim {
	t.Helper()
	tg := [trunk.MaxTalkgroups]uint32{9}
	cfgs := append([]*trunk.MSConfig{
		{Address: 3120001, Talkgroups: tg, TalkgroupsLen: 1, Seed: 1},
		{Address: 3120002, Talkgroups: tg, TalkgroupsLen: 1, Seed: 2},
		{Address: 3120003, Seed: 3},
	}, extra...)
	s := newSim(t, tsccConfig(), cfgs...)
	s.until(5*time.Second, func() bool {
		for _, m := range s.ms {
			if !m.ms.Registered() {
				return false
			}
		}
		return true
	})
	for _, m := range s.ms {
		m.log = nil
	}
	s.events = nil
	return s
}

func waitLog(s *sim, m *simMS, kind trunk.OutputKind) trunk.Output {
	s.t.Helper()
	s.until(5*time.Second, func() bool { _, ok := has(m.log, kind); return ok })
	o, _ := has(m.log, kind)
	return o
}

func waitEvent(s *sim, kind trunk.TSCCOutputKind) trunk.TSCCOutput {
	s.t.Helper()
	s.until(5*time.Second, func() bool { _, ok := s.event(kind); return ok })
	e, _ := s.event(kind)
	return e
}

func TestTSCC_ShortData(t *testing.T) {
	t.Parallel()
	s := udtSim(t)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	var text trunk.Appended
	if err := text.SetText(enums.UDTISO8, []byte("trunked short data")); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.SendShortData(s.now, false, 3120002, &text, layer3.ShortDataServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	d := waitLog(s, a, trunk.OutputDelivered)
	if d.Ack != enums.AckReasonMSAccepted {
		t.Errorf("delivered %v", d.Ack)
	}
	if _, ok := has(b.log, trunk.OutputUDT); !ok {
		t.Fatal("B got no UDT")
	}
	got := udtOf(b.ms.UDT()).Appended()
	if s, err := got.Text(nil); err != nil || string(s) != "trunked short data" || udtOf(b.ms.UDT()).Header.LLIDSource != 3120001 {
		t.Errorf("B got %q %v", s, err)
	}
	if e, ok := s.event(trunk.TSCCDelivered); !ok || e.Address != 3120001 || e.Grant.Target != 3120002 {
		t.Errorf("TSCCDelivered %+v %v", e, ok)
	}

	a.log, b.log, s.events = nil, nil, nil
	if err := a.ms.SendShortData(s.now, true, 9, &text, layer3.ShortDataServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if d := waitLog(s, a, trunk.OutputDelivered); d.Ack != enums.AckReasonMessageAccepted {
		t.Errorf("group delivered %v", d.Ack)
	}
	s.run(time.Second)
	if _, ok := has(b.log, trunk.OutputUDT); !ok {
		t.Error("talkgroup member got no UDT")
	}
	for _, o := range b.log {
		if o.Kind == trunk.OutputBurst {
			t.Error("talkgroup member answered the UDT")
		}
	}

	c.log, s.events = nil, nil
	if err := s.tscc.SendShortData(s.now, false, 3120003, &text); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.SendShortData(s.now, false, 3120003, &text); !errors.Is(err, trunk.ErrBusy) {
		t.Errorf("second download %v", err)
	}
	if e := waitEvent(s, trunk.TSCCDelivered); e.Address != constants.GatewaySDMI || e.Grant.Target != 3120003 {
		t.Errorf("gateway delivered %+v", e)
	}
	if _, ok := has(c.log, trunk.OutputUDT); !ok {
		t.Error("C got no UDT")
	}
}

func TestTSCC_Polling(t *testing.T) {
	t.Parallel()
	s := udtSim(t)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	var pos trunk.Appended
	if err := pos.SetNMEAShort(&trunk.NMEAShort{North: true, Fix: true, LatDegrees: 41, LonDegrees: 87}); err != nil {
		t.Fatal(err)
	}
	b.ms.SetPollResponse(&pos)
	if err := a.ms.PollShortData(s.now, 3120002, enums.UDTNMEA, 1); err != nil {
		t.Fatal(err)
	}
	waitLog(s, a, trunk.OutputDelivered)
	got := udtOf(a.ms.UDT()).Appended()
	if n, err := got.NMEAShort(); err != nil || n.LatDegrees != 41 {
		t.Errorf("polled %+v %v", n, err)
	}
	if _, ok := has(b.log, trunk.OutputPolled); !ok {
		t.Error("B saw no poll")
	}
	if e := waitEvent(s, trunk.TSCCDelivered); e.Address != 3120001 || e.Grant.Target != 3120002 {
		t.Errorf("TSCCDelivered %+v", e)
	}

	a.log, s.events = nil, nil
	if err := a.ms.PollShortData(s.now, 3120003, enums.UDTNMEA, 1); err != nil {
		t.Fatal(err)
	}
	if r := waitLog(s, a, trunk.OutputRefused); r.Ack != enums.AckReasonMSNotSupported {
		t.Errorf("poll refused %+v", r)
	}

	s.events = nil
	if err := s.tscc.PollShortData(s.now, 3120002, 1); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(s, trunk.TSCCUDT); e.Address != 3120002 {
		t.Errorf("TSCCUDT %+v", e)
	}
	u := udtOf(s.tscc.UDT()).Appended()
	if n, err := u.NMEAShort(); err != nil || n.LonDegrees != 87 || udtOf(s.tscc.UDT()).Header.LLIDDestination != constants.GatewaySDMI {
		t.Errorf("gateway poll %+v %v", n, err)
	}

	s.events = nil
	lip := trunk.USBDLIPResponse{Latitude: 45, Longitude: -90}
	p, err := lip.Encode()
	if err != nil {
		t.Fatal(err)
	}
	c.ms.SetUSBDResponse(&pdu.UnifiedSingleBlockData{ServiceType: pdu.ServiceTypeLIP, Payload: p})
	if err := s.tscc.PollUSBD(s.now, 3120003, pdu.ServiceTypeLIP, 0); err != nil {
		t.Fatal(err)
	}
	waitEvent(s, trunk.TSCCUSBD)
	if back := trunk.DecodeUSBDLIPResponse(s.tscc.USBD().Payload); back != lip {
		t.Errorf("USBD response %+v", back)
	}
	s.events = nil
	if err := s.tscc.PollUSBD(s.now, 3120002, pdu.ServiceTypeLIP, 1); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(s, trunk.TSCCRefused); e.Ack != enums.AckReasonMSNotSupported {
		t.Errorf("USBD refused %+v", e)
	}
	if err := s.tscc.PollUSBD(s.now, 0, pdu.ServiceTypeLIP, 0); err == nil {
		t.Error("USBD poll of address 0")
	}
}

func TestTSCC_Diversion(t *testing.T) {
	t.Parallel()
	s := udtSim(t)
	a, b := s.ms[0], s.ms[1]
	var to trunk.Appended
	if err := to.SetAddresses(false, []uint32{3120003}); err != nil {
		t.Fatal(err)
	}
	opts := layer3.DiversionServiceOptions{Set: true, Voice: true}
	if err := b.ms.Divert(s.now, constants.GatewayMSI, opts, &to); err != nil {
		t.Fatal(err)
	}
	waitLog(s, b, trunk.OutputDelivered)
	if e := waitEvent(s, trunk.TSCCDiversion); e.Address != 3120002 {
		t.Errorf("TSCCDiversion %+v", e)
	}
	got, dest, ok := s.tscc.Diverted(3120002)
	if !ok || got != opts || dest != to {
		t.Fatalf("Diverted %+v %v", got, ok)
	}
	if err := a.ms.Call(s.now, false, 3120002, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := waitLog(s, a, trunk.OutputRefused); r.Reason != trunk.ReasonDiverted {
		t.Fatalf("call to a diverted MS %+v", r)
	}
	u := udtOf(a.ms.UDT()).Appended()
	if l, err := u.Addresses(); err != nil || l.Addresses[0] != 3120003 {
		t.Errorf("diverted to %+v %v", l, err)
	}

	b.log, s.events = nil, nil
	if err := b.ms.Divert(s.now, constants.GatewayTSI, layer3.DiversionServiceOptions{Voice: true}, nil); err != nil {
		t.Fatal(err)
	}
	waitLog(s, b, trunk.OutputDelivered)
	if _, _, ok := s.tscc.Diverted(3120002); ok {
		t.Error("diversion kept after the clear")
	}

	b.log = nil
	var pstn trunk.Appended
	if err := pstn.SetDigits([]byte("5551234")); err != nil {
		t.Fatal(err)
	}
	if err := b.ms.Divert(s.now, constants.GatewayPSTNI, opts, &pstn); err != nil {
		t.Fatal(err)
	}
	if r := waitLog(s, b, trunk.OutputRefused); r.Ack != enums.AckReasonTransientSysRefused {
		t.Errorf("PSTN diversion %+v", r)
	}
}

func TestTSCC_DGNA(t *testing.T) {
	t.Parallel()
	s := udtSim(t)
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	var groups trunk.Appended
	if err := groups.SetAddresses(true, []uint32{501, 502}); err != nil {
		t.Fatal(err)
	}
	if err := a.ms.AssignGroups(s.now, 3120002, &groups); err != nil {
		t.Fatal(err)
	}
	waitLog(s, a, trunk.OutputDelivered)
	if g := b.ms.Groups(); g.Groups[0] != 501 || g.Groups[1] != 502 || g.OneKey != 501 {
		t.Errorf("B groups %+v", g)
	}
	s.events = nil
	if err := s.tscc.AssignGroups(s.now, 3120003, &groups); err != nil {
		t.Fatal(err)
	}
	if e := waitEvent(s, trunk.TSCCDelivered); e.Address != constants.GatewayDGNAI {
		t.Errorf("gateway DGNA %+v", e)
	}
	if g := c.ms.Groups(); g.Groups[1] != 502 {
		t.Errorf("C groups %+v", g)
	}
	if err := s.tscc.AssignGroups(s.now, 3120003, &trunk.Appended{Format: enums.UDTBCD, Blocks: 1}); !errors.Is(err, trunk.ErrUDTFormat) {
		t.Errorf("BCD DGNA %v", err)
	}
}

func TestTSCC_Duplex(t *testing.T) {
	t.Parallel()
	s := udtSim(t, &trunk.MSConfig{Address: 3120004, Seed: 4, NoDuplex: true})
	a, b, d := s.ms[0], s.ms[1], s.ms[3]
	if err := a.ms.CallDuplex(s.now, 3120002, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && b.ms.State() == trunk.StatePayload })
	if a.channel != b.channel {
		t.Errorf("a on %d, b on %d", a.channel, b.channel)
	}
	ga, _ := has(a.log, trunk.OutputGrant)
	gb, _ := has(b.log, trunk.OutputGrant)
	_ = gb
	start := waitEvent(s, trunk.TSCCCallStart)
	if start.Grant.Service != enums.ServiceKindDuplexVoice || ga.Grant.Service != enums.ServiceKindDuplexVoice {
		t.Errorf("duplex grant %+v", start.Grant)
	}
	s.mout = a.ms.EndCall(s.now, s.mout[:0])
	s.msOutputs(a, s.mout)
	ends := 0
	s.until(3*time.Second, func() bool {
		ends = 0
		for _, e := range s.events {
			if e.Kind == trunk.TSCCCallEnd {
				ends++
			}
		}
		return ends == 2 && a.ms.State() == trunk.StateControl && b.ms.State() == trunk.StateControl
	})

	a.log = nil
	if err := a.ms.CallDuplex(s.now, 3120004, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := waitLog(s, a, trunk.OutputRefused); r.Ack != enums.AckReasonMSDuplexNotSupported {
		t.Errorf("NoDuplex %+v", r)
	}
	if _, ok := has(d.log, trunk.OutputAhoy); !ok {
		t.Error("D saw no radio check")
	}
}

func TestTSCC_Ambient(t *testing.T) {
	t.Parallel()
	s := udtSim(t, &trunk.MSConfig{Address: 3120004, Seed: 4, Ambient: true})
	a, d := s.ms[0], s.ms[3]
	if err := a.ms.CallAmbient(s.now, 3120004, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	waitLog(s, d, trunk.OutputAmbient)
	if a.ms.State() != trunk.StatePayload || a.channel != d.channel {
		t.Errorf("caller state %v on %d, called on %d", a.ms.State(), a.channel, d.channel)
	}
}
