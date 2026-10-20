package bs

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/vocoder"
	"github.com/USA-RedDragon/dmrgo/v2/voice"
)

// MaxOutputs is the most outputs that one call of a Repeater method
// appends.
const MaxOutputs = 10

const (
	// SlotTime is the time of one outbound burst with its CACH, and the
	// period of Tick in ModeRF (ETSI TS 102 361-1 clause 4.2.2).
	SlotTime = 30 * time.Millisecond
	// DefaultInactivity is T_MSInactiv, the time without inbound activity
	// after which the Repeater hibernates (ETSI TS 102 361-1 clause F.1).
	DefaultInactivity = 5 * time.Second
	// DefaultPending is the number of refused sources per slot that wait
	// for the slot.
	DefaultPending = 8
	// MaxPending is the most sources per slot that wait for the slot.
	MaxPending = 16
	// DefaultPendingAge is the longest time a source waits for a slot.
	DefaultPendingAge = 30 * time.Second
	// DefaultDataQueue is the number of data and CSBK bursts per slot that
	// wait for the outbound channel.
	DefaultDataQueue = 4
	// MaxDataQueue is the most data and CSBK bursts per slot that wait for
	// the outbound channel.
	MaxDataQueue = 16
)

const burstPeriod = 2 * SlotTime

// Config configures New. A nil config means the zero Config. A Duration of
// 0 means the default in its doc and a negative one turns the timer off.
type Config struct {
	// Mode selects RF repeater or network arbiter.
	Mode Mode
	// ColorCode is the color code of the bursts the Repeater sends, and the
	// only one it takes from RF.
	ColorCode enums.ColorCode
	// Activation selects what wakes up the Repeater in ModeRF.
	Activation Activation
	// CACH makes the Repeater send the CACH and Idle bursts in ModeRF: true
	// for an SDR or a microcontroller modulator, false for an MMDVM modem,
	// whose firmware makes them.
	CACH bool
	// CallHang is the call hang time T_CallHt. 0 means constants.TCallHang
	// (3 s). A broadcast call and a call to constants.AllMSID have none
	// (ETSI TS 102 361-2 clause 5.3.2.3.2).
	CallHang time.Duration
	// DataHang is the data hang time T_DataHngtime after confirmed data. 0
	// means constants.TDataHangtime (180 ms).
	DataHang time.Duration
	// Inactivity is T_MSInactiv. 0 means DefaultInactivity. A negative
	// value keeps the transmitter on once it wakes up.
	Inactivity time.Duration
	// Lost is the time without a burst from the owner of a slot after which
	// its call ends. 0 means voice.DefaultLost (1 s).
	Lost time.Duration
	// Timeout is the longest call. 0 means constants.TTO (180 s). A longer
	// call ends with a Terminator and the rest of it is dropped.
	Timeout time.Duration
	// Headers is the number of Voice LC Headers that start a repeated call.
	// 0 means 1, at most voice.MaxHeaders.
	Headers int
	// Terminators is the number of Terminators with LC that end a repeated
	// call, before the call hang time. 0 means 1, at most
	// voice.MaxTerminators.
	Terminators int
	// Pending is the number of refused sources per slot that wait for the
	// slot and get it in turn when it is free. 0 means DefaultPending, at
	// most MaxPending. A negative value queues none: the first burst after
	// the slot is free takes it.
	Pending int
	// PendingAge is the longest time a source waits for a slot. 0 means
	// DefaultPendingAge.
	PendingAge time.Duration
	// DataQueue is the number of data and CSBK bursts per slot that wait
	// for the outbound channel in ModeRF. 0 means DefaultDataQueue, at most
	// MaxDataQueue.
	DataQueue int
	// Silence is the 72-bit vocoder slot that fills lost voice. The zero
	// value means vocoder.AMBESilenceSlot.
	Silence [9]byte
	// Lead makes Tick emit an outbound burst this long before its Due time
	// in ModeRF, so that a transmit FIFO can be filled ahead.
	Lead time.Duration
	// Scratch, when set, is the Burst that the Repeater decodes into. It
	// can be shared with other machines on the same goroutine, because the
	// Repeater is done with it when its method returns. Nil means the
	// Repeater makes its own.
	Scratch *layer2.Burst
}

type queued struct {
	burst layer2.RawBurst
	to    Dest
}

type pending struct {
	from  Source
	first time.Duration
	last  time.Duration
}

