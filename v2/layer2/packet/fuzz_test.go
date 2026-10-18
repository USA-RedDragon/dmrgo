package packet_test

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/packet"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// fuzzRounds is the most fuzzed transmissions of one packet in a session.
// Two clean transmissions follow, after which the packet must be delivered.
const fuzzRounds = 8

// maxEvents is the most receive events of one transmission: every burst of
// a packet of 127 blocks, each duplicated.
const maxEvents = 2 * 128

// Fates of a burst on the fuzzed channel, from the low 3 bits of its op.
const (
	fateLost      = 3 // the burst is lost and the receiver calls Lost
	fateSilent    = 4 // the burst is lost and the receiver does not know
	fateDuplicate = 5 // the burst arrives twice
	fateFlip      = 6 // the burst arrives with bit errors
)

// eventClass is the class of one receive event of a session.
type eventClass uint8

const (
	eventIntact  eventClass = iota // the burst decodes to what was sent
	eventLost                      // Lost, or a block whose FEC failed
	eventUnknown                   // anything else, which the model cannot follow
)

// event is one receive event of a transmission: a burst and the index of
// the burst that was sent, 0 for the header, or a call of Lost.
type event struct {
	raw   layer2.RawBurst
	sent  int
	class eventClass
	lost  bool
}

// session is one fuzzed exchange of a packet between a Builder and an
// Assembler over a channel that loses, duplicates, reorders and damages
// bursts. It keeps a model of the confirmed blocks that the Assembler holds
// while the model can follow the channel. Its storage is allocated once, so
// that a run does not allocate.
type session struct {
	tx      *packet.Builder
	rxb     *packet.Builder
	rx      *packet.Assembler
	ack     *packet.Assembler
	burst   layer2.Burst
	cc      enums.ColorCode
	src     layer2.SyncSource
	rate    enums.DataType
	header  pdu.DataHeader
	payload []byte
	p       packet.Packet
	pad     int

	confirmed bool
	ops       []byte
	air       []layer2.RawBurst
	resp      []layer2.RawBurst
	events    []event
	order     [128]int
	opOf      [128]byte
	missing   []int
	rd        packet.ResponseData
	good      [128]bool
	known     bool
	done      bool

	fail  string
	round int
	at    int
}

// newSession returns a session with storage for packets of 127 blocks.
func newSession() *session {
	return &session{
		tx:      packet.NewBuilder(127),
		rxb:     packet.NewBuilder(0),
		rx:      packet.NewAssembler(127),
		ack:     packet.NewAssembler(2),
		air:     make([]layer2.RawBurst, 0, 128),
		resp:    make([]layer2.RawBurst, 0, 3),
		events:  make([]event, 0, maxEvents),
		missing: make([]int, 0, 127),
	}
}

// failf records the first failure of the session.
func (s *session) failf(msg string) {
	if s.fail == "" {
		s.fail = msg
	}
}

// next returns the next channel op. Past the end of the ops every op is a
// clean delivery.
func (s *session) next() byte {
	if len(s.ops) == 0 {
		return 0
	}
	b := s.ops[0]
	s.ops = s.ops[1:]
	return b
}

// run sends the packet of s until the Assembler delivers it: fuzzRounds
// transmissions over the channel that ops describe, then two clean ones.
func (s *session) run(ops []byte, acceptBadCRC bool) {
	s.rx.Reset()
	s.rx.AcceptBadCRC = acceptBadCRC
	s.ack.Reset()
	s.ops = ops
	s.known, s.done, s.fail = true, false, ""
	clear(s.good[:])
	for s.round = 0; s.round < fuzzRounds+2 && !s.done && s.fail == ""; s.round++ {
		if s.round == fuzzRounds {
			s.ops = nil
		}
		q, ok := s.nextTry()
		if !ok {
			return
		}
		s.transmit(&q)
	}
	if s.fail == "" && !s.done {
		s.failf("packet not delivered by two clean transmissions")
	}
}

