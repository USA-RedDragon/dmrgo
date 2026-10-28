package voice

import (
	"encoding/binary"
	"errors"
	"math/bits"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/ambe"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
)

// MaxEvents is the most events that one call of a Receiver method appends.
const MaxEvents = 16

// maxMisses is the number of voice bursts in a row that the sync check of
// the package doc drops before the Receiver ends the call. ETSI TS 102 361-1
// Annex G.1.1 gives no count; the MMDVM firmware reports the loss after 13
// bursts without a SYNC (MAX_SYNC_LOST_FRAMES, DMRSlotRX.cpp lines 36 and
// 199-206, g4klx/MMDVM 046e1df1).
const maxMisses = 13

const codec2FID enums.FID = 0xC2

const maxPend = 6

const sameCallEMB = 3

const ownKeep = 64

const embFar = -60

const notA = -80

// The sync check works in log-likelihood ratios of voice against noise, in
// 1/16 nat, for a bit error rate of 10%: per frame by the errors that the
// Golay codes of AMBE+2 sections A (index 0 for uncorrectable) and B correct,
// and for an EMB of an unknown color code by the errors that its decode
// corrects.
const (
	oddsCap  = 40
	oddsIdle = -160
	oddsLow  = -128
	oddsHi   = 64
	oddsLo   = -200
)

func ambeAScore(errs int) int {
	switch errs {
	case 0:
		return 93
	case 1:
		return 57
	case 2:
		return 22
	case 3:
		return -13
	}
	return -11
}

func ambeBScore(errs int) int {
	return [4]int{83, 48, 13, -22}[min(max(errs, 0), 3)]
}

func embScore(errs int) int {
	return [3]int{117, 82, 47}[min(max(errs, 0), 2)]
}

// DefaultLost is the time without a burst after which a Receiver ends a
// call, when ReceiverConfig.Lost is 0. The spec gives no value; MMDVM
// firmware waits about 780 ms, MMDVMHost 1.5 s on the network side.
const DefaultLost = time.Second

// ReceiverConfig configures NewReceiver. A nil config means the zero
// ReceiverConfig.
type ReceiverConfig struct {
	// ColorCode, when set, drops bursts with another color code, except a
	// voice burst of a call that the transport does not place whose EMB is
	// within 5 bits of a codeword of ColorCode, which goes to the sync check
	// of the package doc. Burst A carries no color code, so it does not
	// start a late entry. Nil accepts every color code.
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
	// Scratch, when set, is the Burst that Burst decodes into. Receivers
	// that run on one goroutine can share one, because a Receiver is done
	// with it when its method returns. Make it with layer2.NewBurst, so
	// that no decode allocates. Nil means the Receiver makes its own.
	Scratch *layer2.Burst
}

// Receiver turns the bursts of one logical channel into call events, as
// the package doc describes. The zero Receiver does nothing. Make it with
// NewReceiver. A Receiver is not safe for concurrent use and must not be
// copied; go vet reports a copy.
type Receiver struct {
	lostAfter time.Duration
	hang      time.Duration
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
	alias    pdu.TalkerAliasAssembler
	text     [pdu.TalkerAliasMaxText]byte
	held     pdu.FullLinkControl
	heldText [pdu.TalkerAliasMaxText]byte
	heldN    int
	hasHeld  bool
	heldTA   bool
	callCC   enums.ColorCode
	hasCC    bool
	misses   int
	odds     int
	fragLC   pdu.LCBytes
	frags4   [4][4]byte
	heard    [4][4]byte
	heardSet uint8
	prev     [2][4][4]byte
	nPrev    int
	embCodes [8]uint16
	lastIdx  int
	lastAt   time.Duration
	hasIdx   bool
	pb       *layer2.Burst
	pendRaw  [maxPend]layer2.RawBurst
	pendHint [maxPend]layer2.Hint
	pendAt   [maxPend]time.Duration
	pendS    [maxPend]int
	pendRawd [maxPend]bool
	pendV    int
	nPend    int
	noLearn  bool
	repeat   bool
	seenRaw  layer2.RawBurst
	seenAt   time.Duration
	hasSeen  bool
	ccFixed  bool
	ccVotes  [16]uint8
	embCC    enums.ColorCode
	hasCodes bool
	noCopy   noCopy
}