type slot struct {
	id    enums.Slot
	rx    *voice.Receiver
	tx    *voice.Transmitter
	state SlotState
	owner Source

	call      voice.Call
	hasCall   bool
	service   Service
	reason    Reason
	last      time.Duration
	hangUntil time.Duration
	hang      layer2.RawBurst
	dataLeft  int
	confirmed bool
	activity  enums.ActivityID
	hash      byte

	queue []queued
	head  int
	count int
	pend  [MaxPending]pending
	npend int
}

// Repeater is the base station of one carrier with two slots, as the
// package doc describes. Make it with New. A Repeater is not safe for
// concurrent use and must not be copied.
type Repeater struct {
	mode       Mode
	cc         enums.ColorCode
	activation Activation
	cach       bool
	callHang   time.Duration
	dataHang   time.Duration
	inactivity time.Duration
	lost       time.Duration
	pendingMax int
	pendingAge time.Duration
	lead       time.Duration

	b        *layer2.Burst
	opts     layer2.DecodeOptions
	dataType enums.DataType
	slots    [2]slot
	ev       []voice.Event
	txOut    []voice.Output

	keyed        bool
	lastActivity time.Duration
	next         time.Duration
	nextSlot     enums.Slot
	gen          CACH
	slc          pdu.ShortLC
	au           pdu.ShortLCActivityUpdate
	null         pdu.ShortLCNullMessage
	idle         layer2.RawBurst
	lc           pdu.LCBytes
	noCopy       noCopy
}

// New returns a Repeater for cfg. A nil cfg means the zero Config. It
// returns a *ConfigError for a field out of range. It allocates the
// Repeater, a voice.Receiver and a voice.Transmitter per slot, the data
// queues, and the scratch Burst unless cfg.Scratch is set.
func New(cfg *Config) (*Repeater, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	r := &Repeater{mode: cfg.Mode, cc: cfg.ColorCode, activation: cfg.Activation, cach: cfg.CACH && cfg.Mode == ModeRF,
		callHang: orDefault(cfg.CallHang, constants.TCallHang), dataHang: orDefault(cfg.DataHang, constants.TDataHangtime),
		inactivity: orDefault(cfg.Inactivity, DefaultInactivity), lost: orDefault(cfg.Lost, voice.DefaultLost),
		pendingMax: cfg.Pending, pendingAge: orDefault(cfg.PendingAge, DefaultPendingAge), lead: cfg.Lead, b: cfg.Scratch}
	if r.pendingMax == 0 {
		r.pendingMax = DefaultPending
	}
	dataQueue := cfg.DataQueue
	if dataQueue == 0 {
		dataQueue = DefaultDataQueue
	}
	switch {
	case cfg.Mode > ModeNetwork:
		return nil, errMode
	case cfg.ColorCode > 15:
		return nil, errColorCode
	case cfg.Activation > ActivationWakeup:
		return nil, errActivation
	case r.pendingMax > MaxPending:
		return nil, errPending
	case cfg.Headers < 0 || cfg.Headers > voice.MaxHeaders:
		return nil, errHeaders
	case cfg.Terminators < 0 || cfg.Terminators > voice.MaxTerminators:
		return nil, errTerminators
	case dataQueue < 0 || dataQueue > MaxDataQueue:
		return nil, errDataQueue
	}
	if r.lost < 0 {
		r.lost = voice.DefaultLost
	}
	silence := cfg.Silence
	if silence == [9]byte{} {
		silence = vocoder.AMBESilenceSlot()
	}
	if r.b == nil {
		r.b = layer2.NewBurst()
	}
	headers := max(cfg.Headers, 1)
	for i := range r.slots {
		s := &r.slots[i]
		s.id = enums.Slot(i + 1)
		rx, err := voice.NewReceiver(&voice.ReceiverConfig{Lost: r.lost, Hang: -1, Scratch: r.b})
		if err != nil {
			return nil, err
		}
		tx, err := voice.NewTransmitter(&voice.TransmitterConfig{ColorCode: r.cc, Silence: silence, Headers: cfg.Headers,
			Terminators: cfg.Terminators, Timeout: cfg.Timeout, Queue: layer2.VoiceSlotsPerBurst * (headers + 2)})
		if err != nil {
			return nil, err
		}
		s.rx, s.tx = rx, tx
		s.queue = make([]queued, dataQueue)
	}
	r.ev = make([]voice.Event, 0, voice.MaxEvents)
	r.txOut = make([]voice.Output, 0, voice.MaxOutputs)
	r.idle, _ = layer2.BuildDataBurst(pdu.PRFillBits(), enums.DataTypeIdle, r.cc, layer2.SyncSourceBS)
	r.Reset()
	return r, nil
}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// Reset ends every call without outputs, empties the queues and puts the
// Repeater in StateHibernating, or keyed in ModeNetwork. It keeps the
// config.
func (r *Repeater) Reset() {
	for i := range r.slots {
		s := &r.slots[i]
		s.rx.Reset()
		s.tx.Reset()
		s.state, s.hasCall, s.head, s.count, s.npend = SlotIdle, false, 0, 0, 0
	}
	r.keyed = r.mode == ModeNetwork
	r.gen.Reset()
}

