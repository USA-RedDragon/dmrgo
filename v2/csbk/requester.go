package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
	"github.com/USA-RedDragon/dmrgo/v2/layer3"
)

// RequesterConfig configures NewRequester. A nil config means the zero
// RequesterConfig.
type RequesterConfig struct {
	// ColorCode is the color code of the slot types of the bursts sent.
	ColorCode enums.ColorCode
	// Source selects the SYNC pattern: layer2.SyncSourceBS, the zero value,
	// for a repeater outbound, layer2.SyncSourceMS for a radio.
	Source layer2.SyncSource
	// Slot is the TDMA slot of the machine, which it puts in the Hint of
	// each burst it sends. It must be Slot1 or Slot2.
	Slot enums.Slot
	// AckWait is T_AckWait, the wait for the answer after each try. 0 means
	// constants.TAckWait (360 ms), and DefaultMotorolaAckWait for the
	// Motorola radio check and call alert. A negative value waits until an
	// answer or Cancel. (ETSI TS 102 361-2 clause A.1)
	AckWait time.Duration
	// Retries is N_CSBKRetry, the number of tries after the first. 0 means
	// constants.NCSBKRetry (1), a negative value means none, and it is at
	// most MaxRetries.
	Retries int
	// IdleSearch is T_IdleSrch, the wait for an idle channel before a
	// polite time critical try. 0 means DefaultIdleSearch, and a negative
	// value does not wait.
	IdleSearch time.Duration
	// HoldoffMax is the upper end of the random holdoff T_Holdoff of a
	// request that is not time critical: a polite try on a busy channel
	// waits a random holdoff, and then another while the channel stays
	// busy. 0 means DefaultHoldoffMax, and a negative value means no
	// holdoff: the try waits for an idle channel. (ETSI TS 102 361-1
	// clause 5.2.2.3)
	HoldoffMax time.Duration
	// Seed seeds the generator of the random holdoff, so that the same
	// inputs give the same outputs. Requesters on one channel need
	// different seeds.
	Seed uint64
	// Lead makes Tick emit a burst this long before its Due time, so that
	// an RF FIFO can be filled ahead. 0 means at Due; it must not be
	// negative. The timers of the machine keep to the Due times.
	Lead time.Duration
	// Scratch, when set, is the Burst that Burst decodes into. Machines
	// that run on one goroutine can share one, because a machine is done
	// with it when its method returns. Make it with layer2.NewBurst. Nil
	// means the Requester makes its own.
	Scratch *layer2.Burst
}

// RequestOptions are the per-request settings. A nil RequestOptions means
// the zero RequestOptions: polite to own color code, not time critical, and
// no Pre_CSBKs.
type RequestOptions struct {
	// Access is the channel access policy of each try. A polite try waits
	// for Channel to report an idle channel.
	Access enums.Access
	// TimeCritical denies the request when the channel stays busy for
	// T_IdleSrch, without a random holdoff (ETSI TS 102 361-2 Figure 5.22).
	TimeCritical bool
	// Preambles is the number of Pre_CSBKs before the first try, for radios
	// that scan or sleep. 0 means none; at most MaxPreambles. An AnyTone
	// radio sends 5 before a radio check.
	Preambles int
	// RetryPreambles is the number of Pre_CSBKs before each retry. 0 means
	// Preambles; at most MaxPreambles. An AnyTone radio sends 15.
	RetryPreambles int
}

// Match describes the answer that ends a request of Requester.Request: a
// CSBK with feature set FID and one of the first N Opcodes, addressed from
// the target of the request to its source. A NACK_Rsp from the target that
// names the opcode of the request also ends it. NewMatch makes a Match
// and sets N.
type Match struct {
	// FID is the feature set ID of the answer.
	FID enums.FID
	// Opcodes holds the opcodes of the answer.
	Opcodes [4]pdu.CSBKOpcode
	// N is the number of Opcodes that are used, from 1 to 4.
	N int
}