// NewReceiver returns a Receiver for cfg. A nil cfg means the zero
// ReceiverConfig. It returns a *dmrgo.ConfigError when the color code is
// above 15. It allocates the Receiver, a Burst for the bursts that wait for
// the sync check, and its Burst unless cfg.Scratch is set.
func NewReceiver(cfg *ReceiverConfig) (*Receiver, error) {
	var conf ReceiverConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	r := &Receiver{lostAfter: cfg.Lost, hang: cfg.Hang, b: cfg.Scratch}
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
	r.pb = layer2.NewBurst()
	r.odds = oddsIdle
	return r, nil
}

// Reset ends any call and session without events and clears the expected
// call. It keeps the config.
func (r *Receiver) Reset() {
	if r == nil {
		return
	}

	r.state = StateIdle
	r.call, r.stats, r.session = Call{}, Stats{}, Call{}
	r.inSession, r.hasStream, r.hasExpect = false, false, false
	r.track = tracker{}
	r.fragNext = 0
	r.alias.Reset()
	r.hasHeld, r.heldTA = false, false
	r.hasCC, r.ccFixed, r.ccVotes, r.misses, r.odds, r.nPend, r.hasSeen = false, false, [16]uint8{}, 0, oddsIdle, 0, false
}

// State returns the state of the Receiver.
func (r *Receiver) State() State {
	if r == nil {
		return 0
	}

	return r.state
}

// Expect gives the call of voice that arrives without an LC before until,
// such as after a Tier III grant, or a network stream that starts without
// a header. The first voice burst then starts the call with
// OriginExpected. A header or embedded LC with the same addresses that
// comes later sets Call.LC without an event, and one with other addresses
// ends the call and starts a new one.
func (r *Receiver) Expect(now time.Duration, c Call, until time.Duration) {
	if r == nil || r.b == nil {
		return
	}

	if until <= now {
		r.hasExpect = false
		return
	}
	c.Origin, c.LC = OriginExpected, pdu.LCBytes{}
	r.expect, r.expectUntil, r.hasExpect = c, until, true
	if r.state == StateIdle || r.state == StateHang {
		r.odds = oddsCap
	}
}

// Burst decodes the burst raw with the data in the Hint h, runs it
// through the Receiver as Decoded does, and appends the events to dst. A
// burst that does not decode, or has another color code than
// ReceiverConfig.ColorCode by the rules of that field, is dropped, but its Hint
// still applies, so an
// IPSC "sync not detected" burst with Hint.SyncLost still ends the call. A
// nil raw is a transport frame without a burst: only h applies, and
// Hint.SyncLost ends the call as Lost does. It allocates only when dst has
// no room for MaxEvents events.
func (r *Receiver) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Event) []Event {
	if r == nil || r.b == nil {
		return dst
	}

	if raw == nil {
		return r.receive(now, nil, nil, &h, dst)
	}
	r.opts.Kind, r.opts.VoiceBurst, r.opts.DataType = h.Kind, enums.VoiceBurstUnknown, nil
	if h.Kind == layer2.KindVoice {
		r.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		r.dataType = h.DataType
		r.opts.DataType = &r.dataType
	}
	if err := r.b.DecodeFromBytesWith(*raw, &r.opts); err != nil {
		if r.b.Kind == layer2.KindVoice && syncGated(&h) && errors.Is(err, layer2.ErrColorCodeMismatch) &&
			(r.state == StateInCall || r.state == StateLateEntry) && r.embScore(r.b, raw) > embFar {
			return r.receive(now, r.b, raw, &h, dst)
		}
		r.seen(r.b)
		dst = r.receive(now, nil, nil, &h, dst)
		if r.b.Kind == layer2.KindVoice && syncGated(&h) && (r.state == StateInCall || r.state == StateLateEntry) && errors.Is(err, layer2.ErrColorCodeMismatch) {
			dst = r.dropPending(now, dst)
			dst = r.miss(now, dst)
		}
		return dst
	}
	return r.receive(now, r.b, raw, &h, dst)
}

