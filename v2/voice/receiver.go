package voice

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// MaxEvents is the most events that one call of a Receiver method appends.
const MaxEvents = 6

// DefaultLost is the time without a burst after which a Receiver ends a
// call, when ReceiverConfig.Lost is 0. The spec gives no value; MMDVM
// firmware waits about 780 ms, MMDVMHost 1.5 s on the network side.
const DefaultLost = time.Second

// ReceiverConfig configures NewReceiver. A nil config means the zero
// ReceiverConfig.
type ReceiverConfig struct {
	// ColorCode, when set, drops bursts with another color code. Burst A
	// carries no color code, so it does not start a late entry. Nil
	// accepts every color code.
	ColorCode *enums.ColorCode
	// Lost is the time without a burst after which a call ends with
	// EndLost. 0 means DefaultLost, and a negative value turns the timer
	// off.
	Lost time.Duration
	// Hang is the call hang time, in which a call to the same target
	// continues the session. 0 means constants.TCallHang (3 s), and a
	// negative value turns session tracking off. A broadcast call and a
	// call to constants.AllMSID have no hang time (ETSI TS 102 361-2
	// clause 5.3.2.3.2).
	Hang time.Duration
	// Strict selects the strict embedded LC decode of
	// layer2.DecodeEmbeddedLC.
	Strict bool
	// Scratch, when set, is the Burst that Burst decodes into. Receivers
	// that run on one goroutine can share one, because a Receiver is done
	// with it when its method returns. Make it with layer2.NewBurst, so
	// that no decode allocates. Nil means the Receiver makes its own.
	Scratch *layer2.Burst
}

// Receiver turns the bursts of one logical channel into call events, as
// the package doc describes. Make it with NewReceiver. A Receiver is not
// safe for concurrent use and must not be copied; go vet reports a copy.
type Receiver struct {
	lostAfter time.Duration
	hang      time.Duration
	strict    bool
	b         *layer2.Burst
	opts      layer2.DecodeOptions
	cc        enums.ColorCode
	dataType  enums.DataType

	state       State
	call        Call
	stats       Stats
	start       time.Duration
	last        time.Duration
	hangUntil   time.Duration
	session     Call
	inSession   bool
	stream      uint32
	hasStream   bool
	expect      Call
	expectUntil time.Duration
	hasExpect   bool

	track    tracker
	raw      layer2.RawBurst
	frags    [4][4]byte
	fragNext int
	lc       pdu.FullLinkControl
	lcp      pdu.FullLinkControlPayloads
	alias    pdu.TalkerAliasAssembler
	text     [talkerAliasText]byte
	noCopy   noCopy
}

// talkerAliasText is the size of the talker alias buffer: 31 UTF-16 code
// units of at most 3 UTF-8 octets each.
const talkerAliasText = 96

// NewReceiver returns a Receiver for cfg. A nil cfg means the zero
// ReceiverConfig. It returns a *ConfigError when the color code is above 15.
// It allocates the Receiver, and its Burst unless cfg.Scratch is set.
func NewReceiver(cfg *ReceiverConfig) (*Receiver, error) {
	if cfg == nil {
		cfg = &ReceiverConfig{}
	}
	r := &Receiver{lostAfter: cfg.Lost, hang: cfg.Hang, strict: cfg.Strict, b: cfg.Scratch}
	if cfg.ColorCode != nil {
		if *cfg.ColorCode > 15 {
			return nil, errColorCode
		}
		r.cc = *cfg.ColorCode
		r.opts.ColorCode = &r.cc
	}
	if r.lostAfter == 0 {
		r.lostAfter = DefaultLost
	}
	if r.hang == 0 {
		r.hang = constants.TCallHang
	}
	if r.b == nil {
		r.b = layer2.NewBurst()
	}
	return r, nil
}

// Reset ends any call and session without events and forgets the expected
// call. It keeps the config.
func (r *Receiver) Reset() {
	r.state = StateIdle
	r.call, r.stats, r.session = Call{}, Stats{}, Call{}
	r.inSession, r.hasStream, r.hasExpect = false, false, false
	r.track = tracker{}
	r.fragNext = 0
	r.alias.Reset()
}

// State returns the state of the Receiver.
func (r *Receiver) State() State {
	return r.state
}

