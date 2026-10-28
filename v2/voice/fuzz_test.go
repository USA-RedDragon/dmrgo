package voice_test

import (
	"slices"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

const (
	maxScriptOps = 512
	maxScript    = 128
	maxRaw       = 4 * 33
	maxAlias     = 0x1F
	maxFrames    = 128
	maxDrop      = 128
)

type opKind uint8

const (
	opBurst opKind = iota
	opDecoded
	opLost
	opTick
	opExpect
	opReset
)

type scriptOp struct {
	kind  opKind
	at    time.Duration
	raw   layer2.RawBurst
	hint  layer2.Hint
	call  voice.Call
	until time.Duration
}

type script struct {
	cfg    voice.ReceiverConfig
	cc     enums.ColorCode
	txCfg  voice.TransmitterConfig
	ops    []scriptOp
	badCfg bool
}

type byteReader struct {
	data []byte
	i    int
}

func (r *byteReader) next() byte {
	if r.i >= len(r.data) {
		return 0
	}
	r.i++
	return r.data[r.i-1]
}

func (r *byteReader) done() bool {
	return r.i >= len(r.data)
}

func step(d byte) time.Duration {
	switch {
	case d < 64:
		return slot
	case d < 128:
		return time.Duration(d-64) * time.Millisecond
	case d < 192:
		return slot * time.Duration(d-126)
	}
	return time.Duration(d-192) * 100 * time.Millisecond
}

func configTime(m, b byte) time.Duration {
	switch m & 3 {
	case 1:
		return -1
	case 2:
		return time.Duration(b)*10*time.Millisecond + time.Millisecond
	case 3:
		return time.Duration(b) * 100 * time.Millisecond
	}
	return 0
}

type pool struct {
	bursts []layer2.RawBurst
	starts []int
}

func newPool(t testing.TB) *pool {
	t.Helper()
	p := &pool{}
	add := func(b []layer2.RawBurst) {
		p.starts = append(p.starts, len(p.bursts))
		p.bursts = append(p.bursts, b...)
	}
	for _, name := range []string{"voice.bin", "parrot_kerchunk.bin", "h-sms.bin"} {
		add(loadBin(t, name))
	}
	weakIn := loadWeak(t)
	weak := make([]layer2.RawBurst, 0, len(weakIn))
	for _, in := range weakIn {
		weak = append(weak, in.raw)
	}
	add(weak)
	tx, err := voice.NewTransmitter(&voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: 1, Silence: vocoder.AMBESilenceSlot(), Headers: 2})
	if err != nil {
		t.Fatal(err)
	}
	gps := pdu.FLCGPSInfo{Latitude: 32.75, Longitude: -97.33}
	opts := &voice.CallOptions{PIHeader: &pdu.PIHeader{AlgorithmID: 1}, TalkerAlias: "KI5VMF Jacob", GPS: &gps}
	if err := tx.Start(0, voice.Call{Group: true, Target: 3100, Source: 3191868}, opts); err != nil {
		t.Fatal(err)
	}
	var out []voice.Output
	buf := make([]voice.Output, 0, voice.MaxOutputs)
	for i := 0; ; i++ {
		for tx.Room() > 0 && i < 40 {
			_ = tx.Frame(vocoder.AMBEFrame{byte(i), 0x55, byte(tx.Room()), 0, 0, 0, 0x80})
		}
		if i == 40 {
			tx.Stop(0)
		}
		at, ok := tx.Deadline()
		if !ok {
			break
		}
		buf = tx.Tick(at, buf[:0])
		out = append(out, buf...)
	}
	add(outBursts(out))
	return p
}

type rxInput struct {
	flags, cc, lost, hang, seqBits, start uint8
	ops, raw                              []byte
}

type scriptState struct {
	p      *pool
	cur    int
	now    time.Duration
	seq    uint16
	stream uint32
}

func parseScript(in *rxInput, p *pool) script {
	var s script
	flags, cc := in.flags, in.cc
	if flags&1 != 0 {
		s.cc = enums.ColorCode(cc)
		s.cfg.ColorCode = &s.cc
		s.badCfg = s.cc > 15
	}
	s.cfg.Lost = configTime(flags>>2, in.lost)
	s.cfg.Hang = configTime(flags>>4, in.hang)
	s.txCfg = voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: enums.ColorCode(cc & 15), Source: layer2.SyncSource(flags >> 6),
		Silence: vocoder.AMBESilenceSlot(), Timeout: 10 * time.Second, Queue: voice.MaxQueue}

	r := byteReader{data: in.ops[:min(len(in.ops), maxScript)]}
	raws := byteReader{data: in.raw[:min(len(in.raw), maxRaw)]}
	st := scriptState{p: p, cur: p.starts[int(in.start)%len(p.starts)], stream: 0x1000}
	for !r.done() && len(s.ops) < maxScriptOps {
		code := r.next()
		reps := 1
		if code&0x40 != 0 {
			reps += int(r.next())
		}
		var raw layer2.RawBurst
		if code&7 == 2 {
			for i := range raw {
				raw[i] = raws.next()
			}
		}
		body := r.i
		for range reps {
			if len(s.ops) == maxScriptOps {
				break
			}
			r.i = body
			s.ops = append(s.ops, st.op(&r, code, &raw, in.seqBits))
		}
	}
	return s
}