// NewMatch returns the Match of an answer with feature set fid and one of
// opcodes, with N set to the number of opcodes. It returns a
// *dmrgo.ConfigError for no opcodes or more than 4.
func NewMatch(fid enums.FID, opcodes ...pdu.CSBKOpcode) (Match, error) {
	m := Match{FID: fid, N: len(opcodes)}
	if m.N < 1 || m.N > len(m.Opcodes) {
		return Match{}, errMatch
	}
	copy(m.Opcodes[:], opcodes)
	return m, nil
}

// RequesterState is the state of a Requester.
type RequesterState uint8

// The RequesterState values.
const (
	// RequesterIdle runs no request.
	RequesterIdle RequesterState = iota
	// RequesterAccess waits for an idle channel or the end of a holdoff.
	RequesterAccess
	// RequesterSending sends the Pre_CSBKs and the request.
	RequesterSending
	// RequesterWaiting waits T_AckWait for the answer.
	RequesterWaiting
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s RequesterState) String() string {
	switch s {
	case RequesterIdle:
		return idleName
	case RequesterAccess:
		return accessName
	case RequesterSending:
		return sendingName
	case RequesterWaiting:
		return "Waiting"
	}
	return "RequesterState(" + text.Int(int64(s)) + ")"
}

// Requester sends CSBK requests on one logical channel and waits for their
// answers, one request at a time, as the package doc describes. Make it
// with NewRequester; a zero Requester does nothing, and its request
// methods return dmrgo.ErrZeroValue. A Requester is not safe for
// concurrent use and must not be copied; go vet reports a copy.
type Requester struct {
	ackWait time.Duration
	retries int
	s       sender
	d       decoder

	state          RequesterState
	service        Service
	match          Match
	req            pdu.CSBKBytes
	opcode         pdu.CSBKOpcode
	target         uint32
	source         uint32
	group          bool
	retryPreambles int
	wait           time.Duration
	tries          int
	waitUntil      time.Duration

	build  pdu.CSBK
	dec    pdu.CSBK
	noCopy noCopy
}

// NewRequester returns a Requester for cfg. A nil cfg means the zero
// RequesterConfig. It returns a *dmrgo.ConfigError for a color code above
// 15, an unknown Source, Retries above MaxRetries, a negative Lead or a
// Slot other than Slot1 or Slot2, so also for a nil cfg. It allocates the
// Requester, and its Burst unless cfg.Scratch is set.
func NewRequester(cfg *RequesterConfig) (*Requester, error) {
	var conf RequesterConfig
	if cfg != nil {
		conf = *cfg
	}
	cfg = &conf
	if cfg.ColorCode > 15 {
		return nil, errColorCode
	}
	if _, _, ok := cfg.Source.SyncPatterns(); !ok {
		return nil, errSource
	}
	if cfg.Retries > MaxRetries {
		return nil, errRetries
	}
	if cfg.Lead < 0 {
		return nil, errLead
	}
	if err := checkSlot(cfg.Slot); err != nil {
		return nil, err
	}
	q := &Requester{ackWait: cfg.AckWait, retries: cfg.Retries}
	switch {
	case q.retries == 0:
		q.retries = constants.NCSBKRetry
	case q.retries < 0:
		q.retries = 0
	}
	q.s.init(cfg.ColorCode, cfg.Source, cfg.IdleSearch, cfg.HoldoffMax, cfg.Seed)
	q.s.lead, q.s.slot = cfg.Lead, cfg.Slot
	q.d = newDecoder(cfg.Scratch)
	return q, nil
}

// Reset ends any request without an output and forgets the channel state.
// It keeps the config and the state of the random generator.
func (q *Requester) Reset() {
	if q == nil {
		return
	}
	q.state = RequesterIdle
	q.s.stop()
	q.s.busy = false
}

// State returns the state of the Requester.
func (q *Requester) State() RequesterState {
	if q == nil {
		return 0
	}
	return q.state
}

