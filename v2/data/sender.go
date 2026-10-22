package data

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/ip"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxOutputs is the most outputs that one call of a Sender method appends:
// one burst and the end of each message of a window.
const MaxOutputs = MaxWindow + 1

// Defaults and limits of SenderConfig.
const (
	// MaxWindow is the most C_HEAD packets of one sliding window: 7 that do
	// not ask for a response and the 8th that does (ETSI TS 102 361-3
	// clause 5.4.4).
	MaxWindow = 8
	// DefaultIdleSearch is the time that a polite Sender waits for an idle
	// channel before it holds off, the largest T_IdleSrch (ETSI TS 102
	// 361-1 clause F.1).
	DefaultIdleSearch = 540 * time.Millisecond
	// MaxPreambles is the most preamble CSBKs before a header.
	MaxPreambles = 64
	// MaxRetries is the most transmissions of one message.
	MaxRetries = 64
)

// SenderConfig configures NewSender. A nil config means the zero
// SenderConfig.
type SenderConfig struct {
	// ColorCode is the color code of the bursts.
	ColorCode enums.ColorCode
	// Source selects the SYNC patterns: layer2.SyncSourceBS, the zero
	// value, for a repeater outbound, layer2.SyncSourceMS for a radio.
	Source layer2.SyncSource
	// Access is the channel access policy. The zero value,
	// enums.AccessPoliteOwnCC, sends only while Channel last reported the
	// channel idle (ETSI TS 102 361-1 clause 5.2.1.6).
	Access enums.Access
	// Preambles is the number of preamble CSBKs before the header of each
	// try, so that a radio in power save wakes up. 0 sends none; at most
	// MaxPreambles. Radios send 5.
	Preambles int
	// IdleSearch is the time that a polite Sender waits for an idle
	// channel before it holds off. 0 means DefaultIdleSearch, and a
	// negative value holds off at once.
	IdleSearch time.Duration
	// Holdoff is the longest random holdoff T_Holdoff after a busy
	// channel. 0 means constants.THoldoffMax (2 s), and a negative value
	// turns the holdoff off.
	Holdoff time.Duration
	// ResponseWait is the time T_RspnsWait from the end of the last burst
	// to the response of a confirmed packet. 0 means
	// constants.TResponseWait (180 ms). A response that comes through a
	// network can need more; constants.TResponseWaitSimulcast is 2 s.
	ResponseWait time.Duration
	// TxLimit is the time T_DataTxLmt from Send after which no new try
	// starts. 0 means constants.TDataTxLimit (60 s), and a negative value
	// turns the limit off.
	TxLimit time.Duration
	// Retries is the most transmissions N_RtryLmt of a confirmed message,
	// the first one included. 0 means constants.NRetryLimit (8); at most
	// MaxRetries.
	Retries int
	// MaxBlocks is the most data blocks of a packet. 0 means MaxBlocks.
	// Each block takes about 110 bytes, so a microcontroller can set 16.
	MaxBlocks int
	// Peers is the number of links whose send sequence number V(S) the
	// Sender keeps. 0 means DefaultPeers; at most MaxPeers.
	Peers int
	// Resync sets the S flag on the first packet to a link and on the
	// resend after a NACK for N(S) out of sequence, so that the receiver
	// takes the N(S). ETSI TS 102 361-1 clause 9.3.23 keeps S for special
	// registration messages, so false, the default, never sets it.
	Resync bool
	// Window is the most C_HEAD messages of the IP bearer service in
	// progress at once, for sliding window flow control. Send queues up to
	// Window of them to one link, and the Sender sends them one after the
	// other with the Response Requested flag set only on the last, then
	// takes one response for all of them. 0 or 1, the default, means stop
	// and wait: each message asks for its own response. At most MaxWindow.
	// Each message of the window takes its own packet.Builder. Use a
	// window only with receivers known to keep every packet of it: the
	// AnyTone D890UV firmware (also in the BTECH DA-7X2) sends an ACK for
	// the last packet of a window and keeps only that packet, so the
	// earlier ones are lost. (ETSI TS 102 361-3 clause 5.4.4)
	Window int
	// Seed seeds the random holdoff. Two Senders with the same seed hold
	// off for the same times, so give each radio its own seed.
	Seed uint64
}

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputBurst is a burst to send.
	OutputBurst OutputKind = iota + 1
	// OutputDone is the end of the message.
	OutputDone
)

