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
	text     [pdu.TalkerAliasMaxText]byte
	noCopy   noCopy
}

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
// through the Receiver as Decoded does, and appends the events to dst. A
// burst that does not decode, or has another color code than
// ReceiverConfig.ColorCode, is dropped, but its Hint still applies, so an
// IPSC "sync not detected" burst with Hint.SyncLost still ends the call. It
// allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Event) []Event {
	r.opts.Kind, r.opts.VoiceBurst, r.opts.DataType = h.Kind, enums.VoiceBurstUnknown, nil
	if h.Kind == layer2.KindVoice {
		r.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		r.dataType = h.DataType
		r.opts.DataType = &r.dataType
	}
	if r.b.DecodeFromBytesWith(*raw, &r.opts) != nil {
		return r.receive(now, nil, nil, &h, dst)
	}
	return r.receive(now, r.b, raw, &h, dst)
}

// Decoded runs the decoded burst b through the Receiver and appends the
// events to dst. ReceiverConfig.ColorCode does not apply: b was decoded
// with the options of the caller, so set DecodeOptions.ColorCode for the
// check. A nil b is a burst that did not decode, and only h applies. A
// monitor that feeds a burst to several machines decodes it once with
// layer2.Burst.DecodeFromBytesWith and calls Decoded on each. For a
// transport event without a burst, such as an MMDVM TAG_LOST, call Lost.
// It allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Decoded(now time.Duration, b *layer2.Burst, h layer2.Hint, dst []Event) []Event {
	return r.receive(now, b, nil, &h, dst)
}

// Lost tells the Receiver that the transport lost the signal, such as an
// MMDVM TAG_LOST. A call ends with EndSyncLost. It appends the events to
// dst, and allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Lost(now time.Duration, dst []Event) []Event {
	dst = r.timers(now, dst)
	return r.syncLost(now, dst)
}

// Tick runs the timers that are due at now and appends their events to dst:
// the end of a call after ReceiverConfig.Lost without a burst, and the end
// of the hang time. A Tick before Deadline does nothing. It allocates only
// when dst has no room for MaxEvents events.
func (r *Receiver) Tick(now time.Duration, dst []Event) []Event {
	return r.timers(now, dst)
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
		if r.state == StateInCall {
			r.track.afterHeader(h, now)
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
			r.track.afterHeader(h, now)
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

func rawOf(b *layer2.Burst, raw *layer2.RawBurst) layer2.RawBurst {
	if raw != nil {
		return *raw
	}
	r, _ := b.Encode()
	return r
}

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

func (r *Receiver) hangs(c *Call) bool {
	return r.hang >= 0 && !c.Options.IsBroadcast && (!c.Group || c.Target != constants.AllMSID)
}

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

func (r *Receiver) endNoHang(at time.Duration, reason EndReason, out []Event) []Event {
	out = r.appendEnd(at, reason, out)
	r.state, r.inSession = StateIdle, false
	return out
}

func (r *Receiver) appendEnd(at time.Duration, reason EndReason, out []Event) []Event {
	r.stats.Duration = max(at-r.start, 0)
	var e *Event
	out, e = appendEvent(out, EventCallEnd, &r.call)
	e.Reason, e.Stats = reason, r.stats
	return out
}

func (r *Receiver) endSession(out []Event) []Event {
	r.state, r.inSession = StateIdle, false
	out, _ = appendEvent(out, EventSessionEnd, &r.session)
	return out
}

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
	r.frags[idx-1] = b.EmbeddedSignallingData
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

func (r *Receiver) embedded(lc pdu.LCBytes, out []Event) []Event {
	if pdu.DecodeFullLinkControlBytesInto(&r.lc, &r.lcp, lc) != nil {
		return out
	}
	var e *Event
	out, e = appendEvent(out, EventEmbedded, &r.call)
	e.LC = r.lc
	n, ok, err := r.alias.AddInto(&r.text, &r.lc)
	if ok && err == nil {
		out, e = appendEvent(out, EventTalkerAlias, &r.call)
		e.Text = r.text[:n]
	}
	return out
}

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

func sat8(n int) uint8 {
	if n >= 0xFF {
		return 0xFF
	}
	if n <= 0 {
		return 0
	}
	return uint8(n)
}
