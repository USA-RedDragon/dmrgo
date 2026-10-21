package data

import (
	"errors"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// MaxEvents is the most events that one call of a Receiver method appends.
const MaxEvents = 14

// Defaults and limits of ReceiverConfig.
const (
	// DefaultLost is the time without a burst, while a packet waits for
	// blocks, after which a Receiver counts the blocks of the slots that
	// passed as lost: one and a half slot periods.
	DefaultLost = 90 * time.Millisecond
	// DefaultHang is the time without a burst after which a Receiver ends
	// a data call. It covers the response wait of the sender, 180 ms
	// (constants.TResponseWait), and the data hang time of a repeater, 180
	// ms (constants.TDataHangtime).
	DefaultHang = 360 * time.Millisecond
	// DefaultPeers is the number of links whose sequence number a Receiver
	// or a Sender keeps.
	DefaultPeers = 8
	// MaxPeers is the most links in the table of sequence numbers.
	MaxPeers = 64
	// MaxBlocks is the most data blocks of one packet, the 7 bits of the
	// Blocks To Follow element (ETSI TS 102 361-1 clause 9.3.21).
	MaxBlocks = 127
	// DefaultMaxFragment is the largest payload of one packet that a
	// Receiver takes when ReceiverConfig.MaxFragment is 0, n_DFragMax
	// (ETSI TS 102 361-1 clause F.2).
	DefaultMaxFragment = 1500
	// DefaultMaxMessage is the largest message that a Receiver joins from
	// fragments when ReceiverConfig.MaxMessage is 0.
	DefaultMaxMessage = 1500
	// MaxMessage is the most octets of ReceiverConfig.MaxFragment and
	// ReceiverConfig.MaxMessage.
	MaxMessage = 65535
)

const maxUDTBlocks = 4

// ReceiverConfig configures NewReceiver. A nil config means the zero
// ReceiverConfig.
type ReceiverConfig struct {
	// ColorCode, when set, drops bursts with another color code, and is the
	// color code of the responses. Nil accepts every color code and
	// answers with the color code of the packet.
	ColorCode *enums.ColorCode
	// Address is the LLID of this station. A confirmed packet gets a
	// response only when its destination is Address. 0 answers every
	// confirmed packet to a radio, as a gateway does.
	Address uint32
	// Respond makes the Receiver answer confirmed packets with an ACK,
	// NACK or SACK (EventRespond), and drop duplicate and out of sequence
	// packets outside the IP bearer service, as the package doc describes.
	// False delivers every complete packet and answers none, as a scanner
	// or bridge needs.
	Respond bool
	// Source selects the SYNC of the responses: layer2.SyncSourceBS, the
	// zero value, for a repeater outbound, layer2.SyncSourceMS for a
	// radio.
	Source layer2.SyncSource
	// ResponseDelay is the time from the last block of a packet to the
	// first response burst. 0 means one slot period (60 ms), the next
	// burst of the logical channel, and a negative value means at once.
	ResponseDelay time.Duration
	// MaxBlocks is the most data blocks of a packet. 0 means MaxBlocks; a
	// larger packet is dropped. Each block takes 24 bytes, so a
	// microcontroller can set 16.
	MaxBlocks int
	// MaxFragment is the largest payload of one packet. 0 means
	// DefaultMaxFragment; at most MaxMessage. A larger confirmed packet
	// gets a NACK for memory full, and any larger packet is dropped.
	MaxFragment int
	// MaxMessage is the size of the buffer that joins the fragments of a
	// confirmed message. 0 means DefaultMaxMessage; at most MaxMessage. A
	// longer message is not joined; its fragments are still delivered.
	MaxMessage int
	// Peers is the number of links whose receive sequence number VI the
	// Receiver keeps. 0 means DefaultPeers; at most MaxPeers. When the
	// table is full, the link heard least recently is dropped (ETSI TS
	// 102 361-1 clause 9.3.24).
	Peers int
	// Lost is the time without a burst, while a packet waits for blocks,
	// after which the slots that passed count as lost blocks, one each 60
	// ms. 0 means DefaultLost, and a negative value counts lost blocks
	// only from the transport sequence number and at the end of the call.
	// A network transport with jitter sets Hint.Seq and a longer Lost.
	Lost time.Duration
	// Hang is the time without a burst after which a data call ends with
	// EndQuiet. 0 means DefaultHang, and a negative value turns the timer
	// off.
	Hang time.Duration
	// AcceptBadCRC delivers packets whose CRC failed, with Message.BadCRC
	// set, as packet.Assembler.AcceptBadCRC describes. A confirmed packet
	// still gets a NACK, so that a good copy can follow.
	AcceptBadCRC bool
	// Scratch, when set, is the Burst that Burst decodes into. Machines
	// that run on one goroutine can share one, because a Receiver is done
	// with it when its method returns. Make it with layer2.NewBurst. Nil
	// means the Receiver makes its own.
	Scratch *layer2.Burst
}

type udt struct {
	active bool
	need   int
	got    int
	header pdu.UDTHeader
	blocks [maxUDTBlocks][12]byte
	buf    [maxUDTBlocks * 12]byte
}

type current struct {
	call      Call
	format    pdu.Format
	sap       enums.ServiceAccessPointID
	cc        enums.ColorCode
	answer    bool
	full      bool
	ns        uint8
	resync    bool
	fsn       uint8
	blocks    uint8
	pending   int
	hasSeq    bool
	seq       uint16
	seqBits   uint8
	lastBlock time.Duration
}

// Receiver turns the bursts of one logical channel into data events: the
// start and end of a data call, each complete packet, the responses heard
// and the responses to send, as the package doc describes. Make it with
// NewReceiver. A Receiver is not safe for concurrent use and must not be
// copied; go vet reports a copy.
type Receiver struct {
	address      uint32
	respond      bool
	source       layer2.SyncSource
	delay        time.Duration
	lostAfter    time.Duration
	hang         time.Duration
	acceptBadCRC bool
	hasCC        bool
	cc           enums.ColorCode
	dataType     enums.DataType
	b            *layer2.Burst
	opts         layer2.DecodeOptions

	state   State
	call    Call
	stats   Stats
	start   time.Duration
	last    time.Duration
	cur     current
	asm     *packet.Assembler
	build   packet.Builder
	peers   peers
	u       udt
	chain   chain
	maxFrag int
	hdr     pdu.UDTHeader
	to      pdu.ConfirmedDataHeader
	pkt     packet.Packet
	vi      uint8
	win     window
	rd      packet.ResponseData
	noCopy  noCopy
}

type window struct {
	n      int
	ns     [MaxWindow]uint8
	resp   [MaxWindow]packet.Response
	blocks [MaxWindow]uint8
}

func (w *window) add(ns uint8, resp packet.Response, blocks uint8) {
	for i := range w.n {
		if w.ns[i] == ns {
			w.n = 0
			break
		}
	}
	if w.n > 0 {
		for next := (w.ns[w.n-1] + 1) & 7; next != ns && w.n < MaxWindow-1; next = (next + 1) & 7 {
			w.ns[w.n], w.resp[w.n], w.blocks[w.n] = next, packet.ResponseSACK, 0
			w.n++
		}
	}
	w.ns[w.n], w.resp[w.n], w.blocks[w.n] = ns, resp, blocks
	w.n++
}

func (w *window) failed() int {
	for i := range w.n {
		if w.resp[i] != packet.ResponseACK {
			return i
		}
	}
	return -1
}

// NewReceiver returns a Receiver for cfg. A nil cfg means the zero
// ReceiverConfig. It returns a *dmrgo.ConfigError for a color code above 15,
// an unknown Source, or a MaxBlocks or Peers out of range. It allocates the
// Receiver, its packet.Assembler, its table of links, and its Burst unless
// cfg.Scratch is set.
func NewReceiver(cfg *ReceiverConfig) (*Receiver, error) {
	if cfg == nil {
		cfg = &ReceiverConfig{}
	}
	if _, _, ok := cfg.Source.SyncPatterns(); !ok {
		return nil, errSource
	}
	if cfg.ColorCode != nil && *cfg.ColorCode > 15 {
		return nil, errColorCode
	}
	blocks, err := limit(cfg.MaxBlocks, MaxBlocks, MaxBlocks, errMaxBlocks)
	if err != nil {
		return nil, err
	}
	n, err := limit(cfg.Peers, DefaultPeers, MaxPeers, errPeers)
	if err != nil {
		return nil, err
	}
	maxFrag, err := limit(cfg.MaxFragment, DefaultMaxFragment, MaxMessage, errMaxFragment)
	if err != nil {
		return nil, err
	}
	maxMsg, err := limit(cfg.MaxMessage, DefaultMaxMessage, MaxMessage, errMaxMessage)
	if err != nil {
		return nil, err
	}
	r := &Receiver{
		address:      cfg.Address,
		respond:      cfg.Respond,
		source:       cfg.Source,
		delay:        max(defaultDuration(cfg.ResponseDelay, slotPeriod), 0),
		lostAfter:    defaultDuration(cfg.Lost, DefaultLost),
		hang:         defaultDuration(cfg.Hang, DefaultHang),
		acceptBadCRC: cfg.AcceptBadCRC,
		b:            cfg.Scratch,
		asm:          packet.NewAssembler(blocks),
		peers:        peers{list: make([]peer, n)},
		chain:        chain{buf: make([]byte, maxMsg)},
		maxFrag:      maxFrag,
	}
	r.asm.AcceptBadCRC = cfg.AcceptBadCRC
	if cfg.ColorCode != nil {
		r.cc, r.hasCC = *cfg.ColorCode, true
		r.opts.ColorCode = &r.cc
	}
	if r.b == nil {
		r.b = layer2.NewBurst()
	}
	return r, nil
}

func limit(v, def, maxV int, err error) (int, error) {
	switch {
	case v == 0:
		return def, nil
	case v < 0 || v > maxV:
		return 0, err
	}
	return v, nil
}

func defaultDuration(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

// Reset ends any data call without events and forgets the packet in
// progress and the sequence numbers of every link. It keeps the config.
func (r *Receiver) Reset() {
	if r == nil {
		return
	}
	r.state, r.call, r.stats = StateIdle, Call{}, Stats{}
	r.cur = current{}
	if r.asm != nil {
		r.asm.Reset()
	}
	r.peers.reset()
	r.u.active = false
	r.chain.active = false
	r.win.n = 0
}

// State returns the state of the Receiver.
func (r *Receiver) State() State {
	if r == nil {
		return 0
	}
	return r.state
}

// Burst decodes the burst raw with what the Hint h says about it, runs it
// through the Receiver as Decoded does, and appends the events to dst. A
// burst that does not decode, or has another color code than
// ReceiverConfig.ColorCode, is dropped, but its Hint still applies. A nil
// raw is a transport frame without a burst: only h applies, and
// Hint.SyncLost ends the call as Lost does. It allocates only when dst has
// no room for MaxEvents events.
func (r *Receiver) Burst(now time.Duration, raw *layer2.RawBurst, h layer2.Hint, dst []Event) []Event {
	if r == nil {
		return dst
	}
	if raw == nil || r.b == nil {
		return r.receive(now, nil, &h, dst)
	}
	r.opts.Kind, r.opts.VoiceBurst, r.opts.DataType = h.Kind, enums.VoiceBurstUnknown, nil
	if h.Kind == layer2.KindVoice {
		r.opts.VoiceBurst = h.VoiceBurst
	}
	if h.HasDataType {
		r.dataType = h.DataType
		r.opts.DataType = &r.dataType
	}
	if r.b.DecodeFromBytesWith(*raw, &r.opts) != nil {
		return r.receive(now, nil, &h, dst)
	}
	return r.receive(now, r.b, &h, dst)
}

// Decoded runs the decoded burst b through the Receiver and appends the
// events to dst. ReceiverConfig.ColorCode does not apply: b was decoded
// with the options of the caller, so set DecodeOptions.ColorCode for the
// check. A nil b means only h applies, as for a nil raw in Burst. A
// monitor that feeds a burst to several machines decodes it once with
// layer2.Burst.DecodeFromBytesWith and calls Decoded on each, as the
// example of voice.Receiver.Decoded shows. It allocates only when dst has
// no room for MaxEvents events.
func (r *Receiver) Decoded(now time.Duration, b *layer2.Burst, h layer2.Hint, dst []Event) []Event {
	if r == nil {
		return dst
	}
	if r.asm == nil {
		b = nil
	}
	return r.receive(now, b, &h, dst)
}

// Lost tells the Receiver that the transport lost the signal, such as an
// MMDVM TAG_LOST. The blocks still due count as lost, which can complete a
// packet, and the call ends with EndSyncLost. It appends the events to
// dst, and allocates only when dst has no room for MaxEvents events.
func (r *Receiver) Lost(now time.Duration, dst []Event) []Event {
	if r == nil {
		return dst
	}
	dst = r.timers(now, dst)
	return r.end(now, EndSyncLost, dst)
}

// Tick runs the timers that are due at now and appends their events to
// dst: lost blocks after ReceiverConfig.Lost, which can complete a packet
// and send its SACK, and the end of the call after ReceiverConfig.Hang. A
// Tick before Deadline does nothing. It allocates only when dst has no
// room for MaxEvents events.
func (r *Receiver) Tick(now time.Duration, dst []Event) []Event {
	if r == nil {
		return dst
	}
	return r.timers(now, dst)
}

// Deadline returns the time at which the Receiver needs a Tick. ok is
// false when no timer runs.
func (r *Receiver) Deadline() (at time.Duration, ok bool) {
	if r == nil {
		return 0, false
	}
	if r.state == StateReceiving && r.lostAfter >= 0 {
		at, ok = r.cur.lastBlock+r.lostAfter, true
	}
	if r.state != StateIdle && r.hang >= 0 && (!ok || r.last+r.hang < at) {
		at, ok = r.last+r.hang, true
	}
	return at, ok
}

func (r *Receiver) timers(now time.Duration, out []Event) []Event {
	for {
		at, ok := r.Deadline()
		if !ok || now < at {
			return out
		}
		if r.state == StateReceiving && r.lostAfter >= 0 && at == r.cur.lastBlock+r.lostAfter {
			out = r.lose(at, max(lostSlots(at-r.cur.lastBlock), 1), out)
			continue
		}
		out = r.end(at, EndQuiet, out)
	}
}

func lostSlots(d time.Duration) int {
	return int((d+slotPeriod/2)/slotPeriod) - 1
}

func (r *Receiver) lose(at time.Duration, n int, out []Event) []Event {
	for ; n > 0 && r.cur.pending > 0; n-- {
		r.cur.pending--
		r.stats.Lost++
		r.cur.lastBlock += slotPeriod
		r.cur.seq++
		if r.u.active {
			r.u.active = false
			r.cur.pending = 0
			r.stats.Dropped++
			continue
		}
		m, done, err := r.asm.Lost()
		if done || errors.Is(err, packet.ErrBlocksMissing) {
			out = r.complete(at, &m, done, err, out)
		}
	}
	r.settle()
	return out
}

func (r *Receiver) settle() {
	switch {
	case r.state == StateIdle:
	case r.cur.pending > 0:
		r.state = StateReceiving
	default:
		r.state = StateCall
	}
}

func (r *Receiver) end(at time.Duration, reason EndReason, out []Event) []Event {
	if r.state == StateIdle {
		return out
	}
	out = r.lose(at, r.cur.pending, out)
	r.u.active = false
	r.stats.Duration = max(at-r.start, 0)
	var e *Event
	out, e = appendEvent(out, EventCallEnd, &r.call)
	e.Reason, e.Stats = reason, r.stats
	r.state = StateIdle
	return out
}

func (r *Receiver) begin(now time.Duration, c *Call, out []Event) []Event {
	reply := !c.Group && c.Target == r.call.Source && c.Source == r.call.Target
	if r.state != StateIdle && *c != r.call && !reply {
		out = r.end(now, EndSuperseded, out)
	}
	if r.state == StateIdle {
		r.call, r.stats, r.start, r.state = *c, Stats{}, now, StateCall
		r.win.n = 0
		out, _ = appendEvent(out, EventCallStart, &r.call)
	}
	r.last = now
	return out
}

func (r *Receiver) receive(now time.Duration, b *layer2.Burst, h *layer2.Hint, out []Event) []Event {
	out = r.timers(now, out)
	if h.SyncLost {
		out = r.end(now, EndSyncLost, out)
	}
	if b != nil {
		switch b.Kind {
		case layer2.KindVoice:
			out = r.end(now, EndVoice, out)
		case layer2.KindData:
			out = r.data(now, b, h, out)
		case layer2.KindUnknown, layer2.KindReverseChannel:
		}
	}
	if h.End == layer2.EndStrong {
		out = r.end(now, EndTransport, out)
	}
	return out
}

func (r *Receiver) data(now time.Duration, b *layer2.Burst, h *layer2.Hint, out []Event) []Event {
	if b.SyncPattern == enums.SyncNone || b.SlotType.FEC.Uncorrectable {
		return out
	}
	switch b.SlotType.DataType {
	case enums.DataTypeCSBK:
		return r.preamble(now, b, out)
	case enums.DataTypeDataHeader:
		if dh, ok := b.Data.(*pdu.DataHeader); ok {
			return r.header(now, dh, b.SlotType.ColorCode, h, out)
		}
	case enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
		return r.block(now, b, h, out)
	case enums.DataTypeVoiceLCHeader:
		return r.end(now, EndVoice, out)
	case enums.DataTypeIdle:
		if r.state == StateCall {
			return r.end(now, EndIdle, out)
		}
	case enums.DataTypeTerminatorWithLC, enums.DataTypePIHeader, enums.DataTypeMBCHeader,
		enums.DataTypeMBCContinuation, enums.DataTypeUnifiedSingleBlock, enums.DataTypeReserved:
		r.touch(now)
	}
	return out
}

func (r *Receiver) touch(now time.Duration) {
	if r.state != StateIdle {
		r.last = now
	}
}

func (r *Receiver) preamble(now time.Duration, b *layer2.Burst, out []Event) []Event {
	c, ok := b.Data.(*pdu.CSBK)
	if !ok || c.FEC.Uncorrectable || c.PreamblePDU == nil || !c.PreamblePDU.Data || r.state == StateReceiving {
		r.touch(now)
		return out
	}
	p := c.PreamblePDU
	call := Call{Group: p.Group, Target: p.TargetAddress, Source: p.SourceAddress}
	return r.begin(now, &call, out)
}

func (r *Receiver) header(now time.Duration, dh *pdu.DataHeader, cc enums.ColorCode, h *layer2.Hint, out []Event) []Event {
	if dh.FEC.Uncorrectable {
		r.stats.HeaderCRC++
		r.touch(now)
		return out
	}
	if dh.Format == pdu.FormatProprietary {
		r.touch(now)
		m, done, err := r.asm.Add(dh)
		switch {
		case err != nil:
			r.stats.Dropped++
		case r.cur.pending > 0:
			r.cur.pending--
			r.cur.lastBlock = now
			r.cur.hasSeq, r.cur.seq, r.cur.seqBits = h.HasSeq, h.Seq, h.SeqBits
		}
		if done {
			out = r.complete(now, &m, done, err, out)
		}
		r.settle()
		return out
	}
	c := callOf(dh)
	out = r.begin(now, &c, out)
	prev := r.cur
	r.cur = current{
		call: c, format: dh.Format, cc: cc, full: true,
		hasSeq: h.HasSeq, seq: h.Seq, seqBits: h.SeqBits, lastBlock: now,
	}
	if r.hasCC {
		r.cur.cc = r.cc
	}
	r.readHeader(dh)
	r.cur.pending = int(r.cur.blocks)
	r.u.active = false
	if dh.Format == pdu.FormatUnifiedDataTransport && dh.UDTHeader != nil {
		r.u = udt{active: true, need: int(dh.UDTHeader.AppendedBlocks) + 1, header: *dh.UDTHeader}
		r.cur.pending = r.u.need
		r.settle()
		return out
	}
	m, done, err := r.asm.Add(dh)
	switch {
	case errors.Is(err, packet.ErrUnexpectedRetry):
		r.stats.Dropped++
		r.cur = prev
	case err != nil && !done:
		r.stats.Dropped++
		r.cur.pending = 0
	case done:
		out = r.complete(now, &m, done, err, out)
	case !r.asm.InProgress():
		r.cur.pending = 0
	}
	r.settle()
	return out
}

func (r *Receiver) readHeader(dh *pdu.DataHeader) {
	switch {
	case dh.UnconfirmedDataHeader != nil:
		u := dh.UnconfirmedDataHeader
		r.cur.sap, r.cur.blocks, r.cur.full = u.SAP, u.BlocksToFollow, u.FullMessageFlag
	case dh.ConfirmedDataHeader != nil:
		c := dh.ConfirmedDataHeader
		r.cur.sap, r.cur.blocks, r.cur.full = c.SAP, c.BlocksToFollow, c.FullMessageFlag
		r.cur.answer, r.cur.ns, r.cur.resync = c.ResponseRequested, c.SendSequenceNumber, c.ReSynchronizeFlag
		r.cur.fsn = c.FragmentSequenceNumber
	case dh.DefinedDataHeader != nil:
		d := dh.DefinedDataHeader
		r.cur.sap, r.cur.blocks, r.cur.full = d.SAP, d.AppendedBlocks, d.FullMessageFlag
		r.cur.answer = d.ResponseRequested && d.SARQ
	case dh.RawDataHeader != nil:
		d := dh.RawDataHeader
		r.cur.sap, r.cur.blocks, r.cur.full = d.SAP, d.AppendedBlocks, d.FullMessageFlag
		r.cur.answer = d.ResponseRequested && d.SARQ
	case dh.ResponsePacketHeader != nil:
		r.cur.sap, r.cur.blocks = dh.ResponsePacketHeader.SAP, dh.ResponsePacketHeader.BlocksToFollow
	case dh.StatusPrecodedHeader != nil:
		r.cur.sap = dh.StatusPrecodedHeader.SAP
	case dh.UDTHeader != nil:
		r.cur.sap = dh.UDTHeader.SAP
	}
}

func (r *Receiver) block(now time.Duration, b *layer2.Burst, h *layer2.Hint, out []Event) []Event {
	if r.cur.pending == 0 {
		r.stats.Dropped++
		r.touch(now)
		return out
	}
	r.last = now
	if n := r.gap(now, h); n > 0 {
		out = r.lose(now, n, out)
		if r.cur.pending == 0 {
			r.stats.Dropped++
			return out
		}
	}
	r.cur.lastBlock = now
	r.cur.hasSeq, r.cur.seq, r.cur.seqBits = h.HasSeq, h.Seq, h.SeqBits
	r.cur.pending--
	r.stats.Blocks++
	if b.FEC.PDU.Uncorrectable {
		r.stats.Lost++
	}
	if r.u.active {
		out = r.udtBlock(b, out)
		r.settle()
		return out
	}
	m, done, err := r.asm.AddBurst(b)
	switch {
	case done || errors.Is(err, packet.ErrBlocksMissing):
		out = r.complete(now, &m, done, err, out)
	case err != nil:
		r.stats.Dropped++
		r.cur.pending = 0
	}
	r.settle()
	return out
}

func (r *Receiver) gap(now time.Duration, h *layer2.Hint) int {
	switch {
	case h.HasSeq && r.cur.hasSeq:
		n := h.SeqBits
		if n == 0 || n > 16 {
			n = 16
		}
		mask := uint16(1<<n - 1)
		d := (h.Seq - r.cur.seq) & mask
		if d == 0 || d > mask/2 {
			return 0
		}
		return int(d) - 1
	case h.HasSeq:
		return 0
	}
	return lostSlots(now - r.cur.lastBlock)
}

func (r *Receiver) udtBlock(b *layer2.Burst, out []Event) []Event {
	blk, ok := b.Data.(*pdu.Rate12Data)
	if !ok || b.FEC.PDU.Uncorrectable {
		r.u.active = false
		r.cur.pending = 0
		r.stats.Dropped++
		return out
	}
	r.u.blocks[r.u.got] = blk.Data
	r.u.got++
	if r.u.got < r.u.need {
		return out
	}
	r.u.active = false
	r.cur.pending = 0
	r.stats.Packets++
	ok = pdu.MBCLastBlockCRCOK(r.u.blocks[:r.u.need])
	if !ok {
		r.stats.BadCRC++
		if !r.acceptBadCRC {
			return out
		}
	}
	n := 0
	for i := range r.u.need {
		n += copy(r.u.buf[n:], r.u.blocks[i][:])
	}
	n = max(n-2-int(r.u.header.PadNibble)/2, 0)
	r.hdr = r.u.header
	var e *Event
	out, e = appendEvent(out, EventDelivered, &r.call)
	e.Message = Message{
		Header:  pdu.DataHeader{Format: pdu.FormatUnifiedDataTransport, UDTHeader: &r.hdr},
		Rate:    enums.DataTypeRate12,
		Payload: r.u.buf[:n],
		BadCRC:  !ok,
	}
	r.stats.Delivered++
	return out
}

func (r *Receiver) complete(now time.Duration, m *packet.Message, done bool, err error, out []Event) []Event {
	r.cur.pending = 0
	if done && m.Header.Format == pdu.FormatResponsePacket {
		return r.response(m, err, out)
	}
	if !done {
		return r.answer(now, r.missing(), out)
	}
	r.stats.Packets++
	if err != nil || m.BadCRC {
		r.stats.BadCRC++
		out = r.answer(now, packet.ResponseNACKPacketCRC, out)
		if err != nil {
			return out
		}
	}
	var link *peer
	if !m.BadCRC && r.answers() && r.cur.format == pdu.FormatConfirmed {
		sap := r.cur.sap
		if sap == enums.SAPProprietaryPacketData && m.Proprietary != nil {
			sap = m.Proprietary.SAP
		}
		var s int
		link, s = r.sequence(sap)
		switch s {
		case seqDuplicate:
			r.stats.Duplicates++
			return r.answer(now, packet.ResponseACK, out)
		case seqOut:
			r.stats.OutOfSequence++
			return r.answer(now, packet.ResponseNACKOutOfSequence, out)
		case seqNext:
		}
	}
	if len(m.Data) > r.maxFrag {
		r.stats.Dropped++
		return r.answer(now, packet.ResponseNACKMemoryFull, out)
	}
	fragmented, joined, ok := r.fragment(m)
	if !ok && r.answers() {
		r.stats.OutOfSequence++
		return r.answer(now, packet.ResponseNACKFSNOutOfSequence, out)
	}
	if link != nil {
		link.seq, link.valid = r.cur.ns, true
	}
	var e *Event
	out, e = appendEvent(out, EventDelivered, &r.call)
	e.Message = Message{Header: m.Header, Proprietary: m.Proprietary, Rate: m.DataType, Payload: m.Data, BadCRC: m.BadCRC, Fragmented: fragmented}
	r.stats.Delivered++
	if joined {
		out, e = appendEvent(out, EventReassembled, &r.call)
		e.Message = Message{Header: m.Header, Rate: m.DataType, Payload: r.chain.buf[:r.chain.n], Fragments: r.chain.count}
	}
	if !m.BadCRC {
		out = r.answer(now, packet.ResponseACK, out)
	}
	return out
}

type chain struct {
	active         bool
	source, target uint32
	next           uint8
	count          uint16
	n              int
	buf            []byte
}

func (r *Receiver) fragment(m *packet.Message) (fragmented, joined, ok bool) {
	c := &r.chain
	if r.cur.format != pdu.FormatConfirmed || r.cur.fsn == 0b1000 {
		c.active = false
		return false, false, true
	}
	low, last := r.cur.fsn&0b111, r.cur.fsn&0b1000 != 0
	if low == 0 {
		c.active = len(m.Data) <= len(c.buf) && !last
		c.source, c.target, c.next, c.count = r.cur.call.Source, r.cur.call.Target, 1, 1
		c.n = copy(c.buf, m.Data)
		return false, false, true
	}
	if !c.active || c.source != r.cur.call.Source || c.target != r.cur.call.Target || low != c.next {
		c.active = false
		return true, false, false
	}
	if c.n+len(m.Data) > len(c.buf) {
		c.active = false
		return true, false, true
	}
	c.n += copy(c.buf[c.n:], m.Data)
	c.count++
	c.next = low%7 + 1
	if last {
		c.active = false
		return true, true, true
	}
	return true, false, true
}

func (r *Receiver) missing() packet.Response {
	if !r.cur.full || r.cur.format != pdu.FormatConfirmed {
		return packet.ResponseNACKPacketCRC
	}
	return packet.ResponseSACK
}

const (
	seqNext = iota
	seqDuplicate
	seqOut
)

// ETSI TS 102 361-1 clauses 9.3.23 and 9.3.24, clause 8.2.2.3 table 8.3
func (r *Receiver) sequence(sap enums.ServiceAccessPointID) (*peer, int) {
	if ipBearer(sap) {
		return nil, seqNext
	}
	p, _ := r.peers.lookup(r.cur.call.Source, r.cur.call.Target)
	switch {
	case p == nil:
		return nil, seqNext
	case !p.valid || r.cur.resync:
		return p, seqNext
	case r.cur.ns == p.seq:
		return nil, seqDuplicate
	case r.cur.ns == (p.seq+1)&7:
		return p, seqNext
	}
	r.vi = p.seq
	return nil, seqOut
}

// ETSI TS 102 361-3 clause 5.4.2.2 figure 5.9, clause 5.4.1.3 table 5.7
func ipBearer(sap enums.ServiceAccessPointID) bool {
	switch sap {
	case enums.SAPUDPIPHeaderCompression, enums.SAPIPBasedPacketData, enums.SAPAddressResolutionProtocol:
		return true
	case enums.SAPUnifiedDataTransport, enums.SAPTCPIPHeaderCompression, enums.SAPProprietaryPacketData, enums.SAPShortData:
	}
	return false
}

func (r *Receiver) response(m *packet.Message, err error, out []Event) []Event {
	rh := m.Header.ResponsePacketHeader
	if rh == nil {
		return out
	}
	code, cerr := packet.ResponseOf(rh)
	if cerr != nil {
		return out
	}
	var e *Event
	out, e = appendEvent(out, EventResponse, &r.call)
	e.Response = code
	if code == packet.ResponseSACK && err == nil {
		e.Retry = m.Response
	}
	return out
}

func (r *Receiver) answers() bool {
	c := &r.cur
	return r.respond && c.answer && !c.call.Group && (r.address == 0 || c.call.Target == r.address)
}

func (r *Receiver) windowed() bool {
	c := &r.cur
	return r.respond && c.format == pdu.FormatConfirmed && ipBearer(c.sap) && !c.call.Group &&
		(r.address == 0 || c.call.Target == r.address)
}

// ETSI TS 102 361-3 clause 5.4.4
func (r *Receiver) answer(now time.Duration, resp packet.Response, out []Event) []Event {
	c := &r.cur
	r.to = pdu.ConfirmedDataHeader{
		SAP: c.sap, LLIDDestination: c.call.Target, LLIDSource: c.call.Source,
		BlocksToFollow: c.blocks, SendSequenceNumber: c.ns,
	}
	current := true
	if r.windowed() {
		blocks := c.blocks
		if !c.full {
			blocks = 0
		}
		r.win.add(c.ns, resp, blocks)
		if !c.answer {
			return out
		}
		i := r.win.failed()
		if i >= 0 && i < r.win.n-1 {
			current = false
			resp, r.to.SendSequenceNumber, r.to.BlocksToFollow = r.win.resp[i], r.win.ns[i], r.win.blocks[i]
			if resp == packet.ResponseNACKPacketCRC {
				resp = packet.ResponseSACK
			}
			if r.to.BlocksToFollow == 0 {
				r.to.BlocksToFollow = MaxBlocks
			}
		}
		r.win.n = 0
	}
	if !r.answers() {
		return out
	}
	if resp == packet.ResponseNACKOutOfSequence {
		r.to.SendSequenceNumber = r.vi
	}
	var err error
	switch {
	case resp == packet.ResponseSACK && current:
		r.pkt, err = r.asm.Response()
	case resp == packet.ResponseSACK:
		for i := range r.rd.Retry {
			r.rd.Retry[i] = true
		}
		r.pkt, err = r.build.NewResponse(&r.to, resp, &r.rd)
	default:
		r.pkt, err = r.build.NewResponse(&r.to, resp, nil)
	}
	if err != nil {
		return out
	}
	p := &r.pkt
	due := now + r.delay
	for i := -1; i < len(p.Blocks); i++ {
		var d layer2.Data = &p.Header
		if i >= 0 {
			d = p.Blocks[i]
		}
		raw, err := layer2.BuildBurst(d, c.cc, r.source)
		if err != nil {
			return out
		}
		var e *Event
		out, e = appendEvent(out, EventRespond, &r.call)
		e.Response, e.Burst, e.Due = resp, raw, due+time.Duration(i+1)*slotPeriod
		r.last = max(r.last, e.Due)
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