// nextTry returns the packet that the sender sends next: the first try,
// the first try again for an unconfirmed packet, the blocks that the SACK
// of the Assembler names, or every block after no response.
func (s *session) nextTry() (packet.Packet, bool) {
	if s.round == 0 || !s.confirmed {
		return s.p, true
	}
	if !s.rx.InProgress() {
		s.known = true
		clear(s.good[:])
	}
	var rd *packet.ResponseData
	switch s.p.Header.Format {
	case pdu.FormatConfirmed:
		rd = s.sack()
	default:
		rd = s.sarq()
	}
	if s.fail != "" {
		return packet.Packet{}, false
	}
	q, err := s.tx.Retry(&s.p, rd)
	if err != nil {
		s.failf("Retry failed")
		return packet.Packet{}, false
	}
	return q, true
}

// sack sends the SACK of the Assembler to the sender over a clean channel
// and returns its response data, or nil when the Assembler has none. It
// checks that the response names the blocks that Missing returns.
func (s *session) sack() *packet.ResponseData {
	resp, err := s.rx.Response()
	if err != nil {
		if !errors.Is(err, packet.ErrFormat) && !errors.Is(err, packet.ErrNoFirstTry) {
			s.failf("Response returned an undocumented error")
		}
		return nil
	}
	m := s.over(&resp)
	if s.fail != "" {
		return nil
	}
	if m.Response == nil {
		s.failf("SACK arrived without response data")
		return nil
	}
	if r, err := packet.ResponseOf(m.Header.ResponsePacketHeader); err != nil || r != packet.ResponseSACK {
		s.failf("response is not a SACK")
		return nil
	}
	c := s.p.Header.ConfirmedDataHeader
	if h := m.Header.ResponsePacketHeader; h.LLIDDestination != c.LLIDSource || h.LLIDSource != c.LLIDDestination ||
		h.SAP != c.SAP || h.ResponseStatus != c.SendSequenceNumber {
		s.failf("SACK header does not answer the packet")
		return nil
	}
	s.missing = s.rx.AppendMissing(s.missing[:0])
	for i := range m.Response.Retry {
		in := slices.Contains(s.missing, i)
		if m.Response.Retry[i] != (i < len(s.p.Blocks) && in) || i < len(s.p.Blocks) && s.known && in == s.good[i] {
			s.failf("SACK flags do not match Missing and the model")
			return nil
		}
	}
	s.rd = *m.Response
	return &s.rd
}

// sarq returns the response data for the blocks that the Assembler misses
// of a SARQ short data packet, which has no SACK packet, or nil when it
// misses none.
func (s *session) sarq() *packet.ResponseData {
	if !s.rx.InProgress() {
		return nil
	}
	s.missing = s.rx.AppendMissing(s.missing[:0])
	if len(s.missing) == 0 {
		s.failf("SARQ packet in progress misses no block")
		return nil
	}
	clear(s.rd.Retry[:])
	for _, i := range s.missing {
		if s.known && s.good[i] {
			s.failf("Missing names a block the model holds")
		}
		s.rd.Retry[i] = true
	}
	return &s.rd
}

// over sends the response packet p over a clean channel to the sender's
// Assembler and returns the Message it delivers.
func (s *session) over(p *packet.Packet) packet.Message {
	var err error
	s.resp, err = packet.AppendBursts(s.resp[:0], p, s.cc, s.src)
	if err != nil {
		s.failf("response bursts do not encode")
		return packet.Message{}
	}
	for i, raw := range s.resp {
		if s.burst.DecodeFromBytes(raw) != nil {
			s.failf("response burst does not decode")
			return packet.Message{}
		}
		m, done, err := s.ack.AddBurst(&s.burst)
		if done != (i == len(s.resp)-1) || err != nil {
			s.failf("response packet not delivered at its last burst")
			return packet.Message{}
		}
		if done {
			return m
		}
	}
	return packet.Message{}
}