// State returns the state of both slots.
func (r *Repeater) State() State {
	if !r.keyed && r.mode == ModeRF {
		return StateHibernating
	}
	b1, b2 := r.slots[0].repeating(), r.slots[1].repeating()
	switch {
	case b1 && b2:
		return StateRepeatingBoth
	case b1:
		return StateRepeatingSlot1
	case b2:
		return StateRepeatingSlot2
	}
	return StateHangtime
}

// SlotState returns the state of slot s, and SlotIdle for a slot other than
// Slot1 or Slot2.
func (r *Repeater) SlotState(s enums.Slot) SlotState {
	sl := r.slot(s)
	if sl == nil {
		return SlotIdle
	}
	return sl.state
}

func (r *Repeater) slot(s enums.Slot) *slot {
	if s != enums.Slot1 && s != enums.Slot2 {
		return nil
	}
	return &r.slots[s-1]
}

func (s *slot) repeating() bool {
	return s.state == SlotVoice || s.state == SlotData || s.state == SlotRejected
}

func (s *slot) busy() bool {
	return s.state != SlotIdle
}

// Inbound takes a burst received on slot h.Slot from the source from, and
// appends the outputs to dst. The burst is decoded with what h says about
// it. RF bursts with another color code than Config.ColorCode are dropped.
// It returns ErrSlot when h.Slot is not Slot1 or Slot2. It allocates only
// when dst has no room for MaxOutputs outputs.
func (r *Repeater) Inbound(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, from Source, dst []Output) ([]Output, error) {
	s := r.slot(h.Slot)
	if s == nil {
		return dst, ErrSlot
	}
	dst = r.timers(now, dst)
	r.opts = layer2.DecodeOptions{Kind: h.Kind, VoiceBurst: enums.VoiceBurstUnknown}
	if h.Kind == layer2.KindVoice {
		r.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		r.dataType = h.DataType
		r.opts.DataType = &r.dataType
	}
	if from.RF {
		r.opts.ColorCode = &r.cc
	}
	b := r.b
	if b.DecodeFromBytesWith(*raw, &r.opts) != nil || b.Kind == layer2.KindData && b.SlotType.FEC.Uncorrectable {
		b = nil
	}
	wakeup := b != nil && isWakeup(b, raw)
	if !r.keyed {
		if b == nil || from.RF && r.activation == ActivationWakeup && !wakeup {
			return dst, nil
		}
		dst = r.key(now, dst)
	}
	if b == nil {
		switch {
		case s.owner != from || !s.repeating():
		case !from.RF:
			dst = r.events(now, s, s.rx.Burst(now, raw, h, r.ev[:0]), dst)
		case h.SyncLost:
			dst = r.events(now, s, s.rx.Lost(now, r.ev[:0]), dst)
		}
		return r.flush(now, dst), nil
	}
	r.lastActivity = now
	if wakeup {
		return dst, nil
	}
	if b.Kind == layer2.KindVoice {
		dst = r.voice(now, s, b, nil, &h, from, dst)
		return r.flush(now, dst), nil
	}
	switch b.SlotType.DataType {
	case enums.DataTypeVoiceLCHeader, enums.DataTypePIHeader, enums.DataTypeTerminatorWithLC:
		var lc *pdu.LCBytes
		cw, _ := raw.BPTCPayload()
		if octets, res, err := pdu.DecodeFullLCCodeword(cw, b.SlotType.DataType); err == nil && !res.Uncorrectable {
			r.lc = octets
			lc = &r.lc
		}
		dst = r.voice(now, s, b, lc, &h, from, dst)
	case enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation, enums.DataTypeUnifiedSingleBlock:
		dst = r.single(now, s, raw, b.SlotType.DataType, from, dst)
	case enums.DataTypeDataHeader:
		dst = r.dataHeader(now, s, raw, from, dst)
	case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
		dst = r.dataBlock(now, s, raw, b.SlotType.DataType, from, dst)
	case enums.DataTypeIdle, enums.DataTypeReserved:
	}
	return r.flush(now, dst), nil
}

