package trunk_test

import (
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/trunk"
	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

const tsccChannel = 1

type simMS struct {
	ms      *trunk.MS
	channel uint16
	log     []trunk.Output
	key     *simKey
}

type simKey struct {
	k   [auth.KeyLen]byte
	psn uint32
}

type simInbound struct {
	due  time.Duration
	ch   uint16
	slot enums.Slot
	raw  layer2.RawBurst
}

type sim struct {
	t       *testing.T
	tscc    *trunk.TSCC
	ms      []*simMS
	now     time.Duration
	pending []simInbound
	events  []trunk.TSCCOutput
	tout    []trunk.TSCCOutput
	mout    []trunk.Output
	relayed int
	keys    map[uint32]simKey
}

func newSim(t *testing.T, cfg *trunk.TSCCConfig, ms ...*trunk.MSConfig) *sim {
	t.Helper()
	ts, err := trunk.NewTSCC(cfg)
	if err != nil {
		t.Fatal(err)
	}
	s := &sim{t: t, tscc: ts, tout: make([]trunk.TSCCOutput, 0, trunk.MaxTSCCOutputs), mout: make([]trunk.Output, 0, trunk.MaxOutputs)}
	for _, c := range ms {
		m, err := trunk.NewMS(c)
		if err != nil {
			t.Fatal(err)
		}
		s.ms = append(s.ms, &simMS{ms: m, channel: tsccChannel})
	}
	return s
}

func (s *sim) msOutputs(m *simMS, out []trunk.Output) {
	for i := range out {
		o := out[i]
		m.log = append(m.log, o)
		switch o.Kind {
		case trunk.OutputTune:
			m.channel = o.Tune.Channel
			if m.channel == trunk.ChannelNull {
				m.channel = tsccChannel
			}
		case trunk.OutputBurst:
			s.pending = append(s.pending, simInbound{due: o.Due, ch: m.channel, slot: o.Slot, raw: o.Burst})
		case trunk.OutputAuthChallenge:
			if m.key != nil {
				s.msOutputs(m, m.ms.AuthResponse(s.now, auth.Response(o.Challenge, m.key.k, m.key.psn), nil))
			}
		default:
		}
	}
}

func (s *sim) inject(m *simMS, raw layer2.RawBurst, slot enums.Slot) {
	s.pending = append(s.pending, simInbound{due: s.now, ch: m.channel, slot: slot, raw: raw})
}

func (s *sim) step() {
	keep := s.pending[:0]
	var due []simInbound
	for _, p := range s.pending {
		if p.due <= s.now {
			due = append(due, p)
		} else {
			keep = append(keep, p)
		}
	}
	s.pending = keep
	for i, p := range due {
		collided := false
		for j, q := range due {
			if i != j && q.ch == p.ch && q.slot == p.slot && q.due == p.due {
				collided = true
			}
		}
		if collided {
			continue
		}
		s.tout = s.tscc.Inbound(s.now, p.ch, p.slot, &p.raw, layer2.Hint{}, s.tout[:0])
		s.record()
	}
	s.tout = s.tscc.Tick(s.now, s.tout[:0])
	bursts := append([]trunk.TSCCOutput(nil), s.tout...)
	s.record()
	for _, m := range s.ms {
		s.mout = m.ms.Tick(s.now, s.mout[:0])
		s.msOutputs(m, s.mout)
	}
	for _, b := range bursts {
		if b.Kind != trunk.TSCCBurst {
			continue
		}
		if b.Channel != tsccChannel {
			if d, err := layer2.NewBurstFromBytes(b.Burst); err == nil && d.SyncPattern == enums.SyncBSSourcedData && d.SlotType.DataType == enums.DataTypeVoiceLCHeader {
				s.relayed++
			}
		}
		for _, m := range s.ms {
			if m.channel != b.Channel {
				continue
			}
			s.mout = m.ms.Burst(s.now, &b.Burst, layer2.Hint{Slot: b.Slot}, &b.CACH, s.mout[:0])
			if len(s.mout) > trunk.MaxOutputs {
				s.t.Fatalf("%d MS outputs", len(s.mout))
			}
			s.msOutputs(m, s.mout)
		}
	}
	s.now += 30 * time.Millisecond
}

func (s *sim) record() {
	if len(s.tout) > trunk.MaxTSCCOutputs {
		s.t.Fatalf("%d TSCC outputs, MaxOutputs %d", len(s.tout), trunk.MaxTSCCOutputs)
	}
	for _, o := range s.tout {
		if o.Kind != trunk.TSCCBurst {
			s.events = append(s.events, o)
		}
		if k, ok := s.keys[o.Address]; ok && o.Kind == trunk.TSCCAuthChallenge {
			s.tscc.ExpectAuth(o.Address, auth.Response(o.Challenge, k.k, k.psn))
		}
	}
}

func (s *sim) run(d time.Duration) {
	for end := s.now + d; s.now < end; {
		s.step()
	}
}

func (s *sim) until(d time.Duration, done func() bool) {
	s.t.Helper()
	for end := s.now + d; s.now < end; {
		s.step()
		if done() {
			return
		}
	}
	s.t.Fatalf("condition not met after %v at %v", d, s.now)
}

func (s *sim) event(kind trunk.TSCCOutputKind) (trunk.TSCCOutput, bool) {
	for _, e := range s.events {
		if e.Kind == kind {
			return e, true
		}
	}
	return trunk.TSCCOutput{}, false
}

func has(log []trunk.Output, kind trunk.OutputKind) (trunk.Output, bool) {
	for _, o := range log {
		if o.Kind == kind {
			return o, true
		}
	}
	return trunk.Output{}, false
}

func voiceLC(t *testing.T, dt enums.DataType) layer2.RawBurst {
	t.Helper()
	lc, err := pdu.NewVoiceLC(true, 9, 3120001, layer3.ServiceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lc.BurstDataType = dt
	raw, err := layer2.BuildBurst(&lc, testCC, layer2.SyncSourceMS)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func tsccConfig() *trunk.TSCCConfig {
	return &trunk.TSCCConfig{SysCode: testSys, ColorCode: testCC, Channel: tsccChannel, Payload: [trunk.MaxPayloadChannels]uint16{10, 11}, PayloadLen: 2,
		Reg: true, NRandWait: 3, Backoff: 4, Hangtime: time.Second}
}

func TestTSCC_EndToEnd(t *testing.T) {
	t.Parallel()
	tg := [trunk.MaxTalkgroups]uint32{9}
	s := newSim(t, tsccConfig(),
		&trunk.MSConfig{Address: 3120001, Talkgroups: tg, TalkgroupsLen: 1, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Talkgroups: tg, TalkgroupsLen: 1, Seed: 2},
		&trunk.MSConfig{Address: 3120003, Seed: 3},
		&trunk.MSConfig{Address: 3120004, Seed: 4, NoRegistration: true})
	a, b, c := s.ms[0], s.ms[1], s.ms[2]

	s.until(5*time.Second, func() bool { return a.ms.Registered() && b.ms.Registered() && c.ms.Registered() })
	for _, addr := range []uint32{3120001, 3120002, 3120003} {
		if !s.tscc.Registered(addr) {
			t.Errorf("TSCC has no registration of %d", addr)
		}
	}
	if s.tscc.Registered(3120004) {
		t.Error("TSCC registered the MS with NoRegistration")
	}

	if err := a.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && b.ms.State() == trunk.StatePayload })
	start, ok := s.event(trunk.TSCCCallStart)
	if !ok || !start.Call.Group || start.Call.Target != 9 || start.Call.Source != 3120001 {
		t.Fatalf("TSCCCallStart %+v %v", start, ok)
	}
	if a.channel != start.Call.Channel || b.channel != start.Call.Channel || c.channel != tsccChannel {
		t.Errorf("channels a %d b %d c %d, grant %d", a.channel, b.channel, c.channel, start.Call.Channel)
	}
	if _, ok := has(c.log, trunk.OutputGrant); !ok {
		t.Error("C saw no grant")
	}
	s.inject(a, voiceLC(t, enums.DataTypeVoiceLCHeader), start.Call.Slot)
	s.run(60 * time.Millisecond)
	s.inject(a, voiceLC(t, enums.DataTypeTerminatorWithLC), start.Call.Slot)
	s.until(3*time.Second, func() bool { _, ok := s.event(trunk.TSCCCallEnd); return ok })
	end, _ := s.event(trunk.TSCCCallEnd)
	if s.relayed != 1 {
		t.Errorf("%d relayed Voice LC Headers, want 1", s.relayed)
	}
	if end.Reason != trunk.ReasonHangtime || end.Call.Target != 9 {
		t.Errorf("TSCCCallEnd %+v", end)
	}
	s.run(300 * time.Millisecond)
	for _, m := range []*simMS{a, b} {
		ce, ok := has(m.log, trunk.OutputCallEnd)
		if !ok || ce.Reason != trunk.ReasonClear || m.ms.State() != trunk.StateControl || m.channel != tsccChannel {
			t.Errorf("after P_CLEAR: %+v %v, state %v, channel %d", ce, ok, m.ms.State(), m.channel)
		}
	}

	individualCall(t, s)
	refusedAndPowerOff(t, s)
}

func individualCall(t *testing.T, s *sim) {
	t.Helper()
	a, c := s.ms[0], s.ms[2]
	s.events = nil
	a.log, c.log = nil, nil
	if err := a.ms.Call(s.now, false, 3120003, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && c.ms.State() == trunk.StatePayload })
	if _, ok := has(c.log, trunk.OutputAhoy); !ok {
		t.Error("C saw no C_AHOY")
	}
	if a.channel != c.channel {
		t.Errorf("a on %d, c on %d", a.channel, c.channel)
	}
	s.mout = a.ms.EndCall(s.now, s.mout[:0])
	s.msOutputs(a, s.mout)
	s.until(2*time.Second, func() bool { _, ok := s.event(trunk.TSCCCallEnd); return ok && c.ms.State() == trunk.StateControl })
	end, _ := s.event(trunk.TSCCCallEnd)
	if end.Reason != trunk.ReasonMaintenance || end.Call.Group || end.Call.Target != 3120003 {
		t.Errorf("TSCCCallEnd %+v", end)
	}
}

