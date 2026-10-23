package trunk_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
)

func TestTSCC_AnswersARepeatedRandomAccess(t *testing.T) {
	t.Parallel()
	for _, kind := range []enums.ServiceKind{enums.ServiceKindIndividualVoice, enums.ServiceKindStatusTransport} {
		cfg := tsccConfig()
		cfg.Reg = false
		ts, err := trunk.NewTSCC(cfg)
		if err != nil {
			t.Fatal(err)
		}
		ra := msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceKind: uint8(kind), TargetAddress: 3120002, SourceAddress: 3120001}))
		now := time.Duration(0)
		waits := func() int {
			n := 0
			for range 20 {
				for _, o := range ts.Tick(now, nil) {
					b, err := layer2.NewBurstFromBytes(o.Burst)
					if o.Kind != trunk.TSCCBurst || o.Channel != tsccChannel || err != nil {
						continue
					}
					if c, ok := b.Data.(*pdu.CSBK); ok {
						if a, ok := c.AsAckOutbound(); ok && a.TargetAddress == 3120001 && enums.AckReason(a.ReasonCode) == enums.AckReasonWait {
							n++
						}
					}
				}
				now += 30 * time.Millisecond
			}
			return n
		}
		ts.Tick(now, nil)
		for try := 1; try <= 2; try++ {
			ts.Inbound(now, tsccChannel, &ra, layer2.Hint{Slot: enums.Slot1}, nil)
			if n := waits(); n == 0 {
				t.Errorf("%v: random access %d got no C_ACKD WAIT", kind, try)
			}
		}
	}
}

func TestTSCC_NewRandomAccessReplacesAPendingSetup(t *testing.T) {
	t.Parallel()
	for _, kind := range []enums.ServiceKind{enums.ServiceKindIndividualVoice, enums.ServiceKindStatusTransport} {
		for _, sent := range []bool{false, true} {
			replaceSetup(t, kind, sent)
		}
	}
}

type setupProbe struct {
	ts     *trunk.TSCC
	now    time.Duration
	events []trunk.TSCCOutput
	waits  int
	ahoys  map[uint32]int
	toB    int
}

func (p *setupProbe) run(n int) {
	for range n {
		for _, o := range p.ts.Tick(p.now, nil) {
			if o.Kind != trunk.TSCCBurst {
				p.events = append(p.events, o)
				continue
			}
			if bu, err := layer2.NewBurstFromBytes(o.Burst); err == nil {
				if cs, ok := bu.Data.(*pdu.CSBK); ok {
					p.scan(cs)
				}
			}
		}
		p.now += 30 * time.Millisecond
	}
}

func (p *setupProbe) scan(cs *pdu.CSBK) {
	const a, b = 3120001, 3120002
	if x, ok := cs.AsAckOutbound(); ok {
		if x.TargetAddress == a && enums.AckReason(x.ReasonCode) == enums.AckReasonWait {
			p.waits++
		}
		if x.TargetAddress == b {
			p.toB++
		}
	}
	if x, ok := cs.AsAhoy(); ok && x.SourceAddress == a {
		p.ahoys[x.TargetAddress]++
	}
	if x, ok := cs.AsPrivateVoiceGrant(); ok && (x.TargetAddress == b || x.SourceAddress == b) {
		p.toB++
	}
}

func (p *setupProbe) inbound(raw layer2.RawBurst) {
	p.events = p.ts.Inbound(p.now, tsccChannel, &raw, layer2.Hint{Slot: enums.Slot1}, p.events)
}

func replaceSetup(t *testing.T, kind enums.ServiceKind, sent bool) {
	t.Helper()
	const a, b, c = 3120001, 3120002, 3120003
	cfg := tsccConfig()
	cfg.Reg = false
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ra := func(target uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceKind: uint8(kind), TargetAddress: target, SourceAddress: a}))
	}
	answer := func(from uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: a, AdditionalInfo: from}))
	}
	p := &setupProbe{ts: ts, ahoys: map[uint32]int{}}
	p.run(1)
	p.inbound(ra(b))
	want := 2
	if sent {
		p.run(20)
		if p.ahoys[b] == 0 {
			t.Fatalf("%v: no C_AHOY to B", kind)
		}
		want = 1
	}
	p.waits, p.ahoys[b], p.toB, p.events = 0, 0, 0, p.events[:0]
	p.inbound(ra(c))
	p.run(20)
	if p.waits < want || p.ahoys[c] == 0 {
		t.Errorf("%v sent %v: new request got %d C_ACKD WAIT and %d C_AHOY to C", kind, sent, p.waits, p.ahoys[c])
	}
	p.inbound(answer(b))
	p.run(20)
	p.inbound(answer(c))
	p.run(20)
	if p.ahoys[b] != 0 || p.toB != 0 {
		t.Errorf("%v sent %v: B got %d C_AHOY and %d other PDUs after the new request", kind, sent, p.ahoys[b], p.toB)
	}
	got := 0
	for _, e := range p.events {
		if e.Grant.Target == b || e.Grant.Source == b {
			t.Errorf("%v sent %v: output for B %+v", kind, sent, e)
		}
		if (e.Kind == trunk.TSCCCallStart || e.Kind == trunk.TSCCStatus) && e.Grant.Target == c {
			got++
		}
	}
	if got == 0 {
		t.Errorf("%v sent %v: no call or status to C, events %+v", kind, sent, p.events)
	}
}

