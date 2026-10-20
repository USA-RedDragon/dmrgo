package data

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/constants"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxOutputs is the most outputs that one call of a Sender method appends:
// one burst and the end of the message.
const MaxOutputs = 2

// Defaults and limits of SenderConfig.
const (
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
	// Due is the time at which the burst goes on air, 60 ms after the one
	// before it.
	Due time.Duration
	// Try is the transmission that the burst belongs to, or that ended the
	// message: 1 for the first.
	Try uint8
	// Result is how the message ended.
	Result Result
	// Response is the last response to the message. HasResponse reports
	// whether one came.
	Response packet.Response
	// HasResponse reports whether Response is set.
	HasResponse bool
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

const (
	resendFirst = iota
	resendLater
	resendSelective
)

// Sender sends one data message at a time on one logical channel: the
// preamble CSBKs, the header and the blocks of each try, one burst per
// Tick, 60 ms apart, after the channel access of SenderConfig.Access. For
// a confirmed message it waits for the response and resends: the blocks
// that a SACK asks for, the whole packet after a NACK for a bad CRC or no
// response, and the whole packet again as a first try after a NACK for
// N(S) out of sequence, which some radios send for a lost block. Make it
// with NewSender. A Sender is not safe for concurrent use and must not be
// copied; go vet reports a copy.
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
	pb   *packet.Builder

	state     SenderState
	busy      bool
	pkt       packet.Packet
	try       packet.Packet
	call      Call
	confirmed bool
	link      *peer
	idx       int
	total     int
	tries     int
	due       time.Duration
	waitUntil time.Duration
	holdUntil time.Duration
	respUntil time.Duration
	limitAt   time.Duration
	last      packet.Response
	hasLast   bool

	u      pdu.UnconfirmedDataHeader
	c      pdu.ConfirmedDataHeader
	dd     pdu.DefinedDataHeader
	raw    pdu.RawDataHeader
	pre    pdu.PreamblePDU
	csbk   pdu.CSBK
	noCopy noCopy
}

// NewSender returns a Sender for cfg. A nil cfg means the zero
// SenderConfig. It returns a *ConfigError for a color code above 15, an
// unknown Source or Access, or a Preambles, Retries, MaxBlocks or Peers out
// of range. It allocates the Sender, its packet.Builder and
// packet.Assembler, its table of links and its Burst.
func NewSender(cfg *SenderConfig) (*Sender, error) {
	if cfg == nil {
		cfg = &SenderConfig{}
	}
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
	return &Sender{
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
		pb:           packet.NewBuilder(blocks),
	}, nil
}

// Reset ends any message without outputs and forgets the sequence numbers
// of every link and the channel state. It keeps the config and the state
// of the random holdoff.
func (s *Sender) Reset() {
	s.state, s.busy, s.hasLast = SenderIdle, false, false
	s.link = nil
	s.peers.reset()
	s.asm.Reset()
}

// State returns the state of the Sender.
func (s *Sender) State() SenderState {
	return s.state
}