// transmit sends q over the fuzzed channel and checks each result of the
// Assembler.
func (s *session) transmit(q *packet.Packet) {
	var err error
	s.air, err = packet.AppendBursts(s.air[:0], q, s.cc, s.src)
	if err != nil {
		s.failf("bursts do not encode")
		return
	}
	accounted := s.plan()
	if s.confirmed {
		accounted = accounted && s.known
		s.known = accounted
	} else if s.rx.InProgress() && !fullFlag(&q.Header) {
		accounted = false
	}
	blocks, inOrder, usable := 0, true, true
	for i, e := range s.events {
		s.at = i
		var m packet.Message
		var done bool
		if e.lost {
			m, done, err = s.rx.Lost()
		} else {
			_ = s.burst.DecodeFromBytes(e.raw)
			m, done, err = s.rx.AddBurst(&s.burst)
		}
		s.checkResult(&m, done, err)
		if s.fail != "" {
			return
		}
		if !accounted {
			if done && err == nil {
				s.finish(&m)
			}
			continue
		}
		if e.sent == 0 {
			if done || err != nil {
				s.failf("header of the attempt returned a result")
			}
			continue
		}
		blocks++
		inOrder = inOrder && e.class == eventIntact && s.air[e.sent] == s.air[blocks]
		usable = usable && e.class == eventIntact
		if s.confirmed {
			s.checkConfirmed(q, e, &m, done, err, blocks == len(q.Blocks))
		} else {
			accept := usable && s.rx.AcceptBadCRC && fullFlag(&q.Header)
			s.checkUnconfirmed(&m, done, err, blocks == len(q.Blocks), inOrder, accept)
		}
		if s.done || s.fail != "" {
			return
		}
	}
}

// checkConfirmed checks the result of a block of a confirmed attempt that
// the model follows. The packet completes at the block that gives the
// model its last DBSN, and an attempt that ends without it returns
// ErrBlocksMissing.
func (s *session) checkConfirmed(q *packet.Packet, e event, m *packet.Message, done bool, err error, last bool) {
	if e.class == eventIntact {
		s.good[dbsnOf(q.Blocks[e.sent-1])] = true
	}
	complete := !slices.Contains(s.good[:len(s.p.Blocks)], false)
	switch {
	case complete:
		if !done || err != nil {
			s.failf("confirmed packet not delivered at its last DBSN")
			return
		}
		s.finish(m)
	case done:
		s.failf("confirmed packet delivered without every DBSN")
	case last && !errors.Is(err, packet.ErrBlocksMissing):
		s.failf("attempt ended without ErrBlocksMissing")
	case !last && err != nil:
		s.failf("block within an attempt returned an error")
	case last:
		s.missing = s.rx.AppendMissing(s.missing[:0])
		for i := range s.p.Blocks {
			if slices.Contains(s.missing, i) == s.good[i] {
				s.failf("Missing does not match the model")
				return
			}
		}
		if !s.rx.InProgress() {
			s.failf("confirmed packet not in progress after ErrBlocksMissing")
		}
	}
}

// checkUnconfirmed checks the result of a block of an unconfirmed attempt
// that the model follows. The packet completes at the last block, without
// an error when every block arrived intact and in order, or, when accept
// reports that AcceptBadCRC applies to blocks that all arrived, with
// BadCRC set when they are out of order.
func (s *session) checkUnconfirmed(m *packet.Message, done bool, err error, last, inOrder, accept bool) {
	switch {
	case done != last:
		s.failf("unconfirmed packet not delivered at its last block")
	case !last:
		if err != nil {
			s.failf("block within an attempt returned an error")
		}
	case (inOrder || accept) != (err == nil) || err == nil && m.BadCRC == inOrder:
		s.failf("unconfirmed result does not match the blocks")
	case err == nil:
		s.finish(m)
	}
}

// finish ends the session on a Message delivered without an error, and for
// a C_HEAD sends the ACK to the sender.
func (s *session) finish(m *packet.Message) {
	s.done = true
	if c := m.Header.ConfirmedDataHeader; c != nil && m.Header.Format == pdu.FormatConfirmed {
		ack, err := s.rxb.NewResponse(c, packet.ResponseACK, nil)
		if err != nil {
			s.failf("ACK does not build")
			return
		}
		r := s.over(&ack)
		if s.fail != "" {
			return
		}
		if code, err := packet.ResponseOf(r.Header.ResponsePacketHeader); err != nil || code != packet.ResponseACK || r.Response != nil {
			s.failf("ACK did not arrive as an ACK")
		}
	}
}