// String returns the name of the kind. It allocates only for a value
// without a name.
func (k OutputKind) String() string {
	switch k {
	case OutputBurst:
		return "Burst"
	case OutputDone:
		return "Done"
	}
	return unnamed("OutputKind", uint8(k))
}

// Result is how a message ended.
type Result uint8

// The Result values.
const (
	// ResultSent is an unconfirmed message whose last burst was sent.
	ResultSent Result = iota + 1
	// ResultDelivered is a confirmed message that got an ACK.
	ResultDelivered
	// ResultRefused is a confirmed message that got a NACK that a resend
	// cannot fix: illegal format, memory full, undeliverable, invalid
	// user, or FSN out of sequence.
	ResultRefused
	// ResultRetryLimit is a confirmed message that got no ACK in
	// SenderConfig.Retries transmissions.
	ResultRetryLimit
	// ResultTimeLimit is a message that could not finish before
	// SenderConfig.TxLimit.
	ResultTimeLimit
	// ResultCancelled is a message that Cancel ended.
	ResultCancelled
)

// String returns the name of the result. It allocates only for a value
// without a name.
func (r Result) String() string {
	switch r {
	case ResultSent:
		return "Sent"
	case ResultDelivered:
		return "Delivered"
	case ResultRefused:
		return "Refused"
	case ResultRetryLimit:
		return "RetryLimit"
	case ResultTimeLimit:
		return "TimeLimit"
	case ResultCancelled:
		return "Cancelled"
	}
	return unnamed("Result", uint8(r))
}

// Output is one result of a Sender. Fields that do not apply to Kind are
// zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Burst is the burst to send.
	Burst layer2.RawBurst
	// Hint is what the burst is, for a transport: KindData with the data
	// type of the burst. Its Slot is SlotUnknown.
	Hint layer2.Hint
	// Due is the time at which the burst goes on air, 60 ms after the one
	// before it.
	Due time.Duration
	// Try is the transmission that the burst belongs to, or that ended the
	// message: 1 for the first.
	Try uint8
	// Seq is the N(S) of the C_HEAD message that the burst belongs to or
	// that ended. Messages to one link take N(S) 0, 1, 2 and so on modulo 8
	// in the order of Send, so with SenderConfig.Window above 1 Seq tells
	// which message an OutputDone ends. It is 0 for other messages.
	Seq uint8
	// Result is how the message ended.
	Result Result
	// Response is the last response to the message. HasResponse reports
	// whether one came.
	Response packet.Response
	// HasResponse reports whether Response is set.
	HasResponse bool
	// Group reports whether the message that ended went to a talkgroup.
	Group bool
}

// SenderState is the state of a Sender.
type SenderState uint8

// The SenderState values.
const (
	// SenderIdle has no message.
	SenderIdle SenderState = iota
	// SenderWaiting waits for an idle channel.
	SenderWaiting
	// SenderHoldoff waits for the random holdoff after a busy channel.
	SenderHoldoff
	// SenderSending sends the bursts of a try, one per Tick.
	SenderSending
	// SenderWaitResponse waits for the response to a confirmed packet.
	SenderWaitResponse
)

// String returns the name of the state. It allocates only for a value
// without a name.
func (s SenderState) String() string {
	switch s {
	case SenderIdle:
		return idleName
	case SenderWaiting:
		return "Waiting"
	case SenderHoldoff:
		return "Holdoff"
	case SenderSending:
		return "Sending"
	case SenderWaitResponse:
		return "WaitResponse"
	}
	return unnamed("SenderState", uint8(s))
}

