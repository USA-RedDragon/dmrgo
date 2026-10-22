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

const fuzzRounds = 8

const maxEvents = 2 * 128

const (
	fateLost      = 3
	fateSilent    = 4
	fateDuplicate = 5
	fateFlip      = 6
)

type eventClass uint8

const (
	eventIntact eventClass = iota
	eventLost
	eventUnknown
)

type event struct {
	raw   layer2.RawBurst
	sent  int
	class eventClass
	lost  bool
}

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
	selective bool
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
	altered   bool
	octets    []byte

	fail  string
	round int
	at    int
}

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
		octets:  make([]byte, 0, 127*24),
	}
}

func (s *session) failf(msg string) {
	if s.fail == "" {
		s.fail = msg
	}
}

func (s *session) next() byte {
	if len(s.ops) == 0 {
		return 0
	}
	b := s.ops[0]
	s.ops = s.ops[1:]
	return b
}

func (s *session) run(ops []byte, acceptBadCRC bool) {
	s.rx.Reset()
	s.rx.AcceptBadCRC = acceptBadCRC
	s.ack.Reset()
	s.ops = ops
	s.known, s.done, s.altered, s.fail = true, false, false, ""
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

func (s *session) nextTry() (packet.Packet, bool) {
	if s.round == 0 || !s.confirmed {
		return s.p, true
	}
	if !s.selective {
		s.known = true
		clear(s.good[:])
		return s.p, true
	}
	if !s.rx.InProgress() {
		s.known = true
		clear(s.good[:])
	}
	var rd *packet.ResponseData
	switch s.p.Header.Format() {
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
	h, ok := m.Header.AsResponse()
	if r, err := packet.ResponseOf(&h); !ok || err != nil || r != packet.ResponseSACK {
		s.failf("response is not a SACK")
		return nil
	}
	c, ok := s.p.Header.AsConfirmed()
	if !ok || h.LLIDDestination != c.LLIDSource || h.LLIDSource != c.LLIDDestination ||
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

func (s *session) finish(m *packet.Message) {
	s.done = true
	if c, ok := m.Header.AsConfirmed(); ok {
		ack, err := s.rxb.NewResponse(&c, packet.ResponseACK, nil)
		if err != nil {
			s.failf("ACK does not build")
			return
		}
		r := s.over(&ack)
		if s.fail != "" {
			return
		}
		rh, ok := r.Header.AsResponse()
		if code, err := packet.ResponseOf(&rh); !ok || err != nil || code != packet.ResponseACK || r.Response != nil {
			s.failf("ACK did not arrive as an ACK")
		}
	}
}

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
		if !m.BadCRC && !s.samePacket(m) && (!s.altered || !s.wellFormed(m)) {
			s.failf("Message without an error is not the packet that was sent")
		}
	}
}

func (s *session) samePacket(m *packet.Message) bool {
	if m.Header.Format() != s.p.Header.Format() || m.DataType != s.rate {
		return false
	}
	gotDst, gotSrc, _ := m.Header.Addresses()
	dst, src, _ := s.p.Header.Addresses()
	if gotDst != dst || gotSrc != src {
		return false
	}
	pad := s.pad
	switch f := m.Header.Format(); {
	case f == pdu.FormatShortDataDefined || f == pdu.FormatShortDataRawOrStatusPrecoded,
		fullFlag(&m.Header) && (f == pdu.FormatUnconfirmed || f == pdu.FormatConfirmed):
		pad = 0
	}
	return len(m.Data) == len(s.payload)+pad && bytes.Equal(m.Data[:len(s.payload)], s.payload) &&
		!slices.ContainsFunc(m.Data[len(s.payload):], func(b byte) bool { return b != 0 })
}

func (s *session) wellFormed(m *packet.Message) bool {
	if m.Header.Format() != s.p.Header.Format() {
		return false
	}
	per, err := packet.BlockOctets(m.DataType, s.confirmed)
	if err != nil {
		return false
	}
	pad := 0
	h := m.Header
	if d, ok := h.AsDefinedData(); ok {
		pad = int(d.BitPadding / 8)
	} else if r, ok := h.AsRawData(); ok {
		pad = int(r.BitPadding / 8)
	} else if fullFlag(&h) {
		if u, ok := h.AsUnconfirmed(); ok {
			pad = int(u.PadOctetCount)
		} else if c, ok := h.AsConfirmed(); ok {
			pad = int(c.PadOctetCount)
		}
	}
	s.octets = append(s.octets[:0], m.Data...)
	for range pad {
		s.octets = append(s.octets, 0)
	}
	return len(s.octets)+4 == blocksOf(&m.Header)*per && packet.MessageCRC(s.octets) == m.CRC
}

func blocksOf(h *pdu.DataHeader) int {
	n, _ := h.BlocksToFollow()
	return n
}

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
		s.altered = s.altered || e.class == eventUnknown
		accounted = accounted && (e.class == eventIntact || e.class == eventLost && sent != 0)
		s.events = append(s.events, e)
	}
	return accounted
}