// checkResult checks any result of the Assembler: the documented errors,
// and a Message delivered without an error is the packet that was sent.
func (s *session) checkResult(m *packet.Message, done bool, err error) {
	switch {
	case done && err != nil:
		if !errors.Is(err, packet.ErrMessageCRC) {
			s.failf("Message delivered with an undocumented error")
		}
	case err != nil:
		if !errors.Is(err, packet.ErrNoHeader) && !errors.Is(err, packet.ErrBlocksMissing) &&
			!errors.Is(err, packet.ErrUnexpectedRetry) && !errors.Is(err, packet.ErrHeaderCRC) &&
			!errors.Is(err, packet.ErrRateChange) {
			s.failf("undocumented error")
		}
	case done:
		if !m.BadCRC && !s.samePacket(m) {
			s.failf("Message without an error is not the packet that was sent")
		}
	}
}

// samePacket reports whether m is the packet of s: the same format and
// addresses, and the payload, followed by the pad octets when the header is
// a later try or short data.
func (s *session) samePacket(m *packet.Message) bool {
	if m.Header.Format != s.p.Header.Format || m.DataType != s.rate {
		return false
	}
	gotDst, gotSrc, _ := m.Header.Addresses()
	dst, src, _ := s.p.Header.Addresses()
	if gotDst != dst || gotSrc != src {
		return false
	}
	pad := s.pad
	if fullFlag(&m.Header) && (m.Header.Format == pdu.FormatUnconfirmed || m.Header.Format == pdu.FormatConfirmed) {
		pad = 0
	}
	return len(m.Data) == len(s.payload)+pad && bytes.Equal(m.Data[:len(s.payload)], s.payload) &&
		!slices.ContainsFunc(m.Data[len(s.payload):], func(b byte) bool { return b != 0 })
}

// plan draws the fate of each burst of s.air from the channel ops and
// fills s.events. It reports whether the model can follow the attempt:
// the header arrives first and intact, and every block arrives once,
// intact or as a loss that the Assembler counts.
func (s *session) plan() bool {
	n := len(s.air)
	for i := range n {
		s.order[i] = i
		s.opOf[i] = s.next()
	}
	for i := 0; i+1 < n; i++ {
		if s.opOf[i]&8 != 0 {
			s.order[i], s.order[i+1] = s.order[i+1], s.order[i]
		}
	}
	accounted := s.order[0] == 0
	s.events = s.events[:0]
	for k := range n {
		sent := s.order[k]
		raw := s.air[sent]
		switch s.opOf[k] & 7 {
		case fateLost:
			s.events = append(s.events, event{sent: sent, class: eventLost, lost: true})
			accounted = accounted && sent != 0
			continue
		case fateSilent:
			accounted = false
			continue
		case fateDuplicate:
			accounted = false
			s.events = append(s.events, s.classify(raw, sent))
		case fateFlip:
			for range 1 + int(s.opOf[k]>>4) {
				pos := (int(s.next())<<8 | int(s.next())) % (8 * len(raw))
				raw[pos/8] ^= 0x80 >> (pos % 8)
			}
		}
		e := s.classify(raw, sent)
		accounted = accounted && (e.class == eventIntact || e.class == eventLost && sent != 0)
		s.events = append(s.events, e)
	}
	return accounted
}

// classify decodes raw, a damaged copy of burst sent of s.air, and returns
// its event: intact when it decodes and encodes back to the burst that was
// sent, lost when it is a block of the same rate whose FEC failed, which
// the Assembler counts as a loss, and unknown otherwise.
func (s *session) classify(raw layer2.RawBurst, sent int) event {
	e := event{raw: raw, sent: sent, class: eventUnknown}
	if s.burst.DecodeFromBytes(raw) != nil || s.burst.Kind != layer2.KindData || s.burst.Data == nil {
		return e
	}
	if enc, err := s.burst.Encode(); err == nil && enc == s.air[sent] && !s.burst.FEC.PDU.Uncorrectable {
		e.class = eventIntact
	} else if sent != 0 && s.burst.FEC.PDU.Uncorrectable && s.burst.Data.DataType() == s.rate {
		e.class = eventLost
	}
	return e
}

// dbsnOf returns the DBSN of a confirmed block that Segment built.
func dbsnOf(d layer2.Data) int {
	switch b := d.(type) {
	case *pdu.Rate12Data:
		return int(b.Data[0] >> 1)
	case *pdu.Rate34Data:
		return int(b.Data[0] >> 1)
	case *pdu.Rate1Data:
		return int(b.Data[0] >> 1)
	}
	return 0
}

