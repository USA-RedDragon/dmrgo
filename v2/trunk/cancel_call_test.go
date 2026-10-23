package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

const cancelCaller, cancelCalled = 3120001, 3120002

type cancelProbe struct {
	t      *testing.T
	ts     *trunk.TSCC
	now    time.Duration
	events []trunk.TSCCOutput
	csbks  []pdu.CSBK
}

func newCancelProbe(t *testing.T) *cancelProbe {
	t.Helper()
	cfg := tsccConfig()
	cfg.Reg = false
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := &cancelProbe{t: t, ts: ts}
	p.run(1)
	return p
}

func (p *cancelProbe) run(n int) {
	for range n {
		for _, o := range p.ts.Tick(p.now, nil) {
			if o.Kind != trunk.TSCCBurst {
				p.events = append(p.events, o)
				continue
			}
			if bu, err := layer2.NewBurstFromBytes(o.Burst); err == nil {
				if cs, ok := bu.Data.(*pdu.CSBK); ok {
					p.csbks = append(p.csbks, *cs)
				}
			}
		}
		p.now += 30 * time.Millisecond
	}
}

func (p *cancelProbe) inbound(set func(*pdu.CSBK) error) {
	p.t.Helper()
	c := pdu.CSBK{LastBlock: true}
	if err := set(&c); err != nil {
		p.t.Fatal(err)
	}
	raw := msBurst(p.t, &c)
	p.events = p.ts.Inbound(p.now, tsccChannel, &raw, layer2.Hint{Slot: enums.Slot1}, p.events)
}

func (p *cancelProbe) random(kind enums.ServiceKind) {
	p.inbound(func(c *pdu.CSBK) error {
		return c.SetRandomAccess(pdu.RandomAccessPDU{ServiceKind: uint8(kind), TargetAddress: cancelCalled, SourceAddress: cancelCaller})
	})
}

func (p *cancelProbe) answer() {
	p.inbound(func(c *pdu.CSBK) error {
		return c.SetAckInbound(pdu.AckInboundPDU{ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: cancelCaller, AdditionalInfo: cancelCalled})
	})
}

func (p *cancelProbe) acks(r enums.AckReason) int {
	n := 0
	for i := range p.csbks {
		if a, ok := p.csbks[i].AsAckOutbound(); ok && a.TargetAddress == cancelCaller && enums.AckReason(a.ReasonCode) == r {
			n++
		}
	}
	return n
}

func (p *cancelProbe) ahoys(kind enums.ServiceKind) int {
	n := 0
	for i := range p.csbks {
		if a, ok := p.csbks[i].AsAhoy(); ok && enums.ServiceKind(a.ServiceKind) == kind && !a.Group && a.TargetAddress == cancelCalled && a.SourceAddress == cancelCaller {
			n++
		}
	}
	return n
}

func (p *cancelProbe) calls() int {
	n := 0
	for _, e := range p.events {
		if e.Kind == trunk.TSCCCallStart {
			n++
		}
	}
	return n
}

func TestTSCC_CancelCall(t *testing.T) {
	t.Parallel()
	for _, sent := range []bool{false, true} {
		p := newCancelProbe(t)
		p.random(enums.ServiceKindIndividualVoice)
		if sent {
			p.run(20)
			if p.ahoys(enums.ServiceKindIndividualVoice) == 0 {
				t.Fatal("no C_AHOY to B")
			}
		}
		p.random(enums.ServiceKindCancelCall)
		p.run(20)
		if n := p.acks(enums.AckReasonMessageAccepted); n != 1 {
			t.Errorf("sent %v: %d C_ACKD Message_Accepted to A, want 1", sent, n)
		}
		if n := p.ahoys(enums.ServiceKindIndividualVoice); sent && n != 1 || !sent && n != 0 {
			t.Errorf("sent %v: %d C_AHOY call to B", sent, n)
		}
		if n := p.ahoys(enums.ServiceKindCancelCall); sent && n != 1 || !sent && n != 0 {
			t.Errorf("sent %v: %d C_AHOY Cancel Call to B", sent, n)
		}
		p.answer()
		p.run(20)
		if p.calls() != 0 {
			t.Errorf("sent %v: the cancelled call started: %+v", sent, p.events)
		}
		for _, e := range p.events {
			if e.Kind == trunk.TSCCRefused {
				t.Errorf("sent %v: %+v", sent, e)
			}
		}
	}
}