func isWakeup(b *layer2.Burst, raw *layer2.RawBurst) bool {
	if b.Kind != layer2.KindData || b.SlotType.DataType != enums.DataTypeCSBK {
		return false
	}
	p, res := raw.BPTCPayload()
	c := pdu.CSBKBytes(p)
	return !res.Uncorrectable && c.CRCOK() && c.Opcode() == pdu.CSBKBSOutboundActivation && c.FID() == 0
}

func (r *Repeater) key(now time.Duration, dst []Output) []Output {
	r.keyed, r.lastActivity = true, now
	r.next, r.nextSlot = now, enums.Slot2
	r.gen.Reset()
	dst, _ = appendOutput(dst, OutputKey)
	return dst
}

// Lost tells the Repeater that the transport lost the signal of slot s,
// such as an MMDVM TAG_LOST. A call of from on s ends. It returns ErrSlot
// for a slot other than Slot1 or Slot2, and allocates only when dst has no
// room for MaxOutputs outputs.
func (r *Repeater) Lost(now time.Duration, s enums.Slot, from Source, dst []Output) ([]Output, error) {
	sl := r.slot(s)
	if sl == nil {
		return dst, ErrSlot
	}
	dst = r.timers(now, dst)
	if sl.owner == from && sl.repeating() {
		dst = r.events(now, sl, sl.rx.Lost(now, r.ev[:0]), dst)
		if sl.state == SlotData {
			dst = r.endData(now, sl, voice.EndSyncLost, dst)
		}
	}
	return r.flush(now, dst), nil
}

// Reject refuses the call on slot s, as for an access rule of the caller,
// typically on its OutputCallStart. The Repeater drops the rest of the call
// and gives it no hang time; OutputCallEnd follows when the call ends. It
// returns ErrSlot for a slot other than Slot1 or Slot2, and does nothing
// for a slot without a call.
func (r *Repeater) Reject(s enums.Slot) error {
	sl := r.slot(s)
	if sl == nil {
		return ErrSlot
	}
	if sl.state == SlotVoice || sl.state == SlotData {
		sl.tx.Reset()
		sl.head, sl.count = 0, 0
		sl.state, sl.reason = SlotRejected, ReasonRejected
	}
	return nil
}

// Tick runs the timers that are due at now and, in ModeRF while keyed,
// appends the next outbound burst when now has reached its Due time minus
// Config.Lead. Each Tick appends at most one outbound burst; a late caller
// calls Tick until Deadline is in the future. In ModeNetwork it appends the
// regenerated bursts that are due. It allocates only when dst has no room
// for MaxOutputs outputs.
func (r *Repeater) Tick(now time.Duration, dst []Output) []Output {
	dst = r.timers(now, dst)
	if r.mode == ModeNetwork {
		return r.flush(now, dst)
	}
	if r.keyed && now >= r.next-r.lead {
		dst = r.turn(dst)
	}
	return dst
}

// Deadline returns the time at which the Repeater needs a Tick. ok is false
// when no timer runs.
func (r *Repeater) Deadline() (at time.Duration, ok bool) {
	var d deadline
	if r.mode == ModeRF && r.keyed {
		d.add(r.next - r.lead)
		if r.inactivity >= 0 {
			d.add(r.lastActivity + r.inactivity)
		}
	}
	for i := range r.slots {
		s := &r.slots[i]
		if t, tok := s.rx.Deadline(); tok {
			d.add(t)
		}
		if t, tok := s.tx.Deadline(); tok && r.mode == ModeNetwork {
			d.add(t)
		}
		switch s.state {
		case SlotVoice, SlotData, SlotRejected:
			d.add(s.last + r.lost)
		case SlotCallHang, SlotDataHang:
			d.add(s.hangUntil)
		case SlotIdle:
		}
		for j := range s.npend {
			d.add(min(s.pend[j].last+r.lost, s.pend[j].first+r.pendingAge))
		}
	}
	return d.at, d.ok
}