func (s *session) classify(raw layer2.RawBurst, sent int) event {
	e := event{raw: raw, sent: sent, class: eventUnknown}
	if s.burst.DecodeFromBytes(raw) != nil || s.burst.Kind != layer2.KindData || s.burst.Data == nil {
		return e
	}
	if enc, err := s.burst.Encode(); err == nil && enc == s.air[sent] && !s.burst.FEC.PDU.Uncorrectable && !s.burst.FEC.Payload.Uncorrectable {
		e.class = eventIntact
	} else if sent != 0 && (s.burst.FEC.PDU.Uncorrectable || s.burst.FEC.Payload.Uncorrectable) && s.burst.Data.DataType() == s.rate {
		e.class = eventLost
	}
	return e
}

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

func fullFlag(h *pdu.DataHeader) bool {
	if u, ok := h.AsUnconfirmed(); ok {
		return u.FullMessageFlag
	}
	if c, ok := h.AsConfirmed(); ok {
		return c.FullMessageFlag
	}
	if d, ok := h.AsDefinedData(); ok {
		return d.FullMessageFlag
	}
	if r, ok := h.AsRawData(); ok {
		return r.FullMessageFlag
	}
	return true
}

func fuzzHeader(t *testing.T, kind uint8, fields uint64, dst, src uint32) pdu.DataHeader {
	t.Helper()
	dst, src = dst&0xFFFFFF, src&0xFFFFFF
	group, resp := kind&0x40 != 0, kind&0x80 != 0
	sap := enums.SAP(fields & 0xF)
	switch kind & 3 {
	case 0:
		return header(t, (*pdu.DataHeader).SetUnconfirmed, pdu.UnconfirmedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			FullMessageFlag: kind&0x20 == 0, FragmentSequenceNumber: uint8(fields>>4) & 0xF})
	case 1:
		return header(t, (*pdu.DataHeader).SetConfirmed, pdu.ConfirmedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			FullMessageFlag: true, ReSynchronizeFlag: fields&0x100 != 0, SendSequenceNumber: uint8(fields>>9) & 7,
			FragmentSequenceNumber: uint8(fields>>4) & 0xF})
	case 2:
		return header(t, (*pdu.DataHeader).SetDefinedData, pdu.DefinedDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, LLIDDestination: dst, LLIDSource: src,
			DefinedData: enums.DDFormat(fields>>12) & 0x3F, SARQ: kind&0x10 != 0, FullMessageFlag: true,
			BitPadding: uint8(fields >> 18)})
	default:
		return header(t, (*pdu.DataHeader).SetRawData, pdu.RawDataHeader{
			Group: group, ResponseRequested: resp, SAP: sap, AppendedBlocks: 1, LLIDDestination: dst, LLIDSource: src,
			SourcePort: uint8(fields>>12) & 7, DestinationPort: uint8(fields>>15) & 7, SARQ: kind&0x10 != 0,
			FullMessageFlag: true, BitPadding: uint8(fields >> 18)})
	}
}