func TestTSCC_CancelCallAfterGrant(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	p.random(enums.ServiceKindIndividualVoice)
	p.run(20)
	p.answer()
	p.run(20)
	if p.calls() != 1 {
		t.Fatalf("no call: %+v", p.events)
	}
	p.events = p.events[:0]
	p.random(enums.ServiceKindCancelCall)
	p.run(20)
	if n := p.acks(enums.AckReasonCallCancelRefused); n != 1 {
		t.Errorf("%d C_NACKD Call_Cancel_Refused to A, want 1", n)
	}
	if len(p.events) != 1 || p.events[0].Kind != trunk.TSCCRefused || p.events[0].Address != cancelCaller || p.events[0].Ack != enums.AckReasonCallCancelRefused {
		t.Errorf("events %+v", p.events)
	}
	if p.ahoys(enums.ServiceKindCancelCall) != 0 {
		t.Error("C_AHOY Cancel Call to B after the grant")
	}
}

func TestTSCC_CancelCallWithoutSetup(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	for range 2 {
		p.random(enums.ServiceKindCancelCall)
		p.run(20)
	}
	if n := p.acks(enums.AckReasonMessageAccepted); n != 2 {
		t.Errorf("%d C_ACKD Message_Accepted to A, want 2", n)
	}
	if len(p.events) != 0 {
		t.Errorf("events %+v", p.events)
	}
}

func TestTSCC_CancelDuplexCall(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	p.random(enums.ServiceKindDuplexVoice)
	p.run(20)
	if p.ahoys(enums.ServiceKindDuplexVoice) == 0 {
		t.Fatal("no C_AHOY to B")
	}
	p.random(enums.ServiceKindCancelCall)
	p.run(20)
	if n := p.acks(enums.AckReasonMessageAccepted); n != 1 {
		t.Errorf("%d C_ACKD Message_Accepted to A, want 1", n)
	}
	if n := p.ahoys(enums.ServiceKindCancelCall); n != 1 {
		t.Errorf("%d C_AHOY Cancel Call to B, want 1", n)
	}
	p.answer()
	p.run(20)
	if p.calls() != 0 {
		t.Errorf("the cancelled call started: %+v", p.events)
	}
}