type slot struct {
	pb        *packet.Builder
	pkt       packet.Packet
	try       packet.Packet
	confirmed bool
	cHead     bool
	windowed  bool
	ns        uint8
	tries     int
	limitAt   time.Duration
	last      packet.Response
	hasLast   bool
}

// Sender sends data messages on one logical channel: the preamble CSBKs,
// the header and the blocks of each try, one burst per Tick, 60 ms apart,
// after the channel access of SenderConfig.Access. For a confirmed message
// it waits for the response and resends: the blocks that a SACK asks for,
// the whole packet after a NACK for a bad CRC or no response, and the
// whole packet again as a first try after a NACK for N(S) out of sequence,
// which some radios send for a lost block. It sends one message at a time,
// or with SenderConfig.Window a sliding window of C_HEAD messages of the
// IP bearer service to one link. Make it with NewSender. A Sender is not
// safe for concurrent use and must not be copied; go vet reports a copy.
type Sender struct {
	cc           enums.ColorCode
	source       layer2.SyncSource
	access       enums.Access
	preambles    int
	idleSearch   time.Duration
	holdoff      time.Duration
	responseWait time.Duration
	txLimit      time.Duration
	retries      int
	resync       bool
	rng          pcg.Source
	peers        peers

	b    *layer2.Burst
	opts layer2.DecodeOptions
	asm  *packet.Assembler

	slots  []slot
	ord    [MaxWindow]uint8
	n      int
	win    int
	cur    int
	closed bool

	state     SenderState
	busy      bool
	call      Call
	link      *peer
	idx       int
	total     int
	due       time.Duration
	waitUntil time.Duration
	holdUntil time.Duration
	respUntil time.Duration
	limitAt   time.Duration

	csbk   pdu.CSBK
	noCopy noCopy
}

// NewSender returns a Sender for cfg. A nil cfg means the zero
// SenderConfig. It returns a *dmrgo.ConfigError for a color code above 15, an
// unknown Source or Access, or a Preambles, Retries, MaxBlocks, Peers or
// Window out of range. It allocates the Sender, a packet.Builder for each
// message of the window, its packet.Assembler, its table of links and its
// Burst.
func NewSender(cfg *SenderConfig) (*Sender, error) {
	var conf SenderConfig
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
	if cfg.Access > enums.AccessImpolite {
		return nil, errAccess
	}
	if cfg.Preambles < 0 || cfg.Preambles > MaxPreambles {
		return nil, errPreambles
	}
	retries, err := limit(cfg.Retries, constants.NRetryLimit, MaxRetries, errRetries)
	if err != nil {
		return nil, err
	}
	blocks, err := limit(cfg.MaxBlocks, MaxBlocks, MaxBlocks, errMaxBlocks)
	if err != nil {
		return nil, err
	}
	n, err := limit(cfg.Peers, DefaultPeers, MaxPeers, errPeers)
	if err != nil {
		return nil, err
	}
	window, err := limit(cfg.Window, 1, MaxWindow, errWindow)
	if err != nil {
		return nil, err
	}
	s := &Sender{
		cc:           cfg.ColorCode,
		source:       cfg.Source,
		access:       cfg.Access,
		preambles:    cfg.Preambles,
		idleSearch:   max(defaultDuration(cfg.IdleSearch, DefaultIdleSearch), 0),
		holdoff:      max(defaultDuration(cfg.Holdoff, constants.THoldoffMax), 0),
		responseWait: max(defaultDuration(cfg.ResponseWait, constants.TResponseWait), 0),
		txLimit:      defaultDuration(cfg.TxLimit, constants.TDataTxLimit),
		retries:      retries,
		resync:       cfg.Resync,
		rng:          pcg.New(cfg.Seed),
		peers:        peers{list: make([]peer, n)},
		b:            layer2.NewBurst(),
		asm:          packet.NewAssembler(2),
		slots:        make([]slot, window),
	}
	for i := range s.slots {
		s.slots[i].pb = packet.NewBuilder(blocks)
	}
	return s, nil
}