// fullFlag reports whether h has its Full Message Flag set, or has no such
// flag.
func fullFlag(h *pdu.DataHeader) bool {
	switch {
	case h.UnconfirmedDataHeader != nil:
		return h.UnconfirmedDataHeader.FullMessageFlag
	case h.ConfirmedDataHeader != nil:
		return h.ConfirmedDataHeader.FullMessageFlag
	case h.DefinedDataHeader != nil:
		return h.DefinedDataHeader.FullMessageFlag
	case h.RawDataHeader != nil:
		return h.RawDataHeader.FullMessageFlag
	}
	return true
}

// fuzzHeader returns the first-try header of a fuzzed packet: kind bits 0-1
// select U_HEAD, C_HEAD, DD_HEAD or R_HEAD, bit 4 sets SARQ and bit 5 clears
// the Full Message Flag of a U_HEAD, bits 6 and 7 set Group and Response
// Requested, and fields fills the other fields.
func fuzzHeader(kind uint8, fields uint64, dst, src uint32) pdu.DataHeader {
	dst, src = dst&0xFFFFFF, src&0xFFFFFF
	group, resp := kind&0x40 != 0, kind&0x80 != 0
	sap := enums.ServiceAccessPointID(fields & 0xF)
	switch kind & 3 {
	case 0:
		return pdu.DataHeader{Format: pdu.FormatUnconfirmed, UnconfirmedDataHeader: &pdu.UnconfirmedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			FullMessageFlag: kind&0x20 == 0, FragmentSequenceNumber: uint8(fields>>4) & 0xF}}
	case 1:
		return pdu.DataHeader{Format: pdu.FormatConfirmed, ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			FullMessageFlag: true, ReSynchronizeFlag: fields&0x100 != 0, SendSequenceNumber: uint8(fields>>9) & 7,
			FragmentSequenceNumber: uint8(fields>>4) & 0xF}}
	case 2:
		return pdu.DataHeader{Format: pdu.FormatShortDataDefined, DefinedDataHeader: &pdu.DefinedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			DefinedData: uint8(fields>>12) & 0x3F, SARQ: kind&0x10 != 0, FullMessageFlag: true,
			BitPadding: uint8(fields >> 18)}}
	default:
		return pdu.DataHeader{Format: pdu.FormatShortDataRawOrStatusPrecoded, RawDataHeader: &pdu.RawDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			SourcePort: uint8(fields>>12) & 7, DestinationPort: uint8(fields>>15) & 7, SARQ: kind&0x10 != 0,
			FullMessageFlag: true, BitPadding: uint8(fields >> 18)}}
	}
}

// setup segments the fuzzed packet into s. It returns false when the
// payload needs more blocks than the header can count, which Segment must
// report as ErrTooLong.
func (s *session) setup(t *testing.T, kind uint8, fields uint64, dst, src uint32, payload []byte) bool {
	t.Helper()
	s.rate = [4]enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeRate12}[kind>>2&3]
	s.cc = enums.ColorCode(fields >> 26 & 0xF)
	s.src = layer2.SyncSource(fields >> 30 & 3)
	s.header = fuzzHeader(kind, fields, dst, src)
	s.payload = payload
	s.confirmed = s.header.Format == pdu.FormatConfirmed || kind&0x10 != 0 && kind&3 >= 2
	per, err := packet.BlockOctets(s.rate, s.confirmed)
	if err != nil {
		t.Fatal(err)
	}
	blocks, pad := packet.Layout(len(payload), per)
	limit := 127
	if kind&3 >= 2 {
		limit = 63
	}
	s.p, err = s.tx.Segment(s.header, payload, s.rate)
	if blocks > limit {
		if !errors.Is(err, packet.ErrTooLong) {
			t.Fatalf("Segment of %d blocks: %v, want ErrTooLong", blocks, err)
		}
		return false
	}
	if err != nil || len(s.p.Blocks) != blocks {
		t.Fatalf("Segment: %v, %d blocks, want %d", err, len(s.p.Blocks), blocks)
	}
	s.pad = pad
	return true
}

// roundTripSeed is one seed of FuzzRoundTrip.
type roundTripSeed struct {
	kind     uint8
	fields   uint64
	dst, src uint32
	payload  []byte
	channel  []byte
}