func refusedAndPowerOff(t *testing.T, s *sim) {
	t.Helper()
	b, d := s.ms[1], s.ms[3]

	s.events = nil
	d.log = nil
	if err := d.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(d.log, trunk.OutputRefused); return ok })
	if r, _ := has(d.log, trunk.OutputRefused); r.Reason != trunk.ReasonNack || r.Ack != enums.AckReasonMSNotRegistered {
		t.Errorf("unregistered call: %+v", r)
	}

	b.log = nil
	s.mout = b.ms.PowerOff(s.now, s.mout[:0])
	s.msOutputs(b, s.mout)
	s.until(3*time.Second, func() bool { return b.ms.Off() })
	if po, _ := has(b.log, trunk.OutputPowerOff); po.Reason != trunk.ReasonNone || s.tscc.Registered(3120002) {
		t.Errorf("power off %+v, still registered %v", po, s.tscc.Registered(3120002))
	}
}

func TestTSCC_NoAnswerAndBusy(t *testing.T) {
	t.Parallel()
	cfg := tsccConfig()
	cfg.PayloadLen = 1
	cfg.Reg = false
	cfg.AnswerTimeout = time.Second
	s := newSim(t, cfg,
		&trunk.MSConfig{Address: 3120001, Seed: 1},
		&trunk.MSConfig{Address: 3120002, Seed: 2},
		&trunk.MSConfig{Address: 3120003, Seed: 3})
	a, b, c := s.ms[0], s.ms[1], s.ms[2]
	s.run(time.Second)
	if err := a.ms.Call(s.now, false, 7777, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(a.log, trunk.OutputRefused); return ok })
	if r, _ := has(a.log, trunk.OutputRefused); r.Ack != enums.AckReasonMSAwayRefused {
		t.Errorf("no answer: %+v", r)
	}
	_ = a.ms.Call(s.now, true, 9, layer3.VoiceServiceOptions{})
	_ = b.ms.Call(s.now, true, 10, layer3.VoiceServiceOptions{})
	s.until(3*time.Second, func() bool { return a.ms.State() == trunk.StatePayload && b.ms.State() == trunk.StatePayload })
	if err := c.ms.Call(s.now, true, 11, layer3.VoiceServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	s.until(3*time.Second, func() bool { _, ok := has(c.log, trunk.OutputRefused); return ok })
	if r, _ := has(c.log, trunk.OutputRefused); r.Ack != enums.AckReasonSysBusyRefused {
		t.Errorf("busy: %+v", r)
	}
}