// Expect gives the call of voice that arrives without an LC before until,
// such as after a Tier III grant, or a network stream that starts without
// a header. The first voice burst then starts the call with
// OriginExpected. A header or embedded LC with the same addresses that
// comes later sets Call.LC without an event, and one with other addresses
// ends the call and starts a new one.
func (r *Receiver) Expect(now time.Duration, c Call, until time.Duration) {
	if until <= now {
		r.hasExpect = false
		return
	}
	c.Origin, c.LC = OriginExpected, pdu.LCBytes{}
	r.expect, r.expectUntil, r.hasExpect = c, until, true
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Receiver as Decoded does, and appends the events to out. A
// burst that does not decode, or has another color code than
// ReceiverConfig.ColorCode, is dropped, but its Hint still applies, so an
// IPSC "sync not detected" burst with Hint.SyncLost still ends the call.
func (r *Receiver) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, out []Event) []Event {
	r.opts.Kind, r.opts.VoiceBurst, r.opts.DataType = h.Kind, enums.VoiceBurstUnknown, nil
	if h.Kind == layer2.KindVoice {
		r.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		r.dataType = h.DataType
		r.opts.DataType = &r.dataType
	}
	if r.b.DecodeFromBytesWith(*raw, &r.opts) != nil {
		return r.receive(now, nil, nil, &h, out)
	}
	return r.receive(now, r.b, raw, &h, out)
}

// Decoded runs the decoded burst b through the Receiver and appends the
// events to out. ReceiverConfig.ColorCode does not apply: b was decoded
// with the options of the caller. For a transport event without a burst,
// such as an MMDVM TAG_LOST, call Lost.
func (r *Receiver) Decoded(now time.Duration, b *layer2.Burst, h layer2.Hint, out []Event) []Event {
	return r.receive(now, b, nil, &h, out)
}

// Lost tells the Receiver that the transport lost the signal, such as an
// MMDVM TAG_LOST. A call ends with EndSyncLost.
func (r *Receiver) Lost(now time.Duration, out []Event) []Event {
	out = r.timers(now, out)
	return r.syncLost(now, out)
}

// Tick runs the timers that are due at now and appends their events: the
// end of a call after ReceiverConfig.Lost without a burst, and the end of
// the hang time. A Tick before Deadline does nothing.
func (r *Receiver) Tick(now time.Duration, out []Event) []Event {
	return r.timers(now, out)
}

// Deadline returns the time at which the Receiver needs a Tick. ok is
// false when no timer runs.
func (r *Receiver) Deadline() (at time.Duration, ok bool) {
	if (r.state == StateInCall || r.state == StateLateEntry) && r.lostAfter >= 0 {
		at, ok = r.last+r.lostAfter, true
	}
	if (r.state == StateHang || r.state == StateLateEntry) && r.inSession && (!ok || r.hangUntil < at) {
		at, ok = r.hangUntil, true
	}
	return at, ok
}

// timers runs every timer that is due at now, in deadline order.
func (r *Receiver) timers(now time.Duration, out []Event) []Event {
	for {
		at, ok := r.Deadline()
		if !ok || now < at {
			return out
		}
		switch r.state {
		case StateInCall:
			out = r.endCall(at, EndLost, out)
		case StateLateEntry:
			if r.inSession && at == r.hangUntil {
				r.inSession = false
				out, _ = appendEvent(out, EventSessionEnd, &r.session)
				continue
			}
			out = r.abandon(at, out)
		case StateHang:
			out = r.endSession(out)
		case StateIdle:
			return out
		}
	}
}

// syncLost ends the call or the late entry for a transport that lost the
// signal.
func (r *Receiver) syncLost(now time.Duration, out []Event) []Event {
	switch r.state {
	case StateInCall:
		return r.endCall(now, EndSyncLost, out)
	case StateLateEntry:
		return r.abandon(now, out)
	case StateIdle, StateHang:
	}
	return out
}