// RequestOACSU starts the presence check of an OACSU individual call from
// source to target: UU_V_Req with the service options so, answered by
// UU_Ans_Rsp. The request is time critical whatever opts says, and its
// access must be polite. The OutputDone has ResultAnswered when the call may
// proceed, with impolite access. It returns ErrBusy while a request runs,
// dmrgo.ErrZeroValue on a zero Requester,
// and a *dmrgo.ConfigError for an address above 24 bits, impolite access or
// a Pre_CSBK count out of range. (ETSI TS 102 361-2 clause 5.2.2.3)
func (q *Requester) RequestOACSU(now time.Duration, target, source uint32, so layer3.ServiceOptions, opts *RequestOptions) error {
	if q == nil {
		return dmrgo.ErrNilReceiver
	}
	if q.d.b == nil {
		return dmrgo.ErrZeroValue
	}
	var o RequestOptions
	if opts != nil {
		o = *opts
	}
	if o.Access == enums.AccessImpolite {
		return errAccess
	}
	o.TimeCritical = true
	if err := q.check(target, source, &o); err != nil {
		return err
	}
	q.build = pdu.CSBK{LastBlock: true}
	if err := q.build.SetUnitToUnitVoiceServiceRequest(pdu.UnitToUnitVoiceServiceRequestPDU{ServiceOptions: so, TargetAddress: target, SourceAddress: source}); err != nil {
		return err
	}
	octets, err := q.build.Encode()
	if err != nil {
		return err
	}
	q.begin(now, pdu.CSBKBytes(octets), target, source, false, ServiceOACSU, &o)
	return nil
}

// RequestRadioCheck starts a Motorola radio check of target by source (FID
// 0x10, opcode 0x24). The OutputDone has ResultAnswered when target
// answers. It returns errors as RequestOACSU does.
func (q *Requester) RequestRadioCheck(now time.Duration, target, source uint32, opts *RequestOptions) error {
	if q == nil {
		return dmrgo.ErrNilReceiver
	}
	if q.d.b == nil {
		return dmrgo.ErrZeroValue
	}
	var o RequestOptions
	if opts != nil {
		o = *opts
	}
	if err := q.check(target, source, &o); err != nil {
		return err
	}
	q.build = pdu.CSBK{LastBlock: true}
	if err := q.build.SetMotorolaRadioCheck(pdu.MotorolaRadioCheckPDU{RequesterAddress: source, CheckedAddress: target}); err != nil {
		return err
	}
	octets, err := q.build.Encode()
	if err != nil {
		return err
	}
	q.begin(now, pdu.CSBKBytes(octets), target, source, false, ServiceRadioCheck, &o)
	return nil
}

// RequestCallAlert starts a Motorola call alert of target by source (FID
// 0x10, opcode 0x1F), which makes target alert its user. The OutputDone
// has ResultAnswered when target acknowledges it. It returns errors as
// RequestOACSU does.
func (q *Requester) RequestCallAlert(now time.Duration, target, source uint32, opts *RequestOptions) error {
	if q == nil {
		return dmrgo.ErrNilReceiver
	}
	if q.d.b == nil {
		return dmrgo.ErrZeroValue
	}
	var o RequestOptions
	if opts != nil {
		o = *opts
	}
	if err := q.check(target, source, &o); err != nil {
		return err
	}
	q.build = pdu.CSBK{LastBlock: true}
	if err := q.build.SetMotorolaCallAlert(pdu.MotorolaCallAlertPDU{TargetAddress: target, SourceAddress: source}); err != nil {
		return err
	}
	octets, err := q.build.Encode()
	if err != nil {
		return err
	}
	q.begin(now, pdu.CSBKBytes(octets), target, source, false, ServiceCallAlert, &o)
	return nil
}