// Reset ends every message without outputs and forgets the sequence
// numbers of every link and the channel state. It keeps the config and the
// state of the random holdoff.
func (s *Sender) Reset() {
	if s == nil {
		return
	}

	s.state, s.busy = SenderIdle, false
	s.n, s.win, s.cur, s.closed = 0, 0, 0, false
	s.link = nil
	s.peers.reset()
	if s.asm != nil {
		s.asm.Reset()
	}
}

// State returns the state of the Sender.
func (s *Sender) State() SenderState {
	if s == nil {
		return 0
	}
	return s.state
}

// Send starts the message m at now, or queues it in the sliding window.
// m.Header is a U_HEAD, a C_HEAD, or a DD_HEAD or R_HEAD that a SetX method
// of pdu.DataHeader set, and m.Rate is the rate of the blocks, where the zero
// value means rate 1/2. Send copies the header and m.Payload, so the
// caller can reuse them. It sets the block count, pad octets and Full
// Message Flag, and for a C_HEAD N(S) from the link, S (with
// SenderConfig.Resync), FSN 1000, a message in one fragment, and the
// Response Requested flag on the last message of a window only. It sets
// the Response Requested flag of a short data header with SARQ set. With
// SenderConfig.Window above 1, a C_HEAD message of the IP bearer service
// to the link of the messages in progress joins them while fewer than
// Window are in progress: it goes in the window on air when the last
// header of that window has not gone yet, else in the next window. Send
// returns ErrBusy for any other message while a message is in progress,
// dmrgo.ErrNilArgument for a nil m, a *dmrgo.ConfigError for another
// header, a P_HEAD in m.Proprietary, or a confirmed message to a group, an error on a zero
// Sender, and the errors of packet.Builder.Segment, such as
// packet.ErrNoRoom for a message of more than SenderConfig.MaxBlocks blocks
// and packet.ErrRate. (ETSI TS 102 361-3 clauses 5.4 and
// 5.4.4)
func (s *Sender) Send(now time.Duration, m *Message) error {
	if s == nil {
		return dmrgo.ErrNilReceiver
	}
	switch {
	case s.asm == nil:
		return errZero
	case m == nil:
		return dmrgo.ErrNilArgument
	case s.n >= len(s.slots) || s.n > 0 && !s.joins(m):
		return ErrBusy
	}
	if m.Proprietary != nil {
		return errHeader
	}
	at := s.free()
	e := &s.slots[at]
	h := m.Header
	c, err := sendHeader(&h, e)
	if err != nil {
		return err
	}
	if e.confirmed && groupOf(&h) {
		return errMessageGroup
	}
	e.cHead = h.Format() == pdu.FormatConfirmed
	e.windowed = e.cHead && ipBearer(c.SAP)
	if s.n == 0 {
		s.call, s.link = callOf(&h), nil
		if e.cHead {
			link, found := s.peers.lookup(s.call.Source, s.call.Target)
			c.ReSynchronizeFlag = s.resync && !found
			s.link = link
		}
	} else {
		c.ReSynchronizeFlag = false
	}
	if e.cHead {
		if err := h.SetConfirmed(c); err != nil {
			return err
		}
	}
	rate := m.Rate
	if rate == enums.DataTypePIHeader {
		rate = enums.DataTypeRate12
	}
	p, err := e.pb.Segment(h, m.Payload, rate)
	if err != nil {
		return err
	}
	e.pkt, e.try, e.tries, e.hasLast = p, p, 0, false
	e.limitAt = now + s.txLimit
	s.ord[s.n] = uint8(at & 0xFF)
	s.n++
	switch {
	case s.n == 1:
		s.asm.Reset()
		s.start(now)
	case !s.closed && s.state != SenderWaitResponse:
		s.join(s.n - 1)
	}
	return nil
}