// receive is the common part of Burst and Decoded. b is nil for a burst
// that did not decode, and raw is nil for a burst from Decoded.
func (r *Receiver) receive(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, out []Event) []Event {
	out = r.timers(now, out)
	if h.SyncLost {
		out = r.syncLost(now, out)
	}
	if h.HasStream {
		if r.hasStream && h.Stream != r.stream {
			switch r.state {
			case StateInCall:
				out = r.endCall(now, EndSuperseded, out)
			case StateLateEntry:
				out = r.abandon(now, out)
			case StateIdle, StateHang:
			}
		}
		r.stream, r.hasStream = h.Stream, true
	}

	if b != nil {
		switch b.Kind {
		case layer2.KindVoice:
			out = r.voice(now, b, raw, h, out)
		case layer2.KindData:
			out = r.data(now, b, raw, h, out)
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if h.End == layer2.EndStrong {
		switch r.state {
		case StateInCall:
			out = r.endCall(now, EndTransport, out)
		case StateLateEntry:
			out = r.abandon(now, out)
		case StateIdle, StateHang:
		}
	}
	return out
}

// data handles a data burst.
func (r *Receiver) data(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, out []Event) []Event {
	if b.SyncPattern == enums.SyncNone || b.SlotType.FEC.Uncorrectable {
		return out
	}
	switch b.SlotType.DataType {
	case enums.DataTypeVoiceLCHeader:
		lc, ok := lcOctets(b)
		if !ok {
			return out
		}
		c, ok := callOf(lc, OriginHeader)
		if !ok {
			return out
		}
		out = r.callLC(now, &c, out)
		if h.HasSeq && r.state == StateInCall {
			r.track.afterHeader(h)
		}
		return out
	case enums.DataTypeTerminatorWithLC:
		return r.terminator(now, b, out)
	case enums.DataTypePIHeader:
		if r.state == StateInCall || r.state == StateLateEntry {
			r.last = now
			var e *Event
			out, e = appendEvent(out, EventPIHeader, &r.call)
			e.Burst = rawOf(b, raw)
			if h.HasSeq {
				r.track.afterHeader(h)
			}
		}
		return out
	case enums.DataTypeIdle:
		switch r.state {
		case StateInCall:
			return r.endNoHang(now, EndIdle, out)
		case StateLateEntry:
			return r.abandon(now, out)
		case StateHang:
			return r.endSession(out)
		case StateIdle:
		}
		return out
	case enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeDataHeader,
		enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeUnifiedSingleBlock,
		enums.DataTypeReserved:
	}
	switch r.state {
	case StateInCall:
		return r.endNoHang(now, EndDataSync, out)
	case StateLateEntry:
		return r.abandon(now, out)
	case StateIdle, StateHang:
	}
	return out
}

// rawOf returns *raw, or b encoded when raw is nil.
func rawOf(b *layer2.Burst, raw *layer2.RawBurst) layer2.RawBurst {
	if raw != nil {
		return *raw
	}
	r, _ := b.Encode()
	return r
}

// lcOctets returns the LC octets of a Voice LC Header or Terminator with
// LC burst whose RS(12,9) code is correctable.
func lcOctets(b *layer2.Burst) (pdu.LCBytes, bool) {
	fl, ok := b.Data.(*pdu.FullLinkControl)
	if !ok || fl.FEC.Uncorrectable {
		return pdu.LCBytes{}, false
	}
	octets, err := fl.Bytes()
	if err != nil {
		return pdu.LCBytes{}, false
	}
	return pdu.LCBytes(octets), true
}

// terminator handles a Terminator with LC.
func (r *Receiver) terminator(now time.Duration, b *layer2.Burst, out []Event) []Event {
	lc, lcOK := lcOctets(b)
	c, ok := callOf(lc, OriginTerminator)
	switch r.state {
	case StateInCall:
		return r.endCall(now, EndTerminator, out)
	case StateLateEntry:
		out = r.abandon(now, out)
	case StateIdle, StateHang:
	}
	if !lcOK || !ok {
		return out
	}
	if r.state == StateHang {
		if c.Target == r.session.Target && c.Group == r.session.Group {
			return out
		}
		out = r.endSession(out)
	}
	if !r.hangs(&c) {
		return out
	}
	c.Stream = r.stream
	if !r.hasStream {
		c.Stream = 0
	}
	r.session, r.inSession = c, true
	r.state, r.hangUntil = StateHang, now+r.hang
	out, _ = appendEvent(out, EventSessionStart, &c)
	return out
}

// callLC handles the call LC c from a Voice LC Header or the embedded LC.
func (r *Receiver) callLC(now time.Duration, c *Call, out []Event) []Event {
	switch r.state {
	case StateInCall:
		if c.sameParties(&r.call) {
			if r.call.Origin == OriginExpected {
				r.call.LC, r.call.Options, r.call.FID = c.LC, c.Options, c.FID
			} else if c.Origin == OriginHeader {
				r.stats.Duplicates++
			}
			r.last = now
			return out
		}
		out = r.endCall(now, EndSuperseded, out)
	case StateLateEntry, StateIdle, StateHang:
	}
	return r.startCall(now, c, out)
}

// startCall starts the call c, after the end of a session to another
// target.
func (r *Receiver) startCall(now time.Duration, c *Call, out []Event) []Event {
	keep := r.state == StateLateEntry && c.Origin == OriginEmbedded
	if r.inSession && (c.Target != r.session.Target || c.Group != r.session.Group) {
		out = r.endSession(out)
	}
	if c.Origin == OriginHeader {
		r.track = tracker{}
		r.fragNext = 0
	}
	if !keep {
		r.alias.Reset()
		r.stats = Stats{}
		r.start = now
	}
	r.inSession = false
	if r.hasStream {
		c.Stream = r.stream
	}
	r.call = *c
	r.state, r.last = StateInCall, now
	r.hasExpect = false
	out, _ = appendEvent(out, EventCallStart, &r.call)
	return out
}

// hangs reports whether the call c has a call hang time.
func (r *Receiver) hangs(c *Call) bool {
	return r.hang >= 0 && !c.Options.IsBroadcast && (!c.Group || c.Target != constants.AllMSID)
}

// endCall ends the call at time at with reason, and starts the hang time
// when the call has one.
func (r *Receiver) endCall(at time.Duration, reason EndReason, out []Event) []Event {
	out = r.appendEnd(at, reason, out)
	if !r.hangs(&r.call) {
		r.state, r.inSession = StateIdle, false
		return out
	}
	r.session, r.inSession = r.call, true
	r.state, r.hangUntil = StateHang, at+r.hang
	return out
}

// endNoHang ends the call at time at with reason and goes to Idle.
func (r *Receiver) endNoHang(at time.Duration, reason EndReason, out []Event) []Event {
	out = r.appendEnd(at, reason, out)
	r.state, r.inSession = StateIdle, false
	return out
}

// appendEnd appends the EventCallEnd of the call.
func (r *Receiver) appendEnd(at time.Duration, reason EndReason, out []Event) []Event {
	r.stats.Duration = max(at-r.start, 0)
	var e *Event
	out, e = appendEvent(out, EventCallEnd, &r.call)
	e.Reason, e.Stats = reason, r.stats
	return out
}

// endSession ends the hang time.
func (r *Receiver) endSession(out []Event) []Event {
	r.state, r.inSession = StateIdle, false
	out, _ = appendEvent(out, EventSessionEnd, &r.session)
	return out
}

// abandon ends a late entry whose LC never came: back to the hang time
// when a session is open and its hang time runs, else to Idle.
func (r *Receiver) abandon(now time.Duration, out []Event) []Event {
	switch {
	case r.inSession && now < r.hangUntil:
		r.state = StateHang
	case r.inSession:
		out = r.endSession(out)
	default:
		r.state = StateIdle
	}
	return out
}

// voice handles a voice burst.
func (r *Receiver) voice(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, out []Event) []Event {
	switch r.state {
	case StateIdle, StateHang:
		if raw != nil && r.opts.ColorCode != nil && b.VoiceBurst == enums.VoiceBurstA && (!r.hasExpect || now >= r.expectUntil) {
			return out
		}
		r.track = tracker{}
		r.fragNext = 0
		r.alias.Reset()
		r.stats = Stats{}
		r.start = now
		if r.hasExpect && now < r.expectUntil {
			c := r.expect
			out = r.startCall(now, &c, out)
		} else {
			r.hasExpect = r.hasExpect && now < r.expectUntil
			r.call = Call{}
			if r.hasStream {
				r.call.Stream = r.stream
			}
			r.state = StateLateEntry
		}
	case StateInCall, StateLateEntry:
	}

	r.raw = rawOf(b, raw)
	idx, lost, p := r.track.place(b, &r.raw, h, now)
	switch p {
	case duplicate:
		r.stats.Duplicates++
		r.last = now
		return out
	case late:
		r.stats.Late++
		r.last = now
		return out
	case placed:
	}
	if h.End == layer2.EndWeak {
		r.track.forget()
	}
	r.last = now

	lc, done := r.collect(b, idx, lost)
	callChanged := false
	if done {
		c, ok := callOf(lc, OriginEmbedded)
		if ok {
			out = r.callLC(now, &c, out)
			callChanged = true
		}
	}

	r.stats.Bursts++
	r.stats.Lost = addSat16(r.stats.Lost, lost)
	var e *Event
	out, e = appendEvent(out, EventVoice, &r.call)
	e.Burst, e.Lost = r.raw, sat8(lost)
	e.Position = enums.VoiceBurstUnknown
	if v, ok := enums.VoiceBurstAt(idx); ok {
		e.Position = v
	}
	for i := range b.VoiceSlots {
		_, res := vocoder.DecodeAMBE72(b.VoiceSlots[i])
		e.Frames[i] = FrameQuality{ErrorsCorrected: sat8(res.ErrorsCorrected), Bad: res.Uncorrectable}
		r.stats.BitsChecked += uint32(sat8(res.BitsChecked))
		r.stats.BitErrors += uint32(sat8(res.ErrorsCorrected))
	}

	if done && !callChanged {
		out = r.embedded(lc, out)
	}
	return out
}

// collect adds the embedded signalling of burst b at superframe index idx
// to the fragments of the embedded LC, and returns the LC octets when burst
// E completes a superframe whose bursts B to E all arrived.
func (r *Receiver) collect(b *layer2.Burst, idx, lost int) (pdu.LCBytes, bool) {
	if idx < 1 || idx > 4 || !b.HasEmbeddedSignalling() {
		r.fragNext = 0
		return pdu.LCBytes{}, false
	}
	switch {
	case idx == 1:
		r.fragNext = 1
	case idx != r.fragNext+1 || lost > 0:
		r.fragNext = 0
		return pdu.LCBytes{}, false
	}
	var f [4]byte
	for i, v := range b.EmbeddedSignallingData {
		f[i/8] |= byte(v&1) << (7 - i%8)
	}
	r.frags[idx-1] = f
	r.fragNext = idx
	if idx < 4 {
		return pdu.LCBytes{}, false
	}
	r.fragNext = 0
	lc, res := layer2.DecodeEmbeddedLC(r.frags, r.strict)
	if res.Uncorrectable {
		return pdu.LCBytes{}, false
	}
	return pdu.LCBytes(lc), true
}

// embedded appends the EventEmbedded of an embedded LC that is not a call
// LC, and the EventTalkerAlias when it completes a talker alias.
func (r *Receiver) embedded(lc pdu.LCBytes, out []Event) []Event {
	if pdu.DecodeFullLinkControlBytesInto(&r.lc, &r.lcp, lc) != nil {
		return out
	}
	var e *Event
	out, e = appendEvent(out, EventEmbedded, &r.call)
	e.LC = r.lc
	alias, ok, err := r.alias.AppendAdd(r.text[:0], &r.lc)
	if ok && err == nil {
		out, e = appendEvent(out, EventTalkerAlias, &r.call)
		e.Text = alias
	}
	return out
}

// appendEvent appends an Event of kind k for the call c to out and returns
// the extended slice and the new Event. It grows out only when out lacks
// the capacity. It is not inlined, so that the growth stays in a function
// whose name tells that it appends.
//
//go:noinline
func appendEvent(out []Event, k EventKind, c *Call) ([]Event, *Event) {
	if len(out) < cap(out) {
		out = out[:len(out)+1]
		out[len(out)-1] = Event{}
	} else {
		out = append(out, Event{})
	}
	e := &out[len(out)-1]
	e.Kind, e.Call = k, *c
	return out, e
}

// addSat16 returns a+n, at most the largest uint16.
func addSat16(a uint16, n int) uint16 {
	sum := int(a) + n
	if sum >= 0xFFFF {
		return 0xFFFF
	}
	if sum <= 0 {
		return 0
	}
	return uint16(sum)
}

// sat8 returns n limited to 0 to 255.
func sat8(n int) uint8 {
	if n >= 0xFF {
		return 0xFF
	}
	if n <= 0 {
		return 0
	}
	return uint8(n)
}