// Request starts a request with the CSBK c, which m answers. Request sends
// c with the Last Block bit set and its CRC; c itself is not changed. The
// target and source come from c: from CSBKBytes.Addresses for the standard
// FID, from the payload for the Motorola CSBKs, and from octets 4 to 6 and
// 7 to 9 for any other FID. It returns ErrBusy while a request runs,
// dmrgo.ErrZeroValue on a zero Requester, dmrgo.ErrNilArgument for a nil c,
// the error of CSBK.Encode for a field out of range, ErrNoAddresses for a
// standard CSBK without both addresses, and a *dmrgo.ConfigError for a Match
// without opcodes or a Pre_CSBK count out of range.
func (q *Requester) Request(now time.Duration, c *pdu.CSBK, m Match, opts *RequestOptions) error {
	if q == nil {
		return dmrgo.ErrNilReceiver
	}
	if q.d.b == nil {
		return dmrgo.ErrZeroValue
	}
	if c == nil {
		return dmrgo.ErrNilArgument
	}
	var o RequestOptions
	if opts != nil {
		o = *opts
	}
	if m.N < 1 || m.N > len(m.Opcodes) {
		return errMatch
	}
	q.build = *c
	q.build.LastBlock = true
	octets, err := q.build.Encode()
	if err != nil {
		return err
	}
	b := pdu.CSBKBytes(octets)
	q.dec = pdu.DecodeCSBK(b)
	target, source, ok := addresses(&q.dec)
	if !ok {
		return ErrNoAddresses
	}
	if err := q.check(target, source, &o); err != nil {
		return err
	}
	group, _ := b.Group()
	q.match = m
	q.begin(now, b, target, source, group, ServiceCustom, &o)
	return nil
}

func (q *Requester) check(target, source uint32, o *RequestOptions) error {
	if q.state != RequesterIdle {
		return ErrBusy
	}
	if err := checkAddress(target, errTarget); err != nil {
		return err
	}
	if err := checkAddress(source, errSourceAddr); err != nil {
		return err
	}
	if o.Preambles < 0 || o.Preambles > MaxPreambles || o.RetryPreambles < 0 || o.RetryPreambles > MaxPreambles {
		return errPreambles
	}
	if o.Access > enums.AccessImpolite {
		return errAccess
	}
	if o.RetryPreambles == 0 {
		o.RetryPreambles = o.Preambles
	}
	return nil
}

func (q *Requester) begin(now time.Duration, c pdu.CSBKBytes, target, source uint32, group bool, svc Service, o *RequestOptions) {
	q.req, q.opcode, q.target, q.source, q.group, q.service = c, c.Opcode(), target, source, group, svc
	q.retryPreambles, q.tries = o.RetryPreambles, 0
	q.wait = q.ackWait
	if q.wait == 0 {
		q.wait = constants.TAckWait
		if svc == ServiceRadioCheck || svc == ServiceCallAlert {
			q.wait = DefaultMotorolaAckWait
		}
	}
	q.s.access, q.s.timeCritical = o.Access, o.TimeCritical
	q.s.start(now, c, target, source, group, o.Preambles)
	q.sync()
}

func (q *Requester) sync() {
	switch q.s.state {
	case sendQualify, sendHoldoff, sendWait:
		q.state = RequesterAccess
	case sendBursts:
		q.state = RequesterSending
	case sendIdle:
	}
}

// Channel reports whether the logical channel is busy, from the activity
// that the caller sees on it. A polite try waits for an idle channel. Until
// the first report the channel counts as idle.
func (q *Requester) Channel(now time.Duration, busy bool) {
	if q == nil {
		return
	}
	q.s.channel(now, busy)
	if q.state == RequesterAccess {
		q.sync()
	}
}

// Cancel ends the request, and appends an OutputDone with ResultCanceled
// to dst while one runs. It does not run the timers, so now only gives the
// shape of the other methods. It allocates only when dst has no room for
// MaxOutputs outputs.
func (q *Requester) Cancel(_ time.Duration, dst []Output) []Output {
	if q == nil {
		return dst
	}

	if q.state == RequesterIdle {
		return dst
	}
	return q.done(dst, ResultCanceled, nil)
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Requester as Decoded does, and appends the outputs to dst. A
// nil raw, a transport frame without a burst, is a nil b for Decoded. It
// allocates only when dst has no room for MaxOutputs outputs.
func (q *Requester) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if q == nil {
		return dst
	}
	return q.Decoded(now, q.d.decode(raw, &h), h, dst)
}