func sendHeader(h *pdu.DataHeader, e *slot) (pdu.ConfirmedDataHeader, error) {
	if u, ok := h.AsUnconfirmed(); ok {
		u.FullMessageFlag = true
		e.confirmed = false
		return pdu.ConfirmedDataHeader{}, h.SetUnconfirmed(u)
	}
	if c, ok := h.AsConfirmed(); ok {
		if c.Group {
			return c, errMessageGroup
		}
		c.FullMessageFlag, c.ResponseRequested, c.FragmentSequenceNumber = true, true, 0b1000
		e.confirmed = true
		return c, nil
	}
	if d, ok := h.AsDefinedData(); ok {
		d.FullMessageFlag = true
		d.ResponseRequested = d.ResponseRequested || d.SARQ
		e.confirmed = d.SARQ
		return pdu.ConfirmedDataHeader{}, h.SetDefinedData(d)
	}
	if r, ok := h.AsRawData(); ok {
		r.FullMessageFlag = true
		r.ResponseRequested = r.ResponseRequested || r.SARQ
		e.confirmed = r.SARQ
		return pdu.ConfirmedDataHeader{}, h.SetRawData(r)
	}
	return pdu.ConfirmedDataHeader{}, errHeader
}

func (s *Sender) joins(m *Message) bool {
	c, ok := m.Header.AsConfirmed()
	if !s.slots[s.ord[0]].windowed || !ok {
		return false
	}
	return ipBearer(c.SAP) && !c.Group && c.LLIDDestination == s.call.Target && c.LLIDSource == s.call.Source
}

func (s *Sender) free() int {
	for i := range s.slots {
		used := false
		for k := range s.n {
			used = used || int(s.ord[k]) == i
		}
		if !used {
			return i
		}
	}
	return 0
}

func (s *Sender) join(k int) {
	e := &s.slots[s.ord[k]]
	if c, ok := e.pkt.Header.AsConfirmed(); e.tries == 0 && e.cHead && ok && s.link != nil {
		e.ns, c.SendSequenceNumber = s.link.seq, s.link.seq
		if e.pkt.Header.SetConfirmed(c) == nil {
			e.try.Header = e.pkt.Header
		}
		s.link.seq = (s.link.seq + 1) & 7
	}
	e.tries++
	s.win = k + 1
}

func (s *Sender) start(now time.Duration) {
	s.win, s.cur, s.idx, s.closed = 0, 0, 0, false
	s.limitAt = s.slots[s.ord[0]].limitAt
	for k := range s.n {
		s.join(k)
		s.limitAt = min(s.limitAt, s.slots[s.ord[k]].limitAt)
	}
	s.load()
	switch {
	case s.access == enums.AccessImpolite || !s.busy:
		s.state, s.due = SenderSending, now
	default:
		s.state, s.waitUntil = SenderWaiting, now+s.idleSearch
	}
}

func (s *Sender) load() {
	e := &s.slots[s.ord[s.cur]]
	s.total = 1 + len(e.try.Blocks)
	if s.cur == 0 {
		s.total += s.preambles
	}
}

// Channel tells the Sender whether the channel is busy at now, for example
// from the AT bit of the CACH, the carrier, or a repeater. A polite Sender
// that waits for the channel starts its try when it becomes idle. An
// impolite Sender ignores it.
func (s *Sender) Channel(now time.Duration, busy bool) {
	if s == nil {
		return
	}
	s.busy = busy
	if !busy && s.state == SenderWaiting {
		s.state, s.due = SenderSending, now
	}
}

// Cancel ends every message in progress with ResultCancelled and appends
// the outputs to dst. It does nothing without a message, and allocates
// only when dst has no room for MaxOutputs outputs.
func (s *Sender) Cancel(_ time.Duration, dst []Output) []Output {
	if s == nil {
		return dst
	}

	for s.n > 0 {
		dst = s.finish(0, ResultCancelled, dst)
	}
	return dst
}