type deadline struct {
	at time.Duration
	ok bool
}

func (d *deadline) add(t time.Duration) {
	if !d.ok || t < d.at {
		d.at, d.ok = t, true
	}
}

func (r *Repeater) timers(now time.Duration, dst []Output) []Output {
	for i := range r.slots {
		s := &r.slots[i]
		dst = r.events(now, s, s.rx.Tick(now, r.ev[:0]), dst)
		switch s.state {
		case SlotVoice, SlotRejected:
			if now >= s.last+r.lost {
				s.rx.Reset()
				dst = r.endVoice(now, s, voice.EndLost, dst)
			}
		case SlotData:
			if now >= s.last+r.lost {
				dst = r.endData(now, s, voice.EndLost, dst)
			}
		case SlotCallHang, SlotDataHang:
			if now >= s.hangUntil {
				s.state = SlotIdle
			}
		case SlotIdle:
		}
		n := 0
		for j := range s.npend {
			p := s.pend[j]
			if now < p.last+r.lost && now < p.first+r.pendingAge {
				s.pend[n] = p
				n++
			}
		}
		s.npend = n
	}
	if r.mode == ModeRF && r.keyed && r.inactivity >= 0 && now >= r.lastActivity+r.inactivity && r.quiet() {
		r.keyed = false
		dst, _ = appendOutput(dst, OutputDekey)
	}
	return dst
}

func (r *Repeater) quiet() bool {
	for i := range r.slots {
		s := &r.slots[i]
		if s.state != SlotIdle || s.count > 0 || s.tx.State() != voice.TransmitterIdle {
			return false
		}
	}
	return true
}

func (r *Repeater) admit(now time.Duration, s *slot, from Source, lc *pdu.LCBytes, dst []Output) (bool, []Output) {
	switch s.state {
	case SlotVoice, SlotData, SlotRejected:
		if s.owner == from {
			return true, dst
		}
		return false, r.refuse(now, s, from, ReasonSlotBusy, dst)
	case SlotCallHang:
		if s.owner == from || lc != nil && sameTarget(lc, &s.call) {
			return true, dst
		}
		return false, r.refuse(now, s, from, ReasonHangTime, dst)
	case SlotIdle, SlotDataHang:
	}
	if r.pendingMax > 0 && s.npend > 0 && s.pend[0].from != from {
		return false, r.refuse(now, s, from, ReasonSlotBusy, dst)
	}
	for j := range s.npend {
		if s.pend[j].from == from {
			copy(s.pend[j:s.npend], s.pend[j+1:s.npend])
			s.npend--
			break
		}
	}
	return true, dst
}

func sameTarget(lc *pdu.LCBytes, c *voice.Call) bool {
	target, _, ok := lc.Addresses()
	group, _ := lc.Group()
	return ok && target == c.Target && group == c.Group
}

func (r *Repeater) refuse(now time.Duration, s *slot, from Source, reason Reason, dst []Output) []Output {
	for j := range s.npend {
		if s.pend[j].from == from {
			s.pend[j].last = now
			return dst
		}
	}
	if s.npend == len(s.pend) || r.pendingMax > 0 && s.npend >= r.pendingMax {
		return dst
	}
	s.pend[s.npend] = pending{from: from, first: now, last: now}
	s.npend++
	var o *Output
	dst, o = appendOutput(dst, OutputRejected)
	o.Slot, o.From, o.Reason = s.id, from, reason
	return dst
}

func (r *Repeater) voice(now time.Duration, s *slot, b *layer2.Burst, lc *pdu.LCBytes, h *layer2.Hint, from Source, dst []Output) []Output {
	terminator := b.Kind == layer2.KindData && b.SlotType.DataType == enums.DataTypeTerminatorWithLC
	if terminator && (s.owner != from || !s.repeating()) {
		return dst
	}
	ok, dst := r.admit(now, s, from, lc, dst)
	if !ok {
		return dst
	}
	if s.state == SlotData {
		dst = r.endData(now, s, voice.EndSuperseded, dst)
	}
	if !s.repeating() {
		s.rx.Reset()
		s.state, s.owner, s.service, s.hasCall = SlotVoice, from, ServiceVoice, false
	}
	s.last = now
	return r.events(now, s, s.rx.Decoded(now, b, *h, r.ev[:0]), dst)
}