// Decoded takes the decoded burst b, which may be nil, and appends the
// outputs to dst: an OutputDone when b is the answer to the request, or a
// NACK_Rsp for it, after the first try went out. A CSBK whose CRC fails is
// dropped. It allocates only when dst has no room for MaxOutputs outputs.
func (q *Requester) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if q == nil {
		return dst
	}
	dst = q.Tick(now, dst)
	if q.state == RequesterIdle || q.tries == 0 {
		return dst
	}
	kind, c, octets := classify(b)
	if kind != rxCSBK {
		return dst
	}
	target, source, ok := addresses(c)
	if !ok || target != q.source || source != q.target {
		return dst
	}
	if n, ok := c.AsNegativeAcknowledgement(); ok {
		if n.ServiceType == q.opcode {
			dst = q.done(dst, ResultNACK, &octets)
			dst[len(dst)-1].Reason = Reason(n.ReasonCode)
		}
		return dst
	}
	if r, ok := q.answer(c); ok {
		return q.done(dst, r, &octets)
	}
	return dst
}

func (q *Requester) answer(c *pdu.CSBK) (Result, bool) {
	switch q.service {
	case ServiceOACSU:
		if a, ok := c.AsUnitToUnitVoiceServiceAnswerResponse(); ok {
			switch a.AnswerResponse {
			case answerProceed:
				return ResultAnswered, true
			case answerDeny:
				return ResultRefused, true
			}
		}
	case ServiceRadioCheck:
		if p, ok := c.AsMotorolaRadioCheck(); ok && p.Answer {
			return ResultAnswered, true
		}
	case ServiceCallAlert:
		if _, ok := c.AsMotorolaCallAlertAck(); ok {
			return ResultAnswered, true
		}
	case ServiceCustom:
		if c.FID != q.match.FID {
			return 0, false
		}
		for _, op := range q.match.Opcodes[:q.match.N] {
			if c.Opcode == op {
				return ResultAnswered, true
			}
		}
	case ServiceUnsupported, ServiceWakeup, ServiceTiming:
	}
	return 0, false
}

// Lost tells the Requester that the transport lost the signal: the channel
// counts as idle. It runs the timers as Tick does and appends the outputs
// to dst. It allocates only when dst has no room for MaxOutputs outputs.
func (q *Requester) Lost(now time.Duration, dst []Output) []Output {
	if q == nil {
		return dst
	}
	q.Channel(now, false)
	return q.Tick(now, dst)
}

// Tick runs the timers that are due at now and appends their outputs to
// dst: at most one burst of a try, and the OutputDone of a request that the
// channel denied or that got no answer after the last retry. A Tick before
// Deadline does nothing. It allocates only when dst has no room for
// MaxOutputs outputs.
func (q *Requester) Tick(now time.Duration, dst []Output) []Output {
	if q == nil {
		return dst
	}
	if q.state == RequesterWaiting && q.wait >= 0 && now >= q.waitUntil {
		if q.tries > q.retries {
			return q.done(dst, ResultNoAnswer, nil)
		}
		q.s.start(q.waitUntil, q.req, q.target, q.source, q.group, q.retryPreambles)
		q.s.due = max(q.s.due, now+q.s.lead)
		q.sync()
	}
	if q.state != RequesterAccess && q.state != RequesterSending {
		return dst
	}
	dst, ev := q.s.tick(now, dst)
	switch ev {
	case sendSent:
		q.tries++
		q.state = RequesterWaiting
		q.waitUntil = dst[len(dst)-1].Due + q.wait
	case sendDenied:
		return q.done(dst, ResultDenied, nil)
	case sendNothing:
		q.sync()
	}
	return dst
}

// Deadline returns the time at which the Requester needs a Tick. ok is
// false when no timer runs.
func (q *Requester) Deadline() (at time.Duration, ok bool) {
	if q == nil {
		return 0, false
	}
	switch q.state {
	case RequesterWaiting:
		return q.waitUntil, q.wait >= 0
	case RequesterAccess, RequesterSending:
		return q.s.deadline()
	case RequesterIdle:
	}
	return 0, false
}

func (q *Requester) done(out []Output, r Result, answer *pdu.CSBKBytes) []Output {
	q.state = RequesterIdle
	q.s.stop()
	var o *Output
	out, o = appendOutput(out, OutputDone)
	o.Service, o.Result, o.Target, o.Source = q.service, r, q.target, q.source
	if answer != nil {
		o.CSBK = *answer
	}
	return out
}