func (st *scriptState) op(r *byteReader, code byte, raw *layer2.RawBurst, seqBits byte) scriptOp {
	st.now += step(r.next())
	op := scriptOp{at: st.now}
	hint := code&0x80 != 0
	switch code & 7 {
	case 0, 1, 6:
		a := r.next()
		if a < 0xE0 {
			st.cur += int(a >> 5)
		} else {
			st.cur = int(a&0x1F) * len(st.p.bursts) / 32
		}
		st.cur %= len(st.p.bursts)
		op.raw = st.p.bursts[st.cur]
		if code&0x08 != 0 {
			for range code>>4&3 + 1 {
				bit := (int(r.next())<<8 | int(r.next())) % 264
				op.raw[bit/8] ^= 0x80 >> (bit % 8)
			}
		}
		if code&7 == 6 {
			op.kind = opDecoded
		}
		op.hint = readHint(r, hint, &st.seq, &st.stream, seqBits)
	case 2:
		op.raw = *raw
		op.hint = readHint(r, hint, &st.seq, &st.stream, seqBits)
	case 3:
		op.kind = opLost
	case 4:
		op.kind = opTick
	case 5:
		op.kind = opExpect
		f := r.next()
		op.call = voice.Call{
			Group:   f&1 != 0,
			Target:  uint32(r.next())<<16 | uint32(r.next())<<8 | uint32(r.next()),
			Source:  uint32(r.next())<<16 | uint32(r.next())<<8 | uint32(r.next()),
			Options: layer3.DecodeServiceOptions([1]byte{r.next()}),
			Stream:  uint32(f),
		}
		op.until = st.now + step(r.next())
	case 7:
		op.kind = opReset
	}
	return op
}

func readHint(r *byteReader, present bool, seq *uint16, stream *uint32, seqBits byte) layer2.Hint {
	var h0, h1, h2 byte
	if present {
		h0, h1, h2 = r.next(), r.next(), r.next()
	}
	if h0&0x80 != 0 {
		*stream++
	}
	*seq += uint16(int8(h1))
	return layer2.Hint{
		HasSeq:      h0&1 != 0,
		Seq:         *seq,
		SeqBits:     seqBits,
		HasStream:   h0&2 != 0,
		Stream:      *stream,
		SyncLost:    h0&4 != 0,
		End:         layer2.EndHint(h0 >> 3 & 3),
		Kind:        layer2.Kind(h0 >> 5 & 3),
		VoiceBurst:  enums.VoiceBurstType(h2 & 7),
		HasDataType: h2&8 != 0,
		DataType:    enums.DataType(h2 >> 4),
	}
}

type scriptSeed struct {
	ops  []byte
	last []byte
	reps int
}

func (s *scriptSeed) flush() {
	switch {
	case s.reps == 1:
		s.ops = append(s.ops, s.last...)
	case s.reps > 1:
		s.ops = append(append(s.ops, s.last[0]|0x40, byte(s.reps-1)), s.last[1:]...)
	}
	s.last, s.reps = nil, 0
}

func (s *scriptSeed) op(b ...byte) *scriptSeed {
	if s.reps > 0 && s.reps < 256 && b[0]&7 != 2 && slices.Equal(b, s.last) {
		s.reps++
		return s
	}
	s.flush()
	s.last, s.reps = b, 1
	return s
}

func (s *scriptSeed) burst(skip, dt byte, h ...byte) {
	if len(h) == 0 {
		s.op(0, dt, skip<<5)
		return
	}
	h = append(h, 0, 0, 0)
	s.op(0x80, dt, skip<<5, h[0], h[1], h[2])
}

func (s *scriptSeed) done() []byte {
	s.flush()
	return s.ops
}