// roundTripSeeds returns the seeds of FuzzRoundTrip: the payloads and
// addresses of the SMS captures, sent unconfirmed, confirmed and as SARQ
// short data at each rate, over clean and damaged channels, and a packet of
// zeros that loses a block, whose message CRC still passes.
func roundTripSeeds(t testing.TB) []roundTripSeed {
	t.Helper()
	var seeds []roundTripSeed
	channels := [][]byte{
		nil,
		{0, 0, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		{0, 8, 0, 5, 6, 0x01, 0x23, 0, 4, 0, 0, 0, 0, 0, 3, 3},
		{0, 0x36, 0x10, 0x10, 0x10, 0x10, 0, 0, 0, 0, 3, 0, 0, 0, 0, 0, 0, 3},
	}
	for i, name := range []string{"d-sms", "h-sms", "m-sms"} {
		data, err := os.ReadFile("../testdata/" + name + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		a := packet.NewAssembler(127)
		var b layer2.Burst
		for j := 0; j+33 <= len(data); j += 33 {
			if b.DecodeFromBytes(layer2.RawBurst(data[j:j+33])) != nil {
				continue
			}
			m, done, err := a.AddBurst(&b)
			if !done || err != nil {
				continue
			}
			dst, src, _ := m.Header.Addresses()
			payload := bytes.Clone(m.Data)
			for k, kind := range []uint8{0, 1 | 4, 1 | 0x80, 2 | 0x10 | 8, 3 | 0x10, 2} {
				seeds = append(seeds, roundTripSeed{
					kind: kind, fields: uint64(i*5+k)<<9 | 0x5, dst: dst, src: src,
					payload: payload, channel: channels[(i+k)%len(channels)],
				})
			}
		}
	}
	return append(seeds, roundTripSeed{dst: seeds[0].dst, src: seeds[0].src, payload: make([]byte, 40), channel: []byte{0, 3}})
}

// FuzzRoundTrip segments a fuzzed packet with a Builder and sends it to an
// Assembler over a channel that loses, duplicates, swaps and damages bursts,
// with selective retry by the SACK of the Assembler. The Assembler must
// never deliver a packet without an error that is not the one sent, must
// end each attempt that the model follows as the package doc says, with
// Missing and the SACK naming the blocks it lacks, and must deliver the
// packet after two clean transmissions.
func FuzzRoundTrip(f *testing.F) {
	for _, s := range roundTripSeeds(f) {
		f.Add(s.kind, s.fields, s.dst, s.src, s.payload, s.channel)
	}
	s := newSession()
	f.Fuzz(func(t *testing.T, kind uint8, fields uint64, dst, src uint32, payload, channel []byte) {
		if !s.setup(t, kind, fields, dst, src, payload) {
			return
		}
		s.run(channel, kind&0x20 != 0 && kind&3 != 0)
		if s.fail != "" {
			t.Fatalf("round %d event %d: %s\nheader %+v rate %v blocks %d", s.round, s.at, s.fail, s.header, s.rate, len(s.p.Blocks))
		}
	})
}

// burstSeeds returns the seeds of FuzzAssemblerBursts: the bursts of each
// SMS capture, each after an op byte that adds it.
func burstSeeds(t testing.TB) [][]byte {
	t.Helper()
	var seeds [][]byte
	for _, name := range []string{"d-sms", "h-sms", "m-sms"} {
		data, err := os.ReadFile("../testdata/" + name + ".bin")
		if err != nil {
			t.Fatal(err)
		}
		var in []byte
		for i := 0; i+33 <= len(data); i += 33 {
			in = append(in, byte(i/33)&0x30)
			in = append(in, data[i:i+33]...)
		}
		seeds = append(seeds, in)
	}
	return append(seeds, builtBursts(t, roundTripSeeds(t)[0].payload))
}

// builtBursts returns a seed of FuzzAssemblerBursts made with the Builder:
// a confirmed packet of data that loses a block, the retry and the SACK,
// then an unconfirmed packet with a P_HEAD second header.
func builtBursts(t testing.TB, data []byte) []byte {
	t.Helper()
	b := packet.NewBuilder(127)
	p, err := b.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	var raws []layer2.RawBurst
	add := func(p packet.Packet) {
		out, err := packet.Bursts(p, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		raws = append(raws, out...)
	}
	add(p)
	var rd packet.ResponseData
	rd.Retry[1] = true
	sack, err := b.NewResponse(p.Header.ConfirmedDataHeader, packet.ResponseSACK, &rd)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := b.Retry(&p, &rd)
	if err != nil {
		t.Fatal(err)
	}
	add(retry)
	add(sack)
	h := unconfirmed()
	h.UnconfirmedDataHeader.SAP = enums.SAPProprietaryPacketData
	u, err := b.Segment(h, data[:20], enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	head := *u.Header.UnconfirmedDataHeader
	head.BlocksToFollow++
	u.Header.UnconfirmedDataHeader = &head
	for _, d := range append([]layer2.Data{&u.Header, &pdu.DataHeader{Format: pdu.FormatProprietary,
		ProprietaryHeader: &pdu.ProprietaryHeader{SAP: 9, MFID: 0x10}}}, u.Blocks...) {
		raw, err := layer2.BuildBurst(d, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		raws = append(raws, raw)
	}
	var in []byte
	for i, raw := range raws {
		op := byte(0)
		if i == 2 {
			op = 0x40
		}
		in = append(in, op)
		if op == 0 {
			in = append(in, raw[:]...)
		}
	}
	return in
}

// anyBursts is the reused state of FuzzAssemblerBursts.
type anyBursts struct {
	a       *packet.Assembler
	b       layer2.Burst
	missing []int
}

// newAnyBursts returns the state of FuzzAssemblerBursts.
func newAnyBursts() *anyBursts {
	return &anyBursts{a: packet.NewAssembler(127), missing: make([]int, 0, 127)}
}

// feed runs the ops of in on a reset Assembler: an op byte with the top
// bits 01 calls Lost, 10 calls Reset, and any other adds the 33-byte burst
// that follows it. After each op it checks the result, Missing and the
// SACK.
func (r *anyBursts) feed(t *testing.T, acceptBadCRC bool, in []byte) {
	t.Helper()
	a := r.a
	a.Reset()
	a.AcceptBadCRC = acceptBadCRC
	for len(in) > 0 {
		op := in[0]
		in = in[1:]
		var m packet.Message
		var done bool
		var err error
		switch {
		case op&0xC0 == 0x40:
			m, done, err = a.Lost()
		case op&0xC0 == 0x80:
			a.Reset()
		case len(in) >= 33:
			_ = r.b.DecodeFromBytes(layer2.RawBurst(in[:33]))
			in = in[33:]
			m, done, err = a.AddBurst(&r.b)
		default:
			in = nil
		}
		checkAnyResult(t, &m, done, err)
		r.missing = a.AppendMissing(r.missing[:0])
		if !slices.IsSorted(r.missing) || len(r.missing) > 0 && (r.missing[len(r.missing)-1] > 126 || !a.InProgress()) {
			t.Fatalf("Missing %v, in progress %v", r.missing, a.InProgress())
		}
		checkSACK(t, a, r.missing)
	}
}

// FuzzAssemblerBursts feeds an Assembler arbitrary bursts, losses and
// resets. It must never panic, return only the documented errors, keep
// Missing in order and within the packet, and build a SACK that names the
// blocks Missing returns.
func FuzzAssemblerBursts(f *testing.F) {
	for _, in := range burstSeeds(f) {
		f.Add(false, in)
		f.Add(true, in)
	}
	r := newAnyBursts()
	f.Fuzz(func(t *testing.T, acceptBadCRC bool, in []byte) {
		r.feed(t, acceptBadCRC, in)
	})
}

// checkAnyResult checks a result of an Assembler on arbitrary input: only
// the documented errors, and a Message within the limits of a packet.
func checkAnyResult(t *testing.T, m *packet.Message, done bool, err error) {
	t.Helper()
	if !done {
		if err != nil && !errors.Is(err, packet.ErrNoHeader) && !errors.Is(err, packet.ErrBlocksMissing) &&
			!errors.Is(err, packet.ErrUnexpectedRetry) && !errors.Is(err, packet.ErrHeaderCRC) &&
			!errors.Is(err, packet.ErrRateChange) {
			t.Fatalf("undocumented error %v", err)
		}
		return
	}
	if err != nil && !errors.Is(err, packet.ErrMessageCRC) && !errors.Is(err, packet.ErrResponseData) {
		t.Fatalf("Message with undocumented error %v", err)
	}
	switch m.DataType {
	case 0, enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1:
	default:
		t.Fatalf("Message data type %v", m.DataType)
	}
	if len(m.Data) > 127*24 || m.Header.Format == pdu.FormatResponsePacket && err == nil && m.DataType != 0 && m.Response == nil {
		t.Fatalf("Message %+v", m)
	}
}

// checkSACK checks that the SACK of the Assembler, when it has one, encodes
// and names the blocks in missing.
func checkSACK(t *testing.T, a *packet.Assembler, missing []int) {
	t.Helper()
	resp, err := a.Response()
	if err != nil {
		return
	}
	if _, err := packet.Bursts(resp, 0, layer2.SyncSourceBS); err != nil {
		t.Fatalf("SACK does not encode: %v", err)
	}
	blocks := make([]*pdu.Rate12Data, len(resp.Blocks))
	for i, d := range resp.Blocks {
		r, ok := d.(*pdu.Rate12Data)
		if !ok {
			t.Fatalf("SACK block %T", d)
		}
		blocks[i] = r
	}
	rd, err := packet.DecodeResponseData(blocks...)
	if err != nil {
		t.Fatalf("SACK data: %v", err)
	}
	for i := range rd.Retry {
		if rd.Retry[i] != slices.Contains(missing, i) {
			t.Fatalf("SACK flag %d is %v, Missing %v", i, rd.Retry[i], missing)
		}
	}
}

// responseDataSeed is one seed of FuzzResponseData.
type responseDataSeed struct {
	b0, b1 []byte
	blocks uint8
}

// responseDataSeeds returns the seeds of FuzzResponseData: C_RDATA blocks
// that retry nothing, one block and the first and last of 64.
func responseDataSeeds() []responseDataSeed {
	full := bytes.Repeat([]byte{0xFF}, 12)
	return []responseDataSeed{
		{full, nil, 10},
		{full, full, 100},
		{[]byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0, 0, 0, 0}, nil, 64},
	}
}

// checkResponseData decodes the C_RDATA blocks b0 and b1, when b1 is not
// empty, and checks that the flags survive an encode for a packet of
// blocks blocks. Each block holds the first 12 octets of its input, zero
// padded.
func checkResponseData(t *testing.T, b0, b1 []byte, blocks uint8) {
	t.Helper()
	in := []*pdu.Rate12Data{{}}
	copy(in[0].Data[:], b0)
	if len(b1) > 0 {
		in = append(in, &pdu.Rate12Data{})
		copy(in[1].Data[:], b1)
	}
	rd, err := packet.DecodeResponseData(in...)
	if err != nil && !errors.Is(err, packet.ErrResponseData) {
		t.Fatalf("DecodeResponseData: %v", err)
	}
	n := int(blocks)
	enc, err := rd.AppendEncode(nil, n)
	if n > 127 {
		if !errors.Is(err, packet.ErrTooLong) {
			t.Fatalf("AppendEncode of %d blocks: %v", n, err)
		}
		return
	}
	if err != nil || len(enc) != 1+n/65 {
		t.Fatalf("AppendEncode of %d blocks: %d blocks, %v", n, len(enc), err)
	}
	out := make([]*pdu.Rate12Data, len(enc))
	for i := range enc {
		out[i] = &enc[i]
	}
	back, err := packet.DecodeResponseData(out...)
	if err != nil {
		t.Fatalf("decode of encoded flags: %v", err)
	}
	for i := range back.Retry {
		if back.Retry[i] != (i < n && rd.Retry[i]) {
			t.Fatalf("flag %d is %v, want %v", i, back.Retry[i], i < n && rd.Retry[i])
		}
	}
	want := rd.RetryBlocks()
	if got := rd.AppendRetryBlocks(nil); !slices.Equal(got, want) {
		t.Fatalf("AppendRetryBlocks %v, RetryBlocks %v", got, want)
	}
}

// FuzzResponseData decodes fuzzed C_RDATA blocks and encodes the flags for a
// fuzzed packet size. The flags of the blocks of the packet must survive,
// the rest must read as received, and the CRC must pass.
func FuzzResponseData(f *testing.F) {
	for _, s := range responseDataSeeds() {
		f.Add(s.b0, s.b1, s.blocks)
	}
	f.Fuzz(checkResponseData)
}