func (s *session) setup(t *testing.T, kind uint8, fields uint64, dst, src uint32, payload []byte) bool {
	t.Helper()
	s.rate = [4]enums.DataType{enums.DataTypeRate12, enums.DataTypeRate34, enums.DataTypeRate1, enums.DataTypeRate12}[kind>>2&3]
	s.cc = enums.ColorCode(fields >> 26 & 0xF)
	s.src = layer2.SyncSource(fields >> 30 & 3)
	s.header = fuzzHeader(t, kind, fields, dst, src)
	s.payload = payload
	s.selective = s.header.Format() == pdu.FormatConfirmed || kind&0x10 != 0 && kind&3 >= 2
	s.confirmed = s.selective || kind&0x80 != 0 && kind&3 >= 2
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

type roundTripSeed struct {
	kind     uint8
	fields   uint64
	dst, src uint32
	payload  []byte
	channel  []byte
}

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
			for k, kind := range []uint8{0, 1 | 4, 1 | 0x80, 2 | 0x10 | 8, 3 | 0x10, 2, 2 | 0x80 | 4, 3 | 0x80} {
				seeds = append(seeds, roundTripSeed{
					kind: kind, fields: uint64(i*5+k)<<9 | 0x5, dst: dst, src: src,
					payload: payload, channel: channels[(i+k)%len(channels)],
				})
			}
		}
	}
	return append(seeds, roundTripSeed{dst: seeds[0].dst, src: seeds[0].src, payload: make([]byte, 40), channel: []byte{0, 3}})
}

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

func builtBursts(t testing.TB, data []byte) []byte {
	t.Helper()
	b := packet.NewBuilder(127)
	p, err := b.Segment(confirmed(), data, enums.DataTypeRate34)
	if err != nil {
		t.Fatal(err)
	}
	var raws []layer2.RawBurst
	add := func(p packet.Packet) {
		out, err := packet.Bursts(&p, 1, layer2.SyncSourceMS)
		if err != nil {
			t.Fatal(err)
		}
		raws = append(raws, out...)
	}
	add(p)
	var rd packet.ResponseData
	rd.Retry[1] = true
	to := get(t, &p.Header, (*pdu.DataHeader).AsConfirmed)
	sack, err := b.NewResponse(&to, packet.ResponseSACK, &rd)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := b.Retry(&p, &rd)
	if err != nil {
		t.Fatal(err)
	}
	add(retry)
	add(sack)
	uh := unconfirmedHeader()
	uh.SAP = enums.SAPProprietaryPacketData
	u, err := b.Segment(header(t, (*pdu.DataHeader).SetUnconfirmed, uh), data[:20], enums.DataTypeRate12)
	if err != nil {
		t.Fatal(err)
	}
	head := get(t, &u.Header, (*pdu.DataHeader).AsUnconfirmed)
	head.BlocksToFollow++
	mustSet(t, u.Header.SetUnconfirmed(head))
	prop := header(t, (*pdu.DataHeader).SetProprietary, pdu.ProprietaryHeader{SAP: 9, MFID: 0x10})
	for _, d := range append([]layer2.Data{&u.Header, &prop}, u.Blocks...) {
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

type anyBursts struct {
	a       *packet.Assembler
	b       layer2.Burst
	missing []int
}

func newAnyBursts() *anyBursts {
	return &anyBursts{a: packet.NewAssembler(127), missing: make([]int, 0, 127)}
}

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
	if len(m.Data) > 127*24 || m.Header.Format() == pdu.FormatResponsePacket && err == nil && m.DataType != 0 && m.Response == nil {
		t.Fatalf("Message %+v", m)
	}
}

func checkSACK(t *testing.T, a *packet.Assembler, missing []int) {
	t.Helper()
	resp, err := a.Response()
	if err != nil {
		return
	}
	if _, err := packet.Bursts(&resp, 0, layer2.SyncSourceBS); err != nil {
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

type responseDataSeed struct {
	b0, b1 []byte
	blocks uint8
}

func responseDataSeeds() []responseDataSeed {
	full := bytes.Repeat([]byte{0xFF}, 12)
	return []responseDataSeed{
		{full, nil, 10},
		{full, full, 100},
		{[]byte{0xFE, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x7F, 0, 0, 0, 0}, nil, 64},
	}
}

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

func FuzzResponseData(f *testing.F) {
	for _, s := range responseDataSeeds() {
		f.Add(s.b0, s.b1, s.blocks)
	}
	f.Fuzz(checkResponseData)
}