func (r *Repeater) events(now time.Duration, s *slot, ev []voice.Event, dst []Output) []Output {
	for i := range ev {
		e := &ev[i]
		switch e.Kind {
		case voice.EventCallStart:
			switch {
			case s.state == SlotRejected && !s.hasCall:
				s.call, s.hasCall = e.Call, true
				continue
			case s.state != SlotVoice && s.state != SlotRejected:
				continue
			}
			s.state, s.call, s.hasCall = SlotVoice, e.Call, true
			s.activity, s.hash = activityOf(&e.Call), hashOf(e.Call.Target)
			if s.tx.Relay(now, e) != nil {
				s.tx.Reset()
				_ = s.tx.Relay(now, e)
			}
			var o *Output
			dst, o = appendOutput(dst, OutputCallStart)
			o.Slot, o.From, o.Service, o.Call = s.id, s.owner, ServiceVoice, e.Call
		case voice.EventCallEnd:
			dst = r.endVoice(now, s, e.Reason, dst)
		case voice.EventVoice, voice.EventPIHeader:
			if s.state == SlotVoice {
				_ = s.tx.Relay(now, e)
			}
		case voice.EventEmbedded, voice.EventTalkerAlias, voice.EventSessionStart, voice.EventSessionEnd:
		}
	}
	return dst
}

func (r *Repeater) endVoice(now time.Duration, s *slot, reason voice.EndReason, dst []Output) []Output {
	if s.state != SlotVoice && s.state != SlotRejected {
		return dst
	}
	rejected := s.state == SlotRejected
	s.state = SlotIdle
	if !s.hasCall {
		return dst
	}
	end := voice.Event{Kind: voice.EventCallEnd, Call: s.call, Reason: reason}
	if !rejected {
		_ = s.tx.Relay(now, &end)
	}
	var o *Output
	dst, o = appendOutput(dst, OutputCallEnd)
	o.Slot, o.From, o.Service, o.Call, o.End = s.id, s.owner, ServiceVoice, s.call, reason
	if reason == voice.EndSuperseded {
		s.state, s.hasCall = SlotVoice, false
		return dst
	}
	if rejected || r.callHang < 0 || s.call.Options.IsBroadcast || s.call.Group && s.call.Target == constants.AllMSID {
		return dst
	}
	cw, err := s.call.LC.Codeword(enums.DataTypeTerminatorWithLC)
	if err != nil {
		return dst
	}
	s.hang, _ = layer2.BuildDataBurst(cw, enums.DataTypeTerminatorWithLC, r.cc, layer2.SyncSourceBS)
	s.state, s.hangUntil = SlotCallHang, now+r.callHang
	return dst
}

func (r *Repeater) single(now time.Duration, s *slot, raw *layer2.RawBurst, dt enums.DataType, from Source, dst []Output) []Output {
	p, res := raw.BPTCPayload()
	if res.Uncorrectable || dt == enums.DataTypeCSBK && !pdu.CSBKBytes(p).CRCOK() {
		return dst
	}
	ok, dst := r.admit(now, s, from, nil, dst)
	if !ok {
		return dst
	}
	if s.repeating() {
		s.last = now
	}
	out, err := layer2.BuildDataBurst(p, dt, r.cc, layer2.SyncSourceBS)
	if err != nil {
		return dst
	}
	return r.send(now, s, out, from, dst)
}