func seeds(p *pool) []rxInput {
	out := make([]rxInput, 0, 2*len(p.starts)+2)
	for part := range p.starts {
		end := len(p.bursts)
		if part+1 < len(p.starts) {
			end = p.starts[part+1]
		}
		n := end - p.starts[part]
		var plain, seqd scriptSeed
		plain.burst(0, 0)
		seqd.burst(0, 0, 1, 1)
		for range n - 1 {
			plain.burst(1, 0)
			seqd.burst(1, 0, 1, 1)
		}
		out = append(out, rxInput{start: byte(part), ops: plain.op(4, 255).done()},
			rxInput{flags: 0x01, cc: 1, start: byte(part), ops: seqd.done()})
	}
	var lossy scriptSeed
	lossy.burst(0, 0)
	for i := range 60 {
		switch {
		case i%11 == 3:
			lossy.burst(3, 128)
		case i%13 == 5:
			lossy.burst(0, 64)
		default:
			lossy.burst(1, 0, 0x03, 1)
		}
	}
	out = append(out, rxInput{flags: 0x14, lost: 20, hang: 10, seqBits: 8, ops: lossy.op(3, 0).op(4, 250).done()})
	var late scriptSeed
	late.burst(6, 0)
	for range 30 {
		late.burst(1, 0)
	}
	late.op(5, 0, 1, 0, 0, 91, 0x30, 0xB4, 0x3C, 0, 250)
	for range 12 {
		late.burst(1, 0)
	}
	out = append(out, rxInput{ops: late.op(7, 0).op(2, 0).done(), raw: []byte{0x30, 0x2A, 0x5D, 0x8A, 0x9F, 0x13, 0xE6}})
	return out
}

type eventChecker struct {
	open          bool
	call          voice.Call
	voiceInCall   int
	lostInCall    int
	lostSat       bool
	voiceSinceEnd int
	session       voice.Call
	maySessionEnd bool
}

func (c *eventChecker) reset() {
	*c = eventChecker{}
}

func sameParties(a, b *voice.Call) bool {
	return a.Group == b.Group && a.Target == b.Target && a.Source == b.Source
}

func (c *eventChecker) check(t *testing.T, rx *voice.Receiver, now time.Duration, events []voice.Event) {
	t.Helper()
	if len(events) > voice.MaxEvents {
		t.Fatalf("at %v: %d events, more than MaxEvents", now, len(events))
	}
	for i := range events {
		c.event(t, now, &events[i])
	}
	if open := rx.State() == voice.StateInCall; open != c.open {
		t.Fatalf("at %v: state %v, but the events leave a call open %v", now, rx.State(), c.open)
	}
	if at, ok := rx.Deadline(); ok && at <= now {
		t.Fatalf("at %v: Deadline %v is not in the future", now, at)
	}
}

func (c *eventChecker) member(t *testing.T, now time.Duration, e *voice.Event) {
	t.Helper()
	switch {
	case c.open && !sameParties(&e.Call, &c.call):
		t.Fatalf("at %v: %v of call %+v in call %+v", now, e.Kind, e.Call, c.call)
	case !c.open && e.Call.Origin != voice.OriginUnknown:
		t.Fatalf("at %v: %v of a call %+v with no call open", now, e.Kind, e.Call)
	}
}

func (c *eventChecker) event(t *testing.T, now time.Duration, e *voice.Event) {
	t.Helper()
	switch e.Kind {
	case voice.EventCallStart:
		c.start(t, now, e)
	case voice.EventVoice:
		c.member(t, now, e)
		if _, ok := e.Position.Index(); !ok && e.Position != enums.VoiceBurstUnknown {
			t.Fatalf("at %v: voice position %v out of range", now, e.Position)
		}
		c.voiceInCall++
		c.lostInCall += int(e.Lost)
		c.lostSat = c.lostSat || e.Lost == 0xFF
		c.voiceSinceEnd++
	case voice.EventEmbedded, voice.EventPIHeader:
		c.member(t, now, e)
	case voice.EventTalkerAlias:
		c.member(t, now, e)
		if len(e.Text) > pdu.TalkerAliasMaxText || !utf8.Valid(e.Text) {
			t.Fatalf("at %v: talker alias %q", now, e.Text)
		}
	case voice.EventCallEnd:
		c.end(t, now, e)
	case voice.EventSessionStart:
		if c.open || e.Call.Origin != voice.OriginTerminator {
			t.Fatalf("at %v: session start %+v, call open %v", now, e.Call, c.open)
		}
		c.session, c.maySessionEnd = e.Call, true
	case voice.EventSessionEnd:
		if c.open || !c.maySessionEnd || e.Call.Target != c.session.Target || e.Call.Group != c.session.Group {
			t.Fatalf("at %v: session end %+v, call open %v, session %+v open %v", now, e.Call, c.open, c.session, c.maySessionEnd)
		}
		c.maySessionEnd = false
	default:
		t.Fatalf("at %v: event kind %v", now, e.Kind)
	}
}