// Send starts the message m at now. m.Header is a U_HEAD, a C_HEAD, or a
// DD_HEAD or R_HEAD, with its sub-header set, and m.Rate is the rate of
// the blocks. Send copies the header and m.Payload, so the caller can
// reuse them. It sets the block count, pad octets and Full Message Flag,
// and for a C_HEAD the Response Requested flag, N(S) from the link, S
// (with SenderConfig.Resync) and FSN 1000, a message in one fragment. It
// sets the Response Requested flag of a short data header with SARQ set.
// It returns ErrBusy while a message is in progress, a *ConfigError for
// another header, a P_HEAD in m.Proprietary, or a confirmed message to a
// group, and the errors of packet.Builder.Segment, such as
// packet.ErrTooLong and packet.ErrRate.
func (s *Sender) Send(now time.Duration, m *Message) error {
	if s.state != SenderIdle {
		return ErrBusy
	}
	if m.Proprietary != nil {
		return errHeader
	}
	h := m.Header
	switch {
	case h.Format == pdu.FormatUnconfirmed && h.UnconfirmedDataHeader != nil:
		s.u = *h.UnconfirmedDataHeader
		s.u.FullMessageFlag = true
		h.UnconfirmedDataHeader = &s.u
		s.confirmed = false
	case h.Format == pdu.FormatConfirmed && h.ConfirmedDataHeader != nil:
		if h.ConfirmedDataHeader.Group {
			return errMessageGroup
		}
		s.c = *h.ConfirmedDataHeader
		s.c.FullMessageFlag, s.c.ResponseRequested, s.c.FragmentSequenceNumber = true, true, 0b1000
		h.ConfirmedDataHeader = &s.c
		s.confirmed = true
	case h.Format == pdu.FormatShortDataDefined && h.DefinedDataHeader != nil:
		s.dd = *h.DefinedDataHeader
		s.dd.FullMessageFlag = true
		s.dd.ResponseRequested = s.dd.ResponseRequested || s.dd.SARQ
		h.DefinedDataHeader = &s.dd
		s.confirmed = s.dd.SARQ
	case h.Format == pdu.FormatShortDataRawOrStatusPrecoded && h.RawDataHeader != nil:
		s.raw = *h.RawDataHeader
		s.raw.FullMessageFlag = true
		s.raw.ResponseRequested = s.raw.ResponseRequested || s.raw.SARQ
		h.RawDataHeader = &s.raw
		s.confirmed = s.raw.SARQ
	default:
		return errHeader
	}
	if s.confirmed && groupOf(&h) {
		return errMessageGroup
	}
	s.call = callOf(&h)
	s.link = nil
	if h.Format == pdu.FormatConfirmed {
		link, found := s.peers.lookup(s.call.Source, s.call.Target)
		if link != nil {
			s.c.SendSequenceNumber = link.seq
		}
		s.c.ReSynchronizeFlag = s.resync && !found
		s.link = link
	}
	p, err := s.pb.Segment(h, m.Payload, m.Rate)
	if err != nil {
		return err
	}
	s.pkt, s.tries, s.hasLast = p, 0, false
	s.limitAt = now + s.txLimit
	s.asm.Reset()
	s.start(now, &s.pkt)
	return nil
}

func (s *Sender) start(now time.Duration, p *packet.Packet) {
	s.try = *p
	s.idx, s.total = 0, s.preambles+1+len(p.Blocks)
	s.tries++
	switch {
	case s.access == enums.AccessImpolite || !s.busy:
		s.state, s.due = SenderSending, now
	default:
		s.state, s.waitUntil = SenderWaiting, now+s.idleSearch
	}
}

// Channel tells the Sender whether the channel is busy at now, for example
// from the AT bit of the CACH, the carrier, or a repeater. A polite Sender
// that waits for the channel starts its try when it becomes idle. An
// impolite Sender ignores it.
func (s *Sender) Channel(now time.Duration, busy bool) {
	s.busy = busy
	if !busy && s.state == SenderWaiting {
		s.state, s.due = SenderSending, now
	}
}

// Cancel ends the message in progress with ResultCancelled and appends the
// output to dst. It does nothing without a message, and allocates only
// when dst has no room for MaxOutputs outputs.
func (s *Sender) Cancel(_ time.Duration, dst []Output) []Output {
	if s.state == SenderIdle {
		return dst
	}
	return s.done(ResultCancelled, dst)
}

// Burst decodes the burst raw and takes the response to the confirmed
// message in progress from it: a response packet from the target of the
// message to its source. It appends the end of the message, or nothing
// when a resend starts. Other bursts are ignored; the channel state comes
// from Channel. It allocates only when dst has no room for MaxOutputs
// outputs.
func (s *Sender) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Output) []Output {
	s.opts.Kind = h.Kind
	if s.b.DecodeFromBytesWith(*raw, &s.opts) != nil {
		return dst
	}
	return s.Decoded(now, s.b, h, dst)
}