func (r *Repeater) dataHeader(now time.Duration, s *slot, raw *layer2.RawBurst, from Source, dst []Output) []Output {
	p, res := raw.BPTCPayload()
	h := pdu.DataHeaderBytes(p)
	if res.Uncorrectable || !h.CRCOK() {
		return dst
	}
	if s.state == SlotData && s.owner == from && h.Format() == pdu.FormatProprietary {
		return r.dataBlock(now, s, raw, enums.DataTypeDataHeader, from, dst)
	}
	free := s.state == SlotDataHang && h.Format() == pdu.FormatResponsePacket
	if !free {
		var ok bool
		if ok, dst = r.admit(now, s, from, nil, dst); !ok {
			return dst
		}
	}
	target, source, _ := h.Addresses()
	group, _ := h.Group()
	if s.state == SlotVoice || s.state == SlotRejected {
		s.rx.Reset()
		dst = r.endVoice(now, s, voice.EndDataSync, dst)
	}
	start := s.state != SlotData || s.owner != from
	s.state, s.owner, s.service, s.last = SlotData, from, ServiceData, now
	s.call, s.hasCall = voice.Call{Group: group, Target: target, Source: source}, true
	s.activity, s.hash = enums.ActivityIndividualData, hashOf(target)
	if group {
		s.activity = enums.ActivityGroupData
	}
	s.dataLeft, s.confirmed = -1, false
	switch h.Format() {
	case pdu.FormatConfirmed, pdu.FormatUnconfirmed, pdu.FormatResponsePacket:
		s.dataLeft = int(p[8] & 0x7F)
		s.confirmed = h.Format() == pdu.FormatConfirmed
	case pdu.FormatShortDataDefined, pdu.FormatShortDataRawOrStatusPrecoded:
		s.dataLeft = int(p[0]>>4&0x3)<<4 | int(p[1]&0xF)
	case pdu.FormatUnifiedDataTransport:
		s.dataLeft = int(p[8]&0x3) + 1
	case pdu.FormatProprietary:
	}
	if s.confirmed {
		s.hang = tdlc(&p, r.cc)
	}
	if start {
		var o *Output
		dst, o = appendOutput(dst, OutputCallStart)
		o.Slot, o.From, o.Service, o.Call = s.id, from, ServiceData, s.call
	}
	out, err := layer2.BuildDataBurst(p, enums.DataTypeDataHeader, r.cc, layer2.SyncSourceBS)
	if err == nil {
		dst = r.send(now, s, out, from, dst)
	}
	if s.dataLeft == 0 {
		dst = r.endData(now, s, 0, dst)
	}
	return dst
}

func tdlc(p *[12]byte, cc enums.ColorCode) layer2.RawBurst {
	var lc pdu.LCBytes
	_ = lc.SetFLCO(enums.FLCOTerminatorDataLinkControl)
	copy(lc[2:8], p[2:8])
	lc[8] = p[0]&0xC0 | p[8]&0x80>>2 | p[9]&0x80>>4 | p[9]>>4&0x07
	var out layer2.RawBurst
	if cw, err := lc.Codeword(enums.DataTypeTerminatorWithLC); err == nil {
		out, _ = layer2.BuildDataBurst(cw, enums.DataTypeTerminatorWithLC, cc, layer2.SyncSourceBS)
	}
	return out
}

func (r *Repeater) dataBlock(now time.Duration, s *slot, raw *layer2.RawBurst, dt enums.DataType, from Source, dst []Output) []Output {
	if s.state != SlotData || s.owner != from {
		if s.state != SlotIdle && s.owner != from {
			_, dst = r.admit(now, s, from, nil, dst)
		}
		return dst
	}
	var out layer2.RawBurst
	var err error
	switch dt {
	case enums.DataTypeRate34:
		p, res := raw.Rate34Payload()
		if res.Uncorrectable {
			return dst
		}
		out, err = layer2.BuildRate34DataBurst(p, r.cc, layer2.SyncSourceBS)
	case enums.DataTypeRate1:
		p, _ := raw.Rate1Payload()
		out, err = layer2.BuildRate1DataBurst(p, r.cc, layer2.SyncSourceBS)
	default:
		p, res := raw.BPTCPayload()
		if res.Uncorrectable {
			return dst
		}
		out, err = layer2.BuildDataBurst(p, dt, r.cc, layer2.SyncSourceBS)
	}
	if err != nil {
		return dst
	}
	s.last = now
	dst = r.send(now, s, out, from, dst)
	if s.dataLeft > 0 {
		s.dataLeft--
		if s.dataLeft == 0 {
			dst = r.endData(now, s, 0, dst)
		}
	}
	return dst
}

func (r *Repeater) endData(now time.Duration, s *slot, reason voice.EndReason, dst []Output) []Output {
	if s.state != SlotData {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputCallEnd)
	o.Slot, o.From, o.Service, o.Call, o.End = s.id, s.owner, ServiceData, s.call, reason
	s.state = SlotIdle
	if s.confirmed && reason == 0 && r.dataHang >= 0 {
		s.state, s.hangUntil = SlotDataHang, now+r.dataHang
	}
	return dst
}

func (r *Repeater) send(now time.Duration, s *slot, b layer2.RawBurst, from Source, dst []Output) []Output {
	if r.mode == ModeNetwork {
		var o *Output
		dst, o = appendOutput(dst, OutputBurst)
		o.Slot, o.To, o.Burst, o.Due, o.From = s.id, ToNetwork, b, now, from
		return dst
	}
	if s.count == len(s.queue) {
		return dst
	}
	to := ToRF
	if from.RF {
		to |= ToNetwork
	}
	s.queue[(s.head+s.count)%len(s.queue)] = queued{burst: b, to: to}
	s.count++
	return dst
}