// Burst decodes the burst raw and takes the response to the confirmed
// messages in progress from it: a response packet from the target of the
// messages to their source. It appends the end of each message that the
// response ends. Other bursts are ignored; the channel state comes from
// Channel. A nil raw, a transport frame without a burst, only runs the
// timers. It allocates only when dst has no room for MaxOutputs outputs.
func (s *Sender) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	if s == nil {
		return dst
	}
	s.opts.Kind = h.Kind
	if raw == nil || s.b == nil || s.b.DecodeFromBytesWith(*raw, &s.opts) != nil {
		return s.timers(now, dst)
	}
	return s.Decoded(now, s.b, h, dst)
}

// Decoded takes the response to the confirmed messages in progress from
// the decoded burst b, as Burst does. A nil b only runs the timers. For a
// sliding window, the Status of the response is the N(S) of a message of
// the window: an ACK delivers the messages up to that one, a SACK delivers
// the messages before it and resends its blocks that the SACK flags, and a
// NACK delivers the messages before it and resends it or ends it as
// refused. The messages after it are sent again, and a response with
// another Status is ignored. It allocates only when dst has no room for
// MaxOutputs outputs. (ETSI TS 102 361-3 clause 5.4.4)
func (s *Sender) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	if s == nil {
		return dst
	}
	dst = s.timers(now, dst)
	if b == nil || s.asm == nil || b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable || s.state != SenderWaitResponse {
		return dst
	}
	switch b.SlotType.DataType {
	case enums.DataTypeDataHeader, enums.DataTypeRate12:
	case enums.DataTypePIHeader, enums.DataTypeVoiceLCHeader, enums.DataTypeTerminatorWithLC,
		enums.DataTypeCSBK, enums.DataTypeMBCHeader, enums.DataTypeMBCContinuation,
		enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeIdle,
		enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		return dst
	}
	m, done, err := s.asm.AddBurst(b)
	if !done {
		return dst
	}
	rh, ok := m.Header.AsResponse()
	if !ok || rh.LLIDDestination != s.call.Source || rh.LLIDSource != s.call.Target {
		return dst
	}
	code, cerr := packet.ResponseOf(&rh)
	if cerr != nil {
		return dst
	}
	j := 0
	if s.win > 1 {
		j = -1
		for k := range s.win {
			if s.slots[s.ord[k]].ns == rh.ResponseStatus {
				j = k
			}
		}
		if j < 0 && code != packet.ResponseNACKOutOfSequence {
			return dst
		}
	}
	rd := m.Response
	if err != nil {
		rd = nil
	}
	return s.answered(now, code, j, rd, dst)
}

func (s *Sender) answered(now time.Duration, code packet.Response, j int, rd *packet.ResponseData, dst []Output) []Output {
	w, k := s.win, 0
	for i := range w {
		e := &s.slots[s.ord[k]]
		e.last, e.hasLast = code, true
		switch {
		case code == packet.ResponseNACKOutOfSequence:
			if c, ok := e.pkt.Header.AsConfirmed(); s.resync && e.cHead && ok {
				c.ReSynchronizeFlag = true
				_ = e.pkt.Header.SetConfirmed(c)
			}
			e.try = e.pkt
			k++
		case i < j || i == j && code == packet.ResponseACK:
			dst = s.finish(k, ResultDelivered, dst)
		case i > j || code == packet.ResponseNACKPacketCRC:
			s.again(e, nil)
			k++
		case code == packet.ResponseSACK:
			s.again(e, rd)
			k++
		default:
			dst = s.finish(k, ResultRefused, dst)
		}
	}
	return s.next(now, dst)
}

func (s *Sender) again(e *slot, rd *packet.ResponseData) {
	if rd != nil {
		if p, err := e.pb.Retry(&e.pkt, rd); err == nil {
			e.try = p
			return
		}
	}
	p, err := e.pb.Retry(&e.pkt, nil)
	if err != nil {
		e.try = e.pkt
		return
	}
	e.try = p
}