func cancelRig(t *testing.T) (*msRig, layer2.RawBurst) {
	t.Helper()
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	a := aloha(t)
	r.feed(a, &idle)
	if err := r.ms.Call(r.now, false, 3120002, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	return r, a
}

func (r *msRig) cancelRandom(a layer2.RawBurst) {
	r.t.Helper()
	for range 50 {
		out := r.feed(a, &idle)
		if len(out) == 0 {
			continue
		}
		if len(out) != 1 || out[0].Kind != trunk.OutputBurst {
			r.t.Fatalf("outputs %v before the Cancel Call C_RAND", kinds(out))
		}
		ra, ok := sent(r.t, &out[0]).AsRandomAccess()
		if !ok || ra.ServiceKind != uint8(enums.ServiceKindCancelCall) || ra.TargetAddress != 3120002 || ra.SourceAddress != msAddr {
			r.t.Fatalf("C_RAND %v %+v", ok, ra)
		}
		return
	}
	r.t.Fatal("no Cancel Call C_RAND")
}

func TestMS_CancelBeforeRandomAccess(t *testing.T) {
	t.Parallel()
	r, a := cancelRig(t)
	out := r.ms.Cancel(r.now, r.out[:0])
	wantKinds(t, "Cancel", out, trunk.OutputRefused)
	if out[0].Reason != trunk.ReasonCancelled || r.ms.Busy() {
		t.Errorf("refused %+v, busy %v", out[0], r.ms.Busy())
	}
	for range 20 {
		wantKinds(t, "after Cancel", r.feed(a, &idle))
	}
}

func TestMS_CancelCall(t *testing.T) {
	t.Parallel()
	for _, acked := range []bool{false, true} {
		r, a := cancelRig(t)
		wantKinds(t, "call C_RAND", r.feed(a, &idle), trunk.OutputBurst)
		if acked {
			wantKinds(t, "C_WACKD", r.feed(ack(t, enums.AckReasonWait), &idle))
		}
		wantKinds(t, "Cancel", r.ms.Cancel(r.now, r.out[:0]))
		if !r.ms.Busy() {
			t.Error("not busy while the cancellation runs")
		}
		r.cancelRandom(a)
		wantKinds(t, "late C_WACKD", r.feed(ack(t, enums.AckReasonWait), &idle))
		r.cancelRandom(a)
		out := r.feed(ack(t, enums.AckReasonMessageAccepted), &idle)
		wantKinds(t, "C_ACKD", out, trunk.OutputRefused)
		if out[0].Reason != trunk.ReasonCancelled || out[0].Ack != enums.AckReasonMessageAccepted || r.ms.Busy() {
			t.Errorf("acked %v: refused %+v, busy %v", acked, out[0], r.ms.Busy())
		}
	}
}

func TestMS_CancelCallRefused(t *testing.T) {
	t.Parallel()
	r, a := cancelRig(t)
	r.feed(a, &idle)
	r.feed(ack(t, enums.AckReasonWait), &idle)
	r.ms.Cancel(r.now, r.out[:0])
	r.cancelRandom(a)
	wantKinds(t, "Call_Cancel_Refused", r.feed(ack(t, enums.AckReasonCallCancelRefused), &idle))
	if !r.ms.Busy() {
		t.Error("not busy after Call_Cancel_Refused")
	}
	for range 10 {
		wantKinds(t, "after Call_Cancel_Refused", r.feed(a, &idle))
	}
	pv := bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetPrivateVoiceGrant, pdu.PrivateVoiceGrantPDU{
		PhysicalChannel: 8, TargetAddress: 3120002, SourceAddress: msAddr}), testCC)
	wantKinds(t, "PV_GRANT", r.feed(pv, &idle), trunk.OutputGrant, trunk.OutputTune)
	if r.ms.Busy() {
		t.Error("busy on the payload channel")
	}
}

func TestMS_CancelledCall(t *testing.T) {
	t.Parallel()
	const caller = 3120002
	r := newRig(t, &trunk.MSConfig{Address: msAddr, NoRegistration: true})
	r.feed(aloha(t), &idle)
	ahoy := func(kind enums.ServiceKind) layer2.RawBurst {
		return bsBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAhoy, pdu.AhoyPDU{
			ServiceKind: uint8(kind), TargetAddress: msAddr, SourceAddress: caller}), testCC)
	}
	wantKinds(t, "C_AHOY", r.feed(ahoy(enums.ServiceKindIndividualVoice), &busy), trunk.OutputAhoy, trunk.OutputBurst)
	out := r.feed(ahoy(enums.ServiceKindCancelCall), &busy)
	wantKinds(t, "C_AHOY Cancel Call", out, trunk.OutputAhoy, trunk.OutputBurst)
	if g := out[0].Grant; g.Service != enums.ServiceKindCancelCall || g.Target != msAddr || g.Source != caller {
		t.Errorf("Ahoy %+v", g)
	}
	ackIn, ok := sent(t, &out[1]).AsAckInbound()
	if !ok || enums.AckReason(ackIn.ReasonCode) != enums.AckReasonMSAccepted || ackIn.TargetAddress != caller || ackIn.AdditionalInfo != msAddr {
		t.Fatalf("C_ACKU %v %+v", ok, ackIn)
	}
}

func TestTSCC_CancelCallDuringSupplementaryData(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.Reg, cfg.Supplementary = false, true
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := &cancelProbe{t: t, ts: ts}
	p.run(1)
	p.inbound(func(c *pdu.CSBK) error {
		return c.SetRandomAccess(pdu.RandomAccessPDU{ServiceKind: uint8(enums.ServiceKindIndividualVoice), ServiceOptions: 0x10,
			TargetAddress: cancelCalled, SourceAddress: cancelCaller})
	})
	p.run(5)
	p.random(enums.ServiceKindCancelCall)
	p.run(int(40 * time.Second / (30 * time.Millisecond)))
	if n := p.acks(enums.AckReasonMessageAccepted); n != 1 {
		t.Errorf("%d C_ACKD Message_Accepted to A, want 1", n)
	}
	if n := p.ahoys(enums.ServiceKindIndividualVoice); n != 0 {
		t.Errorf("%d C_AHOY call to B after the cancellation", n)
	}
	p.answer()
	p.run(20)
	if p.calls() != 0 {
		t.Errorf("the cancelled call started: %+v", p.events)
	}
}