func (r *Repeater) flush(now time.Duration, dst []Output) []Output {
	if r.mode != ModeNetwork {
		return dst
	}
	for i := range r.slots {
		s := &r.slots[i]
		var b layer2.RawBurst
		var due time.Duration
		var ok bool
		if b, due, ok, dst = r.pull(now, s, dst); ok {
			var o *Output
			dst, o = appendOutput(dst, OutputBurst)
			o.Slot, o.To, o.Burst, o.Due, o.From = s.id, ToNetwork, b, due, s.owner
		}
	}
	return dst
}

func (r *Repeater) pull(now time.Duration, s *slot, dst []Output) (layer2.RawBurst, time.Duration, bool, []Output) {
	var b layer2.RawBurst
	var due time.Duration
	ok := false
	outs := s.tx.Tick(now, r.txOut[:0])
	for i := range outs {
		if outs[i].Kind == voice.OutputBurst {
			b, due, ok = outs[i].Burst, outs[i].Due, true
		}
	}
	if s.tx.State() == voice.TransmitterTimedOut {
		s.tx.Reset()
		if s.state == SlotVoice {
			s.state, s.reason = SlotRejected, ReasonTimeout
			var o *Output
			dst, o = appendOutput(dst, OutputRejected)
			o.Slot, o.From, o.Reason, o.Call = s.id, s.owner, ReasonTimeout, s.call
		}
	}
	return b, due, ok, dst
}

func (r *Repeater) turn(dst []Output) []Output {
	s := &r.slots[r.nextSlot-1]
	due := r.next
	r.next += SlotTime
	r.nextSlot = r.nextSlot.Other()

	b, _, ok, dst := r.pull(due+burstPeriod-1, s, dst)
	to := ToRF
	if ok && s.owner.RF {
		to |= ToNetwork
	}
	switch {
	case ok:
	case s.count > 0:
		q := &s.queue[s.head]
		b, to, ok = q.burst, q.to, true
		s.head, s.count = (s.head+1)%len(s.queue), s.count-1
	case s.state == SlotCallHang || s.state == SlotDataHang:
		b, ok = s.hang, true
	case r.cach:
		b, ok = r.idle, true
	}
	if !ok {
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst)
	o.Slot, o.To, o.Burst, o.Due, o.From = s.id, to, b, due, s.owner
	if r.cach {
		if r.gen.First() {
			r.shortLC()
		}
		if c, err := r.gen.Next(s.id, r.slots[s.id.Other()-1].busy(), &r.slc); err == nil {
			o.CACH, o.HasCACH = c, true
		}
	}
	return dst
}

func (r *Repeater) shortLC() {
	s1, s2 := &r.slots[0], &r.slots[1]
	if !s1.busy() && !s2.busy() {
		r.slc = pdu.ShortLC{SLCO: enums.SLCONullMessage, NullMessage: &r.null}
		return
	}
	r.au = pdu.ShortLCActivityUpdate{}
	if s1.busy() && s1.hasCall {
		r.au.TS1ActivityID, r.au.HashTS1 = s1.activity, s1.hash
	}
	if s2.busy() && s2.hasCall {
		r.au.TS2ActivityID, r.au.HashTS2 = s2.activity, s2.hash
	}
	r.slc = pdu.ShortLC{SLCO: enums.SLCOActivityUpdate, ActivityUpdate: &r.au}
}

func activityOf(c *voice.Call) enums.ActivityID {
	switch {
	case c.Group && c.Options.IsEmergency:
		return enums.ActivityEmergencyGroupVoice
	case c.Group:
		return enums.ActivityGroupVoice
	case c.Options.IsEmergency:
		return enums.ActivityEmergencyIndividualVoice
	}
	return enums.ActivityIndividualVoice
}

func hashOf(id uint32) byte {
	h, _ := pdu.AddressHash(id)
	return h
}

//go:noinline
func appendOutput(out []Output, k OutputKind) ([]Output, *Output) {
	if len(out) < cap(out) {
		out = out[:len(out)+1]
		out[len(out)-1] = Output{}
	} else {
		out = append(out, Output{})
	}
	o := &out[len(out)-1]
	o.Kind = k
	return out, o
}