func (c *eventChecker) start(t *testing.T, now time.Duration, e *voice.Event) {
	t.Helper()
	if c.open {
		t.Fatalf("at %v: call start %+v while call %+v is open", now, e.Call, c.call)
	}
	switch e.Call.Origin {
	case voice.OriginHeader, voice.OriginEmbedded:
		target, source, ok := e.Call.LC.Addresses()
		if !ok || target != e.Call.Target || source != e.Call.Source {
			t.Fatalf("at %v: call %+v does not match its LC", now, e.Call)
		}
	case voice.OriginExpected:
	default:
		t.Fatalf("at %v: call start with origin %v", now, e.Call.Origin)
	}
	c.open, c.call, c.voiceInCall, c.lostInCall, c.lostSat = true, e.Call, 0, 0, false
}

func (c *eventChecker) end(t *testing.T, now time.Duration, e *voice.Event) {
	t.Helper()
	if !c.open || !sameParties(&e.Call, &c.call) {
		t.Fatalf("at %v: call end %+v, open %v call %+v", now, e.Call, c.open, c.call)
	}
	if e.Reason < voice.EndTerminator || e.Reason >= voice.EndTimeout {
		t.Fatalf("at %v: end reason %v", now, e.Reason)
	}
	st := &e.Stats
	bursts := int(st.Bursts)
	switch {
	case bursts < c.voiceInCall || bursts > c.voiceSinceEnd:
		t.Fatalf("at %v: Stats.Bursts %d, %d voice events in the call, %d since the last end", now, bursts, c.voiceInCall, c.voiceSinceEnd)
	case c.call.Origin != voice.OriginEmbedded && bursts != c.voiceInCall:
		t.Fatalf("at %v: Stats.Bursts %d of a %v call with %d voice events", now, bursts, c.call.Origin, c.voiceInCall)
	case st.Lost != 0xFFFF && int(st.Lost) < c.lostInCall,
		st.Lost != 0xFFFF && !c.lostSat && c.call.Origin != voice.OriginEmbedded && int(st.Lost) != c.lostInCall:
		t.Fatalf("at %v: Stats.Lost %d, the voice events of the call lost %d", now, st.Lost, c.lostInCall)
	case st.Duration < 0 || st.BitErrors > st.BitsChecked:
		t.Fatalf("at %v: Stats %+v", now, *st)
	}
	c.open, c.voiceSinceEnd = false, 0
	c.session, c.maySessionEnd = e.Call, true
}

type relayChecker struct {
	tx      *voice.Transmitter
	rx      *voice.Receiver
	check   eventChecker
	outs    []voice.Output
	events  []voice.Event
	lastDue time.Duration
	inTx    bool
}

func (r *relayChecker) run(t *testing.T, now time.Duration) {
	t.Helper()
	for {
		at, ok := r.tx.Deadline()
		if !ok || at > now {
			return
		}
		r.outs = r.tx.Tick(at, r.outs[:0])
		if len(r.outs) == 0 || len(r.outs) > voice.MaxOutputs {
			t.Fatalf("relay Tick at %v: %d outputs", at, len(r.outs))
		}
		for i := range r.outs {
			o := &r.outs[i]
			if o.Kind == voice.OutputEnded {
				r.inTx = false
				continue
			}
			if r.inTx && o.Due != r.lastDue+slot {
				t.Fatalf("relay burst due %v after %v", o.Due, r.lastDue)
			}
			r.inTx, r.lastDue = true, o.Due
			var b layer2.Burst
			if err := b.DecodeFromBytes(o.Burst); err != nil {
				t.Fatalf("relay burst %x does not decode: %v", o.Burst, err)
			}
			if o.Hint.VoiceBurst == enums.VoiceBurstA && b.VoiceBurst != enums.VoiceBurstA {
				t.Fatalf("relay burst A %x without a voice SYNC", o.Burst)
			}
			r.events = r.rx.Burst(o.Due, &o.Burst, layer2.Hint{}, r.events[:0])
			r.check.check(t, r.rx, o.Due, r.events)
		}
	}
}

func FuzzReceiver(f *testing.F) {
	p := newPool(f)
	for _, s := range seeds(p) {
		f.Add(s.flags, s.cc, s.lost, s.hang, s.seqBits, s.start, s.ops, s.raw)
	}
	f.Fuzz(func(t *testing.T, flags, cc, lost, hang, seqBits, start uint8, ops, raw []byte) {
		runScript(t, p, &rxInput{flags: flags, cc: cc, lost: lost, hang: hang, seqBits: seqBits, start: start, ops: ops, raw: raw})
	})
}