// Decoded runs the decoded burst b through the Receiver and appends the
// events to dst. ReceiverConfig.ColorCode does not apply: b was decoded
// with the options of the caller, so set DecodeOptions.ColorCode for the
// check. The sync check of the package doc follows h, not the options of
// the decode. A nil b means only h applies, as for a nil raw in Burst. A
// monitor that feeds a burst to several machines decodes it once with
// layer2.Burst.DecodeFromBytesWith and calls Decoded on each. For a
// transport event without a burst, such as an MMDVM TAG_LOST, call Lost.
// It allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Decoded(now time.Duration, b *layer2.Burst, h layer2.Hint, dst []Event) []Event {
	if r == nil || r.b == nil {
		return dst
	}

	return r.receive(now, b, nil, &h, dst)
}

// Lost records that the transport lost the signal, such as an
// MMDVM TAG_LOST. A call ends with EndSyncLost. It appends the events to
// dst, and allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Lost(now time.Duration, dst []Event) []Event {
	if r == nil || r.b == nil {
		return dst
	}

	dst = r.dropPending(now, dst)
	dst = r.timers(now, dst)
	return r.syncLost(now, dst)
}

// Tick runs the timers that are due at now and appends their events to dst:
// the end of a call after ReceiverConfig.Lost without a burst, and the end
// of the hang time. A Tick before Deadline does nothing. It allocates only
// when dst has no room for MaxEvents events.
func (r *Receiver) Tick(now time.Duration, dst []Event) []Event {
	if r == nil || r.b == nil {
		return dst
	}

	dst = r.dropStale(now, dst)
	return r.timers(now, dst)
}