func (s *Sender) next(now time.Duration, dst []Output) []Output {
	for k := 0; k < s.n; {
		e := &s.slots[s.ord[k]]
		switch {
		case e.tries >= s.retries:
			dst = s.finish(k, ResultRetryLimit, dst)
		case s.txLimit >= 0 && now >= e.limitAt:
			dst = s.finish(k, ResultTimeLimit, dst)
		default:
			k++
		}
	}
	if s.n > 0 {
		s.start(now)
	}
	return dst
}

func (s *Sender) finish(k int, result Result, dst []Output) []Output {
	e := &s.slots[s.ord[k]]
	var o *Output
	dst, o = appendOutput(dst, OutputDone, 0)
	o.Try, o.Result, o.Response, o.HasResponse = uint8(min(e.tries, MaxRetries)&0xFF), result, e.last, e.hasLast
	if e.cHead {
		o.Seq = e.ns
	}
	o.Group = groupOf(&e.pkt.Header)
	for i := k + 1; i < s.n; i++ {
		s.ord[i-1] = s.ord[i]
	}
	s.n--
	if k < s.win {
		s.win--
	}
	if s.n == 0 {
		s.state, s.link, s.win = SenderIdle, nil, 0
	}
	return dst
}

// Deadline returns the time at which the Sender needs a Tick. ok is false
// when no timer runs.
func (s *Sender) Deadline() (at time.Duration, ok bool) {
	if s == nil {
		return 0, false
	}
	switch s.state {
	case SenderWaiting:
		at = s.waitUntil
	case SenderHoldoff:
		at = s.holdUntil
	case SenderSending:
		return s.due, true
	case SenderWaitResponse:
		return s.respUntil, true
	case SenderIdle:
		return 0, false
	}
	if s.txLimit >= 0 && s.limitAt < at {
		at = s.limitAt
	}
	return at, true
}

// Tick appends the next burst when now has reached its Due time, and runs
// the other timers that are due: the end of the idle search, the holdoff,
// the response wait, which starts a resend, and the transmit limit. A Tick
// before Deadline does nothing. Each Tick appends at most one burst; a
// late caller calls Tick until Deadline is in the future. It allocates
// only when dst has no room for MaxOutputs outputs.
func (s *Sender) Tick(now time.Duration, dst []Output) []Output {
	if s == nil {
		return dst
	}
	return s.timers(now, dst)
}

func (s *Sender) timers(now time.Duration, dst []Output) []Output {
	for {
		at, ok := s.Deadline()
		if !ok || now < at {
			return dst
		}
		switch s.state {
		case SenderWaiting, SenderHoldoff:
			if s.txLimit >= 0 && at == s.limitAt {
				dst = s.expire(at, dst)
				continue
			}
			if s.state == SenderWaiting {
				s.state, s.holdUntil = SenderHoldoff, at+s.rng.Duration(s.holdoff)
				continue
			}
			if s.busy && s.access != enums.AccessImpolite {
				s.state, s.waitUntil = SenderWaiting, at+s.idleSearch
				continue
			}
			s.state, s.due = SenderSending, at
		case SenderSending:
			return s.send(dst)
		case SenderWaitResponse:
			for k := range s.win {
				s.again(&s.slots[s.ord[k]], nil)
			}
			dst = s.next(at, dst)
		case SenderIdle:
			return dst
		}
	}
}

func (s *Sender) expire(at time.Duration, dst []Output) []Output {
	for k := 0; k < s.n; {
		if s.slots[s.ord[k]].limitAt <= at {
			dst = s.finish(k, ResultTimeLimit, dst)
			continue
		}
		k++
	}
	if s.n > 0 {
		s.limitAt = s.slots[s.ord[0]].limitAt
		for k := 1; k < s.n; k++ {
			s.limitAt = min(s.limitAt, s.slots[s.ord[k]].limitAt)
		}
		s.cur = 0
		s.load()
	}
	return dst
}