// Decoded takes the response to the confirmed message in progress from
// the decoded burst b, as Burst does. It allocates only when dst has no
// room for MaxOutputs outputs.
func (s *Sender) Decoded(now time.Duration, b *layer2.Burst, _ layer2.Hint, dst []Output) []Output {
	dst = s.timers(now, dst)
	if b.Kind != layer2.KindData || b.SlotType.FEC.Uncorrectable || s.state != SenderWaitResponse {
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
	if !done || m.Header.Format != pdu.FormatResponsePacket {
		return dst
	}
	rh := m.Header.ResponsePacketHeader
	if rh == nil || rh.LLIDDestination != s.call.Source || rh.LLIDSource != s.call.Target {
		return dst
	}
	code, cerr := packet.ResponseOf(rh)
	if cerr != nil {
		return dst
	}
	s.last, s.hasLast = code, true
	switch code {
	case packet.ResponseACK:
		return s.done(ResultDelivered, dst)
	case packet.ResponseSACK:
		if err != nil || m.Response == nil {
			return s.resend(now, resendLater, nil, dst)
		}
		return s.resend(now, resendSelective, m.Response, dst)
	case packet.ResponseNACKOutOfSequence:
		if s.link != nil && s.resync {
			s.c.ReSynchronizeFlag = true
			if p := s.pkt.Header.ConfirmedDataHeader; p != nil {
				p.ReSynchronizeFlag = true
			}
		}
		return s.resend(now, resendFirst, nil, dst)
	case packet.ResponseNACKPacketCRC:
		return s.resend(now, resendLater, nil, dst)
	case packet.ResponseNACKIllegalFormat, packet.ResponseNACKMemoryFull, packet.ResponseNACKFSNOutOfSequence,
		packet.ResponseNACKUndeliverable, packet.ResponseNACKInvalidUser:
	}
	return s.done(ResultRefused, dst)
}

func (s *Sender) resend(now time.Duration, k int, rd *packet.ResponseData, dst []Output) []Output {
	if s.tries >= s.retries {
		return s.done(ResultRetryLimit, dst)
	}
	if s.txLimit >= 0 && now >= s.limitAt {
		return s.done(ResultTimeLimit, dst)
	}
	if k == resendFirst {
		s.start(now, &s.pkt)
		return dst
	}
	if k == resendSelective {
		p, err := s.pb.Retry(&s.pkt, rd)
		if err == nil {
			s.start(now, &p)
			return dst
		}
	}
	p, err := s.pb.Retry(&s.pkt, nil)
	if err != nil {
		s.start(now, &s.pkt)
		return dst
	}
	s.start(now, &p)
	return dst
}

func (s *Sender) done(result Result, dst []Output) []Output {
	if s.link != nil && s.tries > 0 {
		s.link.seq = (s.link.seq + 1) & 7
	}
	s.link = nil
	var o *Output
	dst, o = appendOutput(dst, OutputDone, 0)
	o.Try, o.Result, o.Response, o.HasResponse = uint8(min(s.tries, MaxRetries)&0xFF), result, s.last, s.hasLast
	s.state = SenderIdle
	return dst
}

// Deadline returns the time at which the Sender needs a Tick. ok is false
// when no timer runs.
func (s *Sender) Deadline() (at time.Duration, ok bool) {
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
				return s.done(ResultTimeLimit, dst)
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
			dst = s.resend(at, resendLater, nil, dst)
		case SenderIdle:
			return dst
		}
	}
}

func (s *Sender) send(dst []Output) []Output {
	var d layer2.Data
	switch {
	case s.idx < s.preambles:
		s.pre = pdu.PreamblePDU{
			Data: true, Group: s.call.Group, CSBKBlocksToFollow: byte(min(s.total-1-s.idx, 0xFF) & 0xFF),
			TargetAddress: s.call.Target, SourceAddress: s.call.Source,
		}
		s.csbk = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPreamble, PreamblePDU: &s.pre}
		d = &s.csbk
	case s.idx == s.preambles:
		d = &s.try.Header
	default:
		d = s.try.Blocks[s.idx-s.preambles-1]
	}
	raw, err := layer2.BuildBurst(d, s.cc, s.source)
	if err != nil {
		return s.done(ResultRefused, dst)
	}
	var o *Output
	dst, o = appendOutput(dst, OutputBurst, s.due)
	o.Burst, o.Try = raw, uint8(min(s.tries, MaxRetries)&0xFF)
	last := s.due
	s.idx++
	s.due += slotPeriod
	if s.idx < s.total {
		return dst
	}
	if !s.confirmed {
		return s.done(ResultSent, dst)
	}
	s.state, s.respUntil = SenderWaitResponse, last+slotPeriod+s.responseWait
	s.asm.Reset()
	return dst
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