// Deadline returns the time at which the Receiver needs a Tick. ok is
// false when no timer runs.
func (r *Receiver) Deadline() (at time.Duration, ok bool) {
	if r == nil {
		return 0, false
	}

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
	out = r.dropStale(now, out)
	out = r.timers(now, out)
	if h.SyncLost {
		out = r.syncLost(now, out)
	}
	if h.HasStream {
		if r.hasStream && h.Stream != r.stream {
			out = r.dropPending(now, out)
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
		r.seen(b)
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
	if r.nPend > 0 && b.SlotType.DataType == enums.DataTypeTerminatorWithLC {
		out = r.release(now, out, nil)
	}
	out = r.dropPending(now, out)
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
		r.callCC, r.hasCC, r.ccFixed, r.misses = b.SlotType.ColorCode, true, true, 0
		if r.state == StateInCall {
			r.odds = oddsCap
		}
		if r.state == StateInCall {
			r.track.afterHeader(h, now)
		}
		return out
	case enums.DataTypeTerminatorWithLC:
		return r.terminator(now, b, out)
	case enums.DataTypePIHeader:
		if r.state == StateInCall || r.state == StateLateEntry {
			r.last, r.odds = now, oddsCap
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
	if !ok || fl == nil || fl.FEC.Uncorrectable {
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
				r.stats.Duplicates = addSat32(r.stats.Duplicates, 1)
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
		r.heardSet, r.nPrev = 0, 0
		r.alias.Reset()
		r.stats = Stats{}
		r.start = now
	}
	r.inSession = false
	if r.hasStream {
		c.Stream = r.stream
	}
	r.call = *c
	r.state, r.last, r.odds = StateInCall, now, oddsCap
	r.hasExpect = false
	out, _ = appendEvent(out, EventCallStart, &r.call)
	if keep && r.hasHeld {
		var e *Event
		out, e = appendEvent(out, EventEmbedded, &r.call)
		e.LC = r.held
		if r.heldTA {
			out, e = appendEvent(out, EventTalkerAlias, &r.call)
			e.Text = r.heldText[:r.heldN]
		}
	}
	r.hasHeld, r.heldTA = false, false
	return out
}

func (r *Receiver) hangs(c *Call) bool {
	return r.hang >= 0 && !c.Options.IsBroadcast && (!c.Group || c.Target != constants.AllMSID)
}

func (r *Receiver) endCall(at time.Duration, reason EndReason, out []Event) []Event {
	out = r.appendEnd(at, reason, out)
	r.odds, r.nPend = oddsIdle, 0
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
	r.odds, r.nPend = oddsIdle, 0
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
	r.odds, r.nPend = oddsIdle, 0
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
	idle := r.state == StateIdle || r.state == StateHang
	if idle && raw != nil && r.opts.ColorCode != nil && b.VoiceBurst == enums.VoiceBurstA && (!r.hasExpect || now >= r.expectUntil) {
		return out
	}
	if !syncGated(h) {
		out = r.dropPending(now, out)
		if r.state == StateIdle || r.state == StateHang {
			out = r.begin(now, out)
		}
		r.odds = oddsCap
		return r.take(now, b, raw, h, out)
	}
	out = r.expire(now, out)
	cur := rawOf(b, raw)
	s := r.gatedScore(now, b, raw, cur)
	if r.nPend > 0 {
		v := min(r.pendV, oddsCap) + s
		switch {
		case v > oddsHi:
			out = r.release(now, out, b)
			return r.accept(now, b, raw, h, v, out)
		case v <= min(oddsLo, r.odds-oddsCap-60):
			if r.state == StateInCall || r.state == StateLateEntry {
				out = r.miss(now, out)
			}
			return out
		case r.nPend == maxPend:
			copy(r.pendRaw[:], r.pendRaw[1:])
			copy(r.pendHint[:], r.pendHint[1:])
			copy(r.pendAt[:], r.pendAt[1:])
			copy(r.pendS[:], r.pendS[1:])
			copy(r.pendRawd[:], r.pendRawd[1:])
			r.nPend--
			if r.state == StateInCall || r.state == StateLateEntry {
				out = r.miss(now, out)
			}
		}
		r.hold(now, h, cur, s, v, raw == nil)
		return out
	}
	v := r.odds + s
	switch {
	case v > oddsHi:
		return r.accept(now, b, raw, h, v, out)
	case v > min(oddsLo, r.odds-oddsCap-60):
		r.hold(now, h, cur, s, v, raw == nil)
		return out
	}
	r.lower(v)
	if r.state == StateInCall || r.state == StateLateEntry {
		out = r.miss(now, out)
	}
	return out
}

func (r *Receiver) expire(now time.Duration, out []Event) []Event {
	out = r.dropStale(now, out)
	if (r.state == StateIdle || r.state == StateHang) && r.nPend == 0 {
		r.hasCC, r.ccFixed, r.ccVotes = false, false, [16]uint8{}
	}
	return out
}

func (r *Receiver) gatedScore(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, cur layer2.RawBurst) int {
	r.repeat = r.hasSeen && cur == r.seenRaw && now-r.seenAt < 2*slotPeriod
	r.seenRaw, r.seenAt, r.hasSeen = cur, now, true
	s := r.score(now, b, raw)
	if r.repeat {
		s = min(s, 0)
	}
	return s
}

func (r *Receiver) hold(now time.Duration, h *layer2.Hint, raw layer2.RawBurst, s, v int, decoded bool) {
	i := r.nPend
	r.pendRaw[i], r.pendHint[i], r.pendAt[i], r.pendS[i], r.pendRawd[i] = raw, *h, now, s, decoded
	r.nPend++
	r.pendV = v
	if r.state == StateInCall || r.state == StateLateEntry {
		r.last = now
	}
}

func (r *Receiver) accept(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, v int, out []Event) []Event {
	if r.state == StateIdle || r.state == StateHang {
		out = r.begin(now, out)
	}
	r.learnCC(b)
	r.odds, r.misses = min(v, oddsCap), 0
	return r.take(now, b, raw, h, out)
}

func (r *Receiver) release(now time.Duration, out []Event, strong *layer2.Burst) []Event {
	n := r.nPend
	r.nPend = 0
	cc, known := r.startCC(strong)
	if !known {
		cc, known = r.pendingCC(n)
	}
	pos := strongPos(strong)
	idle := r.state == StateIdle || r.state == StateHang
	for i := range n {
		_ = r.pb.DecodeFromBytes(r.pendRaw[i])
		if idle {
			k := int((now - r.pendAt[i] + slotPeriod/2) / slotPeriod)
			if r.pendS[i] <= ownKeep && !r.sameCall(r.pb, &r.pendRaw[i], cc, known, pos, k) {
				continue
			}
		}
		r.noLearn = r.pendRawd[i]
		out = r.accept(r.pendAt[i], r.pb, &r.pendRaw[i], &r.pendHint[i], r.pendV, out)
		r.noLearn = false
	}
	return out
}

func (r *Receiver) startCC(strong *layer2.Burst) (enums.ColorCode, bool) {
	switch {
	case r.opts.ColorCode != nil:
		return *r.opts.ColorCode, true
	case strong != nil && strong.HasEmbeddedSignalling() && !strong.EmbeddedSignalling.FEC.Uncorrectable:
		return strong.EmbeddedSignalling.ColorCode, true
	}
	return r.callCC, r.hasCC
}

func (r *Receiver) pendingCC(n int) (enums.ColorCode, bool) {
	var votes [16]uint8
	for i := range n {
		e := r.pendRaw[i].EMB()
		if e.FEC.Uncorrectable || e.FEC.ErrorsCorrected > 1 {
			continue
		}
		votes[e.ColorCode&15]++
		if votes[e.ColorCode&15] >= 2 {
			return e.ColorCode & 15, true
		}
	}
	return 0, false
}

func strongPos(b *layer2.Burst) uint8 {
	switch {
	case b == nil:
		return 0x3F
	case b.VoiceBurst == enums.VoiceBurstA && b.SyncPattern != enums.SyncNone:
		return 1
	case !b.HasEmbeddedSignalling() || b.EmbeddedSignalling.FEC.Uncorrectable:
		return 0x3F
	}
	switch b.EmbeddedSignalling.LCSS {
	case enums.LCSSFirstFragment:
		return 1 << 1
	case enums.LCSSContinuationFragment:
		return 1<<2 | 1<<3
	case enums.LCSSLastFragment:
		return 1 << 4
	case enums.LCSSSingleFragment:
	}
	return 1 << 5
}

func (r *Receiver) sameCall(b *layer2.Burst, raw *layer2.RawBurst, cc enums.ColorCode, known bool, pos uint8, k int) bool {
	var want uint8
	for p := range layer2.VoiceBurstsPerSuperframe {
		if pos&(1<<p) != 0 {
			want |= 1 << ((p - k%6 + 12) % 6)
		}
	}
	if want&1 != 0 && syncScore(b) > 0 {
		return true
	}
	if !known || !b.HasEmbeddedSignalling() {
		return false
	}
	w, d := embWord(raw), 16
	codes := r.codes(cc)
	for i, c := range codes {
		if want&(1<<lcssPos(enums.LCSS(i&3))) != 0 || i&3 == int(enums.LCSSContinuationFragment) && want&(1<<3) != 0 {
			d = min(d, bits.OnesCount16(w^c))
		}
	}
	return d <= sameCallEMB
}

func lcssPos(l enums.LCSS) int {
	switch l {
	case enums.LCSSFirstFragment:
		return 1
	case enums.LCSSContinuationFragment:
		return 2
	case enums.LCSSLastFragment:
		return 4
	case enums.LCSSSingleFragment:
	}
	return 5
}

func (r *Receiver) dropStale(now time.Duration, out []Event) []Event {
	if r.nPend > 0 && now-r.pendAt[r.nPend-1] > maxPend*slotPeriod {
		r.nPend = 0
		r.lower(r.pendV)
	}
	return out
}

func (r *Receiver) dropPending(now time.Duration, out []Event) []Event {
	n := r.nPend
	if n == 0 {
		return out
	}
	r.nPend = 0
	r.lower(r.pendV)
	for range n {
		if r.state == StateInCall || r.state == StateLateEntry {
			out = r.miss(now, out)
		}
	}
	return out
}

func (r *Receiver) lower(v int) {
	r.odds = max(min(r.odds, v), min(r.odds, oddsLow))
}

func (r *Receiver) take(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst, h *layer2.Hint, out []Event) []Event {
	r.raw = rawOf(b, raw)
	idx, lost, p := r.track.place(b, &r.raw, h, now)
	if p == placed && idx >= 0 {
		r.lastIdx, r.lastAt, r.hasIdx = idx, now, true
	}
	if p == placed && idx >= 1 && idx <= 4 && b.HasEmbeddedSignalling() {
		r.heard[idx-1], r.heardSet = b.EmbeddedSignallingData, r.heardSet|1<<(idx-1)
	}
	switch p {
	case duplicate:
		r.stats.Duplicates = addSat32(r.stats.Duplicates, 1)
		r.last = now
		return out
	case late:
		r.stats.Late = addSat32(r.stats.Late, 1)
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

	r.stats.Bursts = addSat32(r.stats.Bursts, 1)
	r.stats.Lost = addSat32(r.stats.Lost, lost)
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

func (r *Receiver) begin(now time.Duration, out []Event) []Event {
	r.misses, r.heardSet, r.nPrev, r.hasIdx = 0, 0, 0, false
	r.track = tracker{}
	r.fragNext = 0
	r.alias.Reset()
	r.hasHeld, r.heldTA = false, false
	r.stats = Stats{}
	r.start = now
	if r.hasExpect && now < r.expectUntil {
		c := r.expect
		return r.startCall(now, &c, out)
	}
	r.hasExpect = false
	r.call = Call{}
	if r.hasStream {
		r.call.Stream = r.stream
	}
	r.state = StateLateEntry
	return out
}

func (r *Receiver) seen(b *layer2.Burst) {
	if b.SyncPattern == enums.SyncNone || b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable {
		return
	}
	switch b.SlotType.DataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypePIHeader:
	default:
		r.odds = oddsIdle
	}
}

func (r *Receiver) score(now time.Duration, b *layer2.Burst, raw *layer2.RawBurst) int {
	s := 0
	for i := range b.VoiceSlots {
		if r.call.Origin != OriginUnknown && r.call.FID == codec2FID {
			break
		}
		ea, eb := ambe.Errors(b.VoiceSlots[i])
		s += ambeAScore(ea)
		if ea >= 0 {
			s += ambeBScore(eb)
		}
	}
	emb, sync := r.embScore(b, raw), syncScore(b)
	if r.hasIdx && (r.state == StateInCall || r.state == StateLateEntry) {
		steps := max(int((now-r.lastAt+slotPeriod/2)/slotPeriod), 1)
		if (r.lastIdx+steps)%layer2.VoiceBurstsPerSuperframe != 0 {
			sync += notA
		}
	}
	frame := sync
	if emb > embFar {
		frame = max(frame, emb+r.fragScore(b))
	} else {
		frame = max(frame, emb)
	}
	if frame <= embFar || constantSlots(&b.VoiceSlots) {
		return frame + min(s, 0)
	}
	return frame + s
}

func constantSlots(v *[layer2.VoiceSlotsPerBurst][9]byte) bool {
	for i := range v {
		for j := range v[i] {
			if v[i][j] != v[0][0] {
				return false
			}
		}
	}
	return true
}

func (r *Receiver) fragScore(b *layer2.Burst) int {
	if !b.HasEmbeddedSignalling() {
		return 0
	}
	f := binary.BigEndian.Uint32(b.EmbeddedSignallingData[:])
	d := bits.OnesCount32(f)
	if r.call.Origin != OriginUnknown && r.call.LC != (pdu.LCBytes{}) {
		if r.call.LC != r.fragLC {
			r.fragLC, r.frags4 = r.call.LC, layer2.EncodeEmbeddedLC(r.call.LC)
		}
		for i := range r.frags4 {
			d = min(d, bits.OnesCount32(f^binary.BigEndian.Uint32(r.frags4[i][:])))
		}
	}
	s := ((32-d)*94 - d*257) / 10
	e := 32
	for i := range r.heard {
		if r.repeat {
			break
		}
		if r.heardSet&(1<<i) != 0 {
			e = min(e, bits.OnesCount32(f^binary.BigEndian.Uint32(r.heard[i][:])))
		}
		for j := range r.nPrev {
			e = min(e, bits.OnesCount32(f^binary.BigEndian.Uint32(r.prev[j][i][:])))
		}
	}
	s = max(s, ((32-e)*77-e*155)/10)
	return max(s-37, 0)
}

func syncScore(b *layer2.Burst) int {
	switch b.NearestSync {
	case enums.SyncBSSourcedVoice, enums.SyncMSSourcedVoice, enums.SyncTDMA1Voice, enums.SyncTDMA2Voice:
	default:
		return oddsIdle
	}
	d := b.SyncDistance
	return ((48-d)*94 - d*257 - 111) / 10
}

func (r *Receiver) embScore(b *layer2.Burst, raw *layer2.RawBurst) int {
	emb := &b.EmbeddedSignalling
	cc, known := r.callCC, r.hasCC
	if raw != nil && r.opts.ColorCode != nil {
		cc, known = *r.opts.ColorCode, true
	}
	switch {
	case !b.HasEmbeddedSignalling():
		return oddsIdle
	case known && raw != nil:
		w := embWord(raw)
		d := 16
		for _, c := range r.codes(cc) {
			d = min(d, bits.OnesCount16(w^c))
		}
		return ((16-d)*94-d*257)/10 - 33
	case emb.FEC.Uncorrectable:
		return -20
	case known && emb.ColorCode != cc:
		return -16
	case !known:
		return embScore(emb.FEC.ErrorsCorrected) - 44
	}
	return embScore(emb.FEC.ErrorsCorrected)
}

func (r *Receiver) codes(cc enums.ColorCode) *[8]uint16 {
	if r.hasCodes && r.embCC == cc {
		return &r.embCodes
	}
	for i := range r.embCodes {
		e := pdu.EmbeddedSignalling{ColorCode: cc, PreemptionAndPowerControlIndicator: i&4 != 0, LCSS: enums.LCSS(i & 3)}
		w, _ := e.Encode()
		r.embCodes[i] = uint16(w[0])<<8 | uint16(w[1])
	}
	r.embCC, r.hasCodes = cc, true
	return &r.embCodes
}

func embWord(raw *layer2.RawBurst) uint16 {
	var w uint16
	for _, start := range [2]int{108, 148} {
		for i := start; i < start+8; i++ {
			w = w<<1 | uint16(raw[i/8]>>(7-i%8)&1)
		}
	}
	return w
}

func (r *Receiver) learnCC(b *layer2.Burst) {
	emb := &b.EmbeddedSignalling
	if r.ccFixed || r.noLearn || !b.HasEmbeddedSignalling() || emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected != 0 {
		return
	}
	c := emb.ColorCode & 15
	r.ccVotes[c] = min(r.ccVotes[c]+1, 255)
	if r.ccVotes[c] >= 2 && (!r.hasCC || r.ccVotes[c] > r.ccVotes[r.callCC&15]) {
		r.callCC, r.hasCC = c, true
	}
}

func syncGated(h *layer2.Hint) bool {
	return h.Kind == layer2.KindUnknown && h.VoiceBurst == enums.VoiceBurstUnknown && !h.HasSeq
}

func (r *Receiver) miss(now time.Duration, out []Event) []Event {
	r.misses++
	if r.misses < maxMisses {
		return out
	}
	r.misses = 0
	if r.state == StateInCall {
		return r.endCall(now, EndSyncLost, out)
	}
	return r.abandon(now, out)
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
	lc, res := layer2.DecodeEmbeddedLC(r.frags)
	if res.Uncorrectable && r.nPrev == len(r.prev) {
		var m [4][4]byte
		for i := range m {
			for j := range m[i] {
				x, y, z := r.frags[i][j], r.prev[0][i][j], r.prev[1][i][j]
				m[i][j] = x&y | y&z | x&z
			}
		}
		lc, res = layer2.DecodeEmbeddedLC(m)
	}
	r.prev[1], r.prev[0] = r.prev[0], r.frags
	r.nPrev = min(r.nPrev+1, len(r.prev))
	if res.Uncorrectable {
		return pdu.LCBytes{}, false
	}
	return lc, true
}

func (r *Receiver) embedded(lc pdu.LCBytes, out []Event) []Event {
	r.lc = pdu.DecodeFullLinkControlBytes(lc)
	if r.state == StateLateEntry {
		r.held, r.hasHeld = r.lc, true
		n, ok, err := r.alias.AddInto(&r.text, &r.lc)
		if ok && err == nil {
			r.heldText, r.heldN, r.heldTA = r.text, n, true
		}
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

func addSat32(a uint32, n int) uint32 {
	sum := int64(a) + int64(n)
	if sum >= 0xFFFFFFFF {
		return 0xFFFFFFFF
	}
	if sum <= 0 {
		return 0
	}
	return uint32(sum)
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