func TestTSCC_CancelRepeatedDuplexCall(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	for range 2 {
		p.random(enums.ServiceKindDuplexVoice)
		p.run(20)
	}
	if n := p.ahoys(enums.ServiceKindDuplexVoice); n != 1 {
		t.Errorf("%d C_AHOY to B for a repeated C_RAND, want 1", n)
	}
	p.random(enums.ServiceKindCancelCall)
	p.run(20)
	p.answer()
	p.run(int(time.Minute / (30 * time.Millisecond)))
	if p.calls() != 0 || len(p.events) != 0 {
		t.Errorf("events after the cancellation: %+v", p.events)
	}
}

func TestTSCC_AnswerWaitsForItsAhoy(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	p.random(enums.ServiceKindIndividualVoice)
	p.run(20)
	p.random(enums.ServiceKindCancelCall)
	p.random(enums.ServiceKindIndividualVoice)
	p.answer()
	if p.calls() != 0 {
		t.Fatalf("an answer before the C_AHOY went out started the call: %+v", p.events)
	}
	p.run(20)
	if n := p.ahoys(enums.ServiceKindIndividualVoice); n != 2 {
		t.Fatalf("%d C_AHOY call to B, want 2", n)
	}
	p.answer()
	p.run(20)
	if p.calls() != 1 {
		t.Errorf("the new call did not start: %+v", p.events)
	}
}

func TestMS_RepeatedCallCancelRefused(t *testing.T) {
	t.Parallel()
	r, a := cancelRig(t)
	r.feed(a, &idle)
	r.feed(ack(t, enums.AckReasonWait), &idle)
	r.ms.Cancel(r.now, r.out[:0])
	r.cancelRandom(a)
	r.cancelRandom(a)
	wantKinds(t, "first Call_Cancel_Refused", r.feed(ack(t, enums.AckReasonCallCancelRefused), &idle))
	wantKinds(t, "second Call_Cancel_Refused", r.feed(ack(t, enums.AckReasonCallCancelRefused), &idle))
	if !r.ms.Busy() {
		t.Error("not busy after Call_Cancel_Refused")
	}
}

func TestTSCC_CancelReplacedDuplexCall(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	p.random(enums.ServiceKindDuplexVoice)
	p.random(enums.ServiceKindDuplexPacketData)
	p.random(enums.ServiceKindCancelCall)
	p.run(40)
	if n := p.ahoys(enums.ServiceKindDuplexVoice) + p.ahoys(enums.ServiceKindDuplexPacketData) + p.ahoys(enums.ServiceKindCancelCall); n != 0 {
		t.Errorf("%d C_AHOY to B after the cancellation", n)
	}
	if n := p.acks(enums.AckReasonMessageAccepted); n != 1 {
		t.Errorf("%d C_ACKD Message_Accepted to A, want 1", n)
	}
}

func TestTSCC_CancelAckIsNotAStatusAnswer(t *testing.T) {
	t.Parallel()
	p := newCancelProbe(t)
	p.random(enums.ServiceKindIndividualVoice)
	p.run(20)
	p.random(enums.ServiceKindCancelCall)
	p.inbound(func(c *pdu.CSBK) error {
		return c.SetRandomAccess(pdu.RandomAccessPDU{ServiceKind: uint8(enums.ServiceKindStatusTransport), ServiceOptions: 0x01,
			TargetAddress: cancelCalled, SourceAddress: cancelCaller})
	})
	p.run(2)
	p.events = p.events[:0]
	p.answer()
	p.run(20)
	for _, e := range p.events {
		if e.Kind == trunk.TSCCStatus {
			t.Errorf("B's C_ACKU to the C_AHOY Cancel Call delivered the status: %+v", e)
		}
	}
}