func runScript(t *testing.T, p *pool, in *rxInput) {
	t.Helper()
	s := parseScript(in, p)
	rx, err := voice.NewReceiver(&s.cfg)
	if s.badCfg {
		if err == nil {
			t.Fatalf("color code %d accepted", s.cc)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	tx, err := voice.NewTransmitter(&s.txCfg)
	if err != nil {
		t.Fatal(err)
	}
	relay := relayChecker{tx: tx, rx: newReceiver(t, nil),
		outs: make([]voice.Output, 0, voice.MaxOutputs), events: make([]voice.Event, 0, voice.MaxEvents)}
	var check eventChecker
	events := make([]voice.Event, 0, voice.MaxEvents)
	scratch := layer2.NewBurst()
	for i := range s.ops {
		op := &s.ops[i]
		events = runOp(rx, scratch, op, events[:0])
		switch op.kind {
		case opReset:
			check.reset()
			continue
		case opExpect:
			continue
		case opBurst, opDecoded, opLost, opTick:
		}
		check.check(t, rx, op.at, events)
		for j := range events {
			_ = relay.tx.Relay(op.at, &events[j])
		}
		relay.run(t, op.at)
	}
}

func runOp(rx *voice.Receiver, scratch *layer2.Burst, op *scriptOp, dst []voice.Event) []voice.Event {
	switch op.kind {
	case opBurst:
		return rx.Burst(op.at, &op.raw, op.hint, dst)
	case opDecoded:
		if scratch.DecodeFromBytes(op.raw) != nil {
			return rx.Tick(op.at, dst)
		}
		return rx.Decoded(op.at, scratch, op.hint, dst)
	case opLost:
		return rx.Lost(op.at, dst)
	case opTick:
		return rx.Tick(op.at, dst)
	case opExpect:
		rx.Expect(op.at, op.call, op.until)
	case opReset:
		rx.Reset()
	}
	return dst
}

type roundTrip struct {
	cfg     voice.TransmitterConfig
	call    voice.Call
	opts    voice.CallOptions
	gps     pdu.FLCGPSInfo
	seq     bool
	seqBits uint8
	frames  []vocoder.AMBEFrame
	drop    []byte
	errs    bool
}

type rtInput struct {
	flags, cc, headers, terms, so uint8
	target, source                uint32
	n, alias                      uint8
	ramp                          bool
	text, frames, drop            []byte
}

func parseRoundTrip(in *rtInput) roundTrip {
	var rt roundTrip
	flags, cc, headers, terms, so := in.flags, in.cc, in.headers, in.terms, in.so
	rt.call = voice.Call{
		Group:   flags&1 != 0,
		Target:  in.target & 0xFFFFFF,
		Source:  in.source & 0xFFFFFF,
		Options: layer3.DecodeServiceOptions([1]byte{so}),
	}
	rt.cfg = voice.TransmitterConfig{Slot: enums.Slot1, ColorCode: enums.ColorCode(cc & 15), Source: layer2.SyncSource(flags >> 5 & 3),
		Silence: vocoder.AMBESilenceSlot(), Headers: int(headers%voice.MaxHeaders) + 1, Terminators: int(terms%4) + 1,
		Queue: voice.MaxQueue}
	if flags&2 != 0 {
		rt.opts.PIHeader = &pdu.PIHeader{AlgorithmID: 1, KeyID: cc, MessageIndicator: uint32(so)}
	}
	if flags&4 != 0 {
		rt.gps = pdu.FLCGPSInfo{Latitude: float64(int8(cc)) / 2, Longitude: float64(int8(so))}
		rt.opts.GPS = &rt.gps
	}
	rt.seq, rt.seqBits, rt.errs = flags&8 != 0, 0, flags&0x10 != 0
	if flags&0x80 != 0 {
		rt.seqBits = 8
	}
	rt.opts.TalkerAliasFormat = layer3.TalkerAliasDataFormat(in.alias >> 6)
	text := make([]byte, in.alias&maxAlias)
	for i := range text {
		var c byte
		if i < len(in.text) {
			c = in.text[i]
		}
		text[i] = 0x20 + c%0x5F
	}
	rt.opts.TalkerAlias = string(text)
	pattern := in.frames[:min(len(in.frames), maxFrames)]
	rt.frames = make([]vocoder.AMBEFrame, in.n)
	for i := range rt.frames {
		f := &rt.frames[i]
		if in.ramp {
			*f = vocoder.AMBEFrame{byte(i), 0xA5, byte(3 * i), 0x5A, byte(i >> 1), 0x0F, 0x80}
		}
		for j := range f {
			if len(pattern) > 0 {
				f[j] ^= pattern[(7*i+j)%len(pattern)]
			}
		}
		f[6] &= 0x80
	}
	rt.drop = in.drop[:min(len(in.drop), maxDrop)]
	return rt
}

type sent struct {
	out    voice.Output
	frames [layer2.VoiceSlotsPerBurst]vocoder.AMBEFrame
	voice  int
}

func transmit(t *testing.T, tx *voice.Transmitter, rt *roundTrip) []sent {
	t.Helper()
	var all []sent
	buf := make([]voice.Output, 0, voice.MaxOutputs)
	next, nVoice := 0, 0
	for {
		for next < len(rt.frames) && tx.Room() > 0 {
			if err := tx.Frame(rt.frames[next]); err != nil {
				t.Fatal(err)
			}
			next++
		}
		if next == len(rt.frames) {
			tx.Stop(0)
		}
		at, ok := tx.Deadline()
		if !ok {
			return all
		}
		buf = tx.Tick(at, buf[:0])
		for _, o := range buf {
			if o.Kind != voice.OutputBurst {
				continue
			}
			s := sent{out: o, voice: -1}
			if o.Hint.VoiceBurst != enums.VoiceBurstUnknown {
				s.voice = nVoice
				for i := range s.frames {
					s.frames[i] = vocoder.AMBESilence()
					if k := nVoice*layer2.VoiceSlotsPerBurst + i; k < len(rt.frames) {
						s.frames[i] = rt.frames[k]
					}
				}
				nVoice++
			}
			all = append(all, s)
		}
	}
}

func FuzzTransmitterRoundTrip(f *testing.F) {
	for _, s := range roundTripSeeds() {
		f.Add(s.flags, s.cc, s.headers, s.terms, s.so, s.target, s.source, s.n, s.alias, s.ramp, s.text, s.frames, s.drop)
	}
	f.Fuzz(func(t *testing.T, flags, cc, headers, terms, so uint8, target, source uint32, n, alias uint8, ramp bool, text, frames, drop []byte) {
		runRoundTrip(t, &rtInput{flags: flags, cc: cc, headers: headers, terms: terms, so: so, target: target, source: source,
			n: n, alias: alias, ramp: ramp, text: text, frames: frames, drop: drop})
	})
}

func roundTripSeeds() []rtInput {
	call := rtInput{cc: 0x22, terms: 0x04, so: 0x01, target: 3100, source: 3191868}
	short := call
	short.n, short.alias, short.text = 18, 0x06, []byte("KI5VMF")
	full := short
	full.n, full.alias, full.ramp = 60, 0x46, true
	with := func(flags byte, drop ...byte) rtInput {
		s := full
		s.flags |= flags
		s.drop = drop
		return s
	}
	return []rtInput{
		call,
		short,
		full,
		with(0, 0x00, 0x10, 0x00, 0x02, 0x01, 0x80, 0x00, 0x00, 0x40),
		with(0x0F, 0x07, 0x00, 0x24, 0x00, 0x00, 0x18),
		with(0, 0x1F, 0xFF, 0xF0),
		with(0x12, 0x45, 0x00, 0x7F, 0x41, 0x52, 0x63, 0x74, 0x45, 0x40, 0x56, 0x67, 0x78, 0x49, 0x5A, 0x6B),
	}
}

func runRoundTrip(t *testing.T, in *rtInput) {
	rt := parseRoundTrip(in)
	tx, err := voice.NewTransmitter(&rt.cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Start(0, rt.call, &rt.opts); err != nil {
		if _, encErr := pdu.EncodeTalkerAlias(rt.opts.TalkerAlias, rt.opts.TalkerAliasFormat); encErr == nil {
			t.Fatalf("Start %+v: %v", rt.call, err)
		}
		return
	}
	checkRoundTrip(t, &rt, transmit(t, tx, &rt))
}

func (rt *roundTrip) dropped(i int) bool {
	if rt.errs {
		return i < len(rt.drop) && rt.drop[i]&0x80 != 0
	}
	return i/8 < len(rt.drop) && rt.drop[i/8]&(0x80>>(i%8)) != 0
}

func (rt *roundTrip) damage(i int, b *sent) layer2.RawBurst {
	raw := b.out.Burst
	if !rt.errs || i >= len(rt.drop) || rt.drop[i]&0x40 == 0 {
		return raw
	}
	k := int(rt.drop[i] & 0x3F)
	var bit int
	switch {
	case b.out.Hint.VoiceBurst == enums.VoiceBurstA:
		bit = 108 + k%48
	case b.voice >= 0 && k%16 < 8:
		bit = 108 + k%16
	case b.voice >= 0:
		bit = 148 + k%16 - 8
	case k*3 < 98:
		bit = k * 3
	default:
		bit = 166 + k*3 - 98
	}
	raw[bit/8] ^= 0x80 >> (bit % 8)
	return raw
}

type segment struct {
	events      []voice.Event
	headers     int
	terminators int
}

type delivery struct {
	segments []segment
	arrived  []arrival
	heard    [][9]byte
	relayed  [][9]byte
}

func deliver(t *testing.T, rt *roundTrip, bursts []sent) delivery {
	t.Helper()
	rx := newReceiver(t, nil)
	var check eventChecker
	events := make([]voice.Event, 0, voice.MaxEvents)
	d := delivery{segments: make([]segment, 1)}
	loose, first, started := false, true, false
	header, anchored := false, false
	var last time.Duration
	relay := newTransmitter(t, &voice.TransmitterConfig{Slot: enums.Slot1, Silence: vocoder.AMBESilenceSlot(), Queue: voice.MaxQueue})
	outs := make([]voice.Output, 0, voice.MaxOutputs)
	tick := func(until time.Duration) {
		for at, ok := relay.Deadline(); ok && at <= until; at, ok = relay.Deadline() {
			outs = relay.Tick(at, outs[:0])
			for _, o := range outs {
				if o.Kind == voice.OutputBurst && o.Hint.VoiceBurst != enums.VoiceBurstUnknown && !o.Fill {
					slots := o.Burst.VoiceSlots()
					d.relayed = append(d.relayed, slots[:]...)
				}
			}
		}
	}
	run := func(at time.Duration, raw *layer2.RawBurst, h layer2.Hint) {
		if raw == nil {
			events = rx.Tick(at, events[:0])
		} else {
			events = rx.Burst(at, raw, h, events[:0])
		}
		check.check(t, rx, at, events)
		seg := &d.segments[len(d.segments)-1]
		seg.events = append(seg.events, events...)
		for i := range events {
			if e := &events[i]; e.Kind == voice.EventVoice && e.Call.Origin != voice.OriginUnknown {
				slots := e.Burst.VoiceSlots()
				d.heard = append(d.heard, slots[:]...)
			}
			if err := relay.Relay(at, &events[i]); err != nil {
				t.Fatalf("Relay %v: %v", events[i].Kind, err)
			}
		}
		tick(at)
	}
	for i := range bursts {
		b := &bursts[i]
		if rt.dropped(i) {
			loose = loose || rt.seq && b.voice < 0
			anchored = anchored && b.voice >= 0
			continue
		}
		if started && b.out.Due-last >= voice.DefaultLost {
			run(last+voice.DefaultLost, nil, layer2.Hint{})
			d.segments = append(d.segments, segment{})
			first, header, anchored = true, false, false
		}
		seg := &d.segments[len(d.segments)-1]
		h := layer2.Hint{}
		if rt.seq {
			h.HasSeq, h.Seq, h.SeqBits = true, uint16(i), rt.seqBits
		}
		switch {
		case b.voice >= 0:
			if p := b.out.Hint.VoiceBurst; p != enums.VoiceBurstC && p != enums.VoiceBurstD {
				loose = false
			}
			d.arrived = append(d.arrived, arrival{sent: b, raw: rt.damage(i, b), loose: loose, first: first,
				afterHeader: first && anchored})
			first = false
		case b.out.Hint.DataType == enums.DataTypeVoiceLCHeader:
			seg.headers++
			header, anchored = true, true
		case b.out.Hint.DataType == enums.DataTypePIHeader:
			anchored = header
		case b.out.Hint.DataType == enums.DataTypeTerminatorWithLC:
			seg.terminators++
		}
		last, started = b.out.Due, true
		raw := rt.damage(i, b)
		run(last, &raw, h)
	}
	run(last+10*time.Second, nil, layer2.Hint{})
	tick(last + time.Hour)
	return d
}

func checkRoundTrip(t *testing.T, rt *roundTrip, bursts []sent) {
	t.Helper()
	d := deliver(t, rt, bursts)
	k := 0
	for i := range d.segments {
		seg := &d.segments[i]
		var starts, ends []voice.Event
		for _, e := range seg.events {
			switch e.Kind {
			case voice.EventCallStart:
				starts = append(starts, e)
			case voice.EventCallEnd:
				ends = append(ends, e)
			case voice.EventVoice:
				if k >= len(d.arrived) {
					t.Fatalf("voice event %d, only %d voice bursts arrived", k, len(d.arrived))
				}
				checkVoice(t, k, &e, d.arrived)
				k++
			case voice.EventTalkerAlias:
				if string(e.Text) != rt.opts.TalkerAlias {
					t.Fatalf("talker alias %q, sent %q", e.Text, rt.opts.TalkerAlias)
				}
			case voice.EventEmbedded, voice.EventPIHeader, voice.EventSessionStart, voice.EventSessionEnd:
			}
		}
		checkSegment(t, rt, seg, i == len(d.segments)-1, starts, ends)
	}
	if k != len(d.arrived) {
		t.Fatalf("%d voice events, %d voice bursts arrived", k, len(d.arrived))
	}
	if !rt.lossy() && !slices.Equal(d.heard, d.relayed) {
		t.Fatalf("relayed %d slots %x, heard %d %x", len(d.relayed), d.relayed, len(d.heard), d.heard)
	}
}

func (rt *roundTrip) lossy() bool {
	if rt.errs {
		return slices.ContainsFunc(rt.drop, func(b byte) bool { return b&0x80 != 0 })
	}
	return slices.ContainsFunc(rt.drop, func(b byte) bool { return b != 0 })
}

func checkSegment(t *testing.T, rt *roundTrip, seg *segment, last bool, starts, ends []voice.Event) {
	t.Helper()
	if seg.headers > 0 && (len(starts) != 1 || starts[0].Call.Origin != voice.OriginHeader) {
		t.Fatalf("%d headers arrived, call starts %+v", seg.headers, starts)
	}
	if len(starts) > 1 || len(ends) != len(starts) {
		t.Fatalf("call starts %+v, ends %+v", starts, ends)
	}
	if len(starts) == 0 {
		return
	}
	c := starts[0].Call
	if !sameParties(&c, &rt.call) || c.Options != rt.call.Options {
		t.Fatalf("call %+v, sent %+v", c, rt.call)
	}
	want := voice.EndLost
	if last && seg.terminators > 0 {
		want = voice.EndTerminator
	}
	if ends[0].Reason != want {
		t.Fatalf("end reason %v, want %v (%d terminators arrived)", ends[0].Reason, want, seg.terminators)
	}
	if c.Origin == voice.OriginHeader && int(ends[0].Stats.Duplicates) != seg.headers-1 {
		t.Fatalf("Stats.Duplicates %d, %d headers arrived", ends[0].Stats.Duplicates, seg.headers)
	}
}

type arrival struct {
	*sent
	raw         layer2.RawBurst
	loose       bool
	first       bool
	afterHeader bool
}

func checkVoice(t *testing.T, k int, e *voice.Event, arrived []arrival) {
	t.Helper()
	b := arrived[k]
	if e.Burst != b.raw {
		t.Fatalf("voice event %d: burst %x, received %x", k, e.Burst, b.raw)
	}
	slots := e.Burst.VoiceSlots()
	for i := range slots {
		f, res := vocoder.DecodeAMBE72(slots[i])
		if f != b.frames[i] || res.ErrorsCorrected != 0 || res.Uncorrectable || e.Frames[i].ErrorsCorrected != 0 || e.Frames[i].Bad {
			t.Fatalf("voice event %d slot %d: frame %x %+v, sent %x", k, i, f, e.Frames[i], b.frames[i])
		}
	}
	if b.loose {
		return
	}
	switch b.out.Hint.VoiceBurst {
	case enums.VoiceBurstC, enums.VoiceBurstD:
		if e.Position != b.out.Hint.VoiceBurst && e.Position != enums.VoiceBurstUnknown {
			t.Fatalf("voice event %d: position %v, sent %v", k, e.Position, b.out.Hint.VoiceBurst)
		}
	default:
		if e.Position != b.out.Hint.VoiceBurst {
			t.Fatalf("voice event %d: position %v, sent %v", k, e.Position, b.out.Hint.VoiceBurst)
		}
	}
	switch {
	case b.afterHeader:
		if int(e.Lost) != b.voice {
			t.Fatalf("voice event %d: Lost %d, %d voice bursts lost after the header", k, e.Lost, b.voice)
		}
	case k > 0 && !b.first:
		if lost := b.voice - arrived[k-1].voice - 1; int(e.Lost) != lost {
			t.Fatalf("voice event %d: Lost %d, %d voice bursts lost", k, e.Lost, lost)
		}
	}
}

func rxCorpus(t *testing.T) []rxInput {
	t.Helper()
	var out []rxInput
	var in rxInput
	fuzztest.Corpus(t, "FuzzReceiver", func() { out = append(out, in) },
		&in.flags, &in.cc, &in.lost, &in.hang, &in.seqBits, &in.start, &in.ops, &in.raw)
	return out
}