func (s *Sender) send(dst []Output) []Output {
	e := &s.slots[s.ord[s.cur]]
	pre := 0
	if s.cur == 0 {
		pre = s.preambles
	}
	var d layer2.Data
	var err error
	switch {
	case s.idx < pre:
		s.csbk = pdu.CSBK{LastBlock: true}
		err = s.csbk.SetPreamble(pdu.PreamblePDU{
			Data: true, Group: s.call.Group, CSBKBlocksToFollow: byte(min(s.total-1-s.idx, 0xFF) & 0xFF),
			TargetAddress: s.call.Target, SourceAddress: s.call.Source,
		})
		d = &s.csbk
	case s.idx == pre:
		if c, ok := e.try.Header.AsConfirmed(); e.cHead && ok {
			c.ResponseRequested = s.cur == s.win-1
			s.closed = s.closed || c.ResponseRequested
			err = e.try.Header.SetConfirmed(c)
		}
		d = &e.try.Header
	default:
		d = e.try.Blocks[s.idx-pre-1]
	}
	var raw layer2.RawBurst
	if err == nil {
		raw, err = layer2.BuildBurst(d, s.cc, s.source)
	}
	if err != nil {
		for s.n > 0 {
			dst = s.finish(0, ResultRefused, dst)
		}
		return dst
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst, s.due)
	o.Burst, o.Try = raw, uint8(min(e.tries, MaxRetries)&0xFF)
	o.Hint = layer2.Hint{Kind: layer2.KindData, DataType: raw.SlotType().DataType, HasDataType: true}
	if e.cHead {
		o.Seq = e.ns
	}
	last := s.due
	s.idx++
	s.due += slotPeriod
	if s.idx < s.total {
		return dst
	}
	if s.cur < s.win-1 {
		s.cur, s.idx = s.cur+1, 0
		s.load()
		return dst
	}
	if !e.confirmed {
		return s.finish(s.cur, ResultSent, dst)
	}
	s.state, s.respUntil = SenderWaitResponse, last+slotPeriod+s.responseWait
	s.asm.Reset()
	return dst
}

// ICMP returns the ICMP error message that the OutputDone o of a message
// of the IP bearer service reports to the IP layer:
// ip.ICMPHostUnreachable for a message that used all its tries on air
// (ResultRetryLimit), that could not finish before SenderConfig.TxLimit
// (ResultTimeLimit), or that got a NACK for the full memory of the
// recipient. It returns ip.ErrNoICMP for any other output, and for a
// message to a talkgroup, which goes as a link layer broadcast (RFC 1122
// clause 3.2.2), and dmrgo.ErrNilReceiver for a nil o. (ETSI TS 102 361-3
// clauses 5.2 and 5.4.2.1)
func (o *Output) ICMP() (ip.ICMPMessage, error) {
	if o == nil {
		return 0, dmrgo.ErrNilReceiver
	}
	if o.Kind != OutputDone || o.Group {
		return 0, ip.ErrNoICMP
	}
	switch o.Result {
	case ResultRetryLimit, ResultTimeLimit:
		return ip.ICMPHostUnreachable, nil
	case ResultRefused:
		if o.HasResponse && o.Response == packet.ResponseNACKMemoryFull {
			return ip.ICMPHostUnreachable, nil
		}
	case ResultSent, ResultDelivered, ResultCancelled:
	}
	return 0, ip.ErrNoICMP
}

//go:noinline
func appendOutput(dst []Output, k OutputKind, due time.Duration) ([]Output, *Output) {
	if len(dst) < cap(dst) {
		dst = dst[:len(dst)+1]
		dst[len(dst)-1] = Output{}
	} else {
		dst = append(dst, Output{})
	}
	o := &dst[len(dst)-1]
	o.Kind, o.Due = k, due
	return dst, o
}