func TestTSCC_LateAnswerGrantsOnlyItsCaller(t *testing.T) {
	t.Parallel()
	const a, b, c, d = 3120001, 3120002, 3120003, 3120004
	cfg := tsccConfig()
	cfg.Reg = false
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ra := func(src, target uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetRandomAccess, pdu.RandomAccessPDU{
			ServiceKind: uint8(enums.ServiceKindIndividualVoice), TargetAddress: target, SourceAddress: src}))
	}
	answer := func(to uint32) layer2.RawBurst {
		return msBurst(t, withPDU(t, &pdu.CSBK{LastBlock: true}, (*pdu.CSBK).SetAckInbound, pdu.AckInboundPDU{
			ReasonCode: byte(enums.AckReasonMSAccepted), TargetAddress: to, AdditionalInfo: b}))
	}
	p := &setupProbe{ts: ts, ahoys: map[uint32]int{}}
	p.run(1)
	for _, r := range []layer2.RawBurst{ra(a, b), ra(d, b), ra(a, c)} {
		p.inbound(r)
		p.run(20)
	}
	p.events = p.events[:0]
	p.inbound(answer(a))
	p.run(20)
	for _, e := range p.events {
		if e.Kind == trunk.TSCCCallStart {
			t.Errorf("answer to the cancelled call of A started %+v", e.Grant)
		}
	}
	p.inbound(answer(d))
	p.run(20)
	if len(p.events) == 0 || p.events[0].Kind != trunk.TSCCCallStart || p.events[0].Grant.Source != d || p.events[0].Grant.Target != b {
		t.Errorf("answer to D: events %+v", p.events)
	}
}

func TestTSCC_UploadDuringDownloadWaitsForIt(t *testing.T) {
	t.Parallel()
	for _, poll := range []bool{false, true} {
		s := udtSim(t)
		a, b := s.ms[0], s.ms[1]
		var text, long trunk.Appended
		if err := text.SetText(enums.UDTISO8, []byte("trunked short data")); err != nil {
			t.Fatal(err)
		}
		if err := long.SetText(enums.UDTISO8, bytes.Repeat([]byte{'x'}, 40)); err != nil {
			t.Fatal(err)
		}
		var err error
		if poll {
			b.ms.SetPollResponse(&text)
			err = a.ms.PollShortData(s.now, 3120002, enums.UDTISO8, 1)
		} else {
			err = a.ms.SendShortData(s.now, false, 3120002, &text, layer3.ShortDataServiceOptions{})
		}
		if err != nil {
			t.Fatal(err)
		}
		uploader := a
		if poll {
			uploader = b
		}
		s.until(5*time.Second, func() bool {
			for _, o := range uploader.log {
				if o.Kind == trunk.OutputBurst && o.Burst.SlotType().DataType == enums.DataTypeDataHeader {
					return true
				}
			}
			return false
		})
		if err := s.tscc.SendShortData(s.now, false, 3120003, &long); err != nil {
			t.Fatalf("poll %v: gateway download: %v", poll, err)
		}
		start := s.now
		s.until(8*time.Second, func() bool {
			_, d := has(a.log, trunk.OutputDelivered)
			_, r := has(a.log, trunk.OutputRefused)
			return d || r
		})
		if r, ok := has(a.log, trunk.OutputRefused); ok {
			t.Fatalf("poll %v: refused %v after %v", poll, r.Ack, s.now-start)
		}
		if _, ok := has(a.log, trunk.OutputDelivered); !ok || s.now-start > 2*time.Second {
			t.Errorf("poll %v: delivered %v after %v, want within 2s", poll, ok, s.now-start)
		}
	}
}

func TestTSCC_HeldUploadDiesWithItsTransaction(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.PowerSave = true
	tg := [trunk.MaxTalkgroups]uint32{9}
	s := newSim(t, cfg,
		&trunk.MSConfig{Address: 3120001, Talkgroups: tg, TalkgroupsLen: 1, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Talkgroups: tg, TalkgroupsLen: 1, Seed: 2},
		&trunk.MSConfig{Address: 3120003, Seed: 3, PowerSave: 7})
	s.until(10*time.Second, func() bool {
		for _, m := range s.ms {
			if !m.ms.Registered() {
				return false
			}
		}
		return true
	})
	s.run(10 * time.Second)
	a, b := s.ms[0], s.ms[1]
	var first, second, long trunk.Appended
	if err := first.SetText(enums.UDTISO8, []byte("FIRST message")); err != nil {
		t.Fatal(err)
	}
	if err := second.SetText(enums.UDTISO8, []byte("SECOND message")); err != nil {
		t.Fatal(err)
	}
	if err := long.SetText(enums.UDTISO8, bytes.Repeat([]byte{'x'}, 40)); err != nil {
		t.Fatal(err)
	}
	if err := s.tscc.SendShortData(s.now, false, 3120003, &long); err != nil {
		t.Fatal(err)
	}
	refused := false
	msg := &first
	for end := s.now + 90*time.Second; s.now < end; {
		a.log, b.log = nil, nil
		if err := a.ms.SendShortData(s.now, false, 3120002, msg, layer3.ShortDataServiceOptions{}); err != nil {
			s.run(time.Second)
			continue
		}
		s.until(20*time.Second, func() bool {
			_, d := has(a.log, trunk.OutputDelivered)
			_, r := has(a.log, trunk.OutputRefused)
			return d || r
		})
		if _, ok := has(b.log, trunk.OutputUDT); ok {
			got := udtOf(b.ms.UDT()).Appended()
			if text, _ := got.Text(nil); refused && string(text) != "SECOND message" {
				t.Fatalf("B got %q after the first message was refused", text)
			}
		}
		if _, ok := has(a.log, trunk.OutputDelivered); ok {
			t.Logf("delivered at %v, first refused %v", s.now, refused)
			return
		}
		refused, msg = true, &second
	}
	t.Fatal("never delivered")
}
