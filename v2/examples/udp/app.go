package main

import (
	"encoding/binary"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/data"
)

const (
	kindForm = 1
	kindAck  = 2

	ackWait     = 10 * time.Second
	maxAttempts = 3
	seenSize    = 64
)

type appStats struct {
	submitted, acked, failed, delivered, copies, resends, bad int
	latencies                                                 []time.Duration
}

type outForm struct {
	id       uint16
	msg      []byte
	start    time.Duration
	attempts int
	ackAt    time.Duration
	waiting  bool
}

// station is the application of one box: it sends each filled form with a
// form ID until the far station acknowledges that ID, and delivers each ID
// once. A DMR ACK covers one hop, and the IP bearer service delivers a copy
// again after a lost response (ETSI TS 102 361-3 clause 5.4.1.3), so the
// application needs both.
type station struct {
	l      *link
	peer   uint32
	codec  *codec
	tmpl   *template
	enc    encoding
	nextID uint16
	cur    *outForm
	seen   [seenSize]uint16
	nseen  int
	got    []form
	stats  appStats
}

func (s *station) submit(now time.Duration, f *form) {
	msg := []byte{kindForm<<4 | byte(s.enc), 0, 0, s.tmpl.id, s.tmpl.version}
	binary.BigEndian.PutUint16(msg[1:], s.nextID)
	msg = s.codec.encode(msg, s.enc, f)
	s.cur = &outForm{id: s.nextID, msg: msg, start: now, attempts: 1}
	s.nextID++
	s.stats.submitted++
	s.l.send(now, s.peer, msg, int(s.cur.id))
}

func (s *station) idle() bool {
	return s.cur == nil
}

func (s *station) deadline() (time.Duration, bool) {
	if s.cur != nil && s.cur.waiting {
		return s.cur.ackAt, true
	}
	return 0, false
}

func (s *station) tick(now time.Duration) {
	if s.cur != nil && s.cur.waiting && now >= s.cur.ackAt {
		s.retry(now)
	}
}

func (s *station) retry(now time.Duration) {
	c := s.cur
	c.waiting = false
	if c.attempts >= maxAttempts {
		s.stats.failed++
		s.cur = nil
		return
	}
	c.attempts++
	s.stats.resends++
	s.l.send(now, s.peer, c.msg, int(c.id))
}

func (s *station) sent(now time.Duration, tag int, r data.Result) {
	c := s.cur
	if c == nil || tag != int(c.id) || c.waiting {
		return
	}
	if r != data.ResultDelivered {
		s.retry(now)
		return
	}
	c.waiting, c.ackAt = true, now+ackWait
}

func (s *station) datagram(now time.Duration, src uint32, p []byte) {
	if len(p) < 3 || src != s.peer {
		s.stats.bad++
		return
	}
	id := binary.BigEndian.Uint16(p[1:])
	switch p[0] >> 4 {
	case kindAck:
		if s.cur != nil && s.cur.id == id {
			s.stats.acked++
			s.stats.latencies = append(s.stats.latencies, now-s.cur.start)
			s.cur = nil
		}
	case kindForm:
		s.form(now, id, p)
	default:
		s.stats.bad++
	}
}

func (s *station) form(now time.Duration, id uint16, p []byte) {
	if len(p) < 5 || p[3] != s.tmpl.id || p[4] != s.tmpl.version {
		s.stats.bad++
		return
	}
	if !s.saw(id) {
		f, err := s.codec.decode(encoding(p[0]&0x0f), s.tmpl, p[5:])
		if err != nil {
			s.stats.bad++
			return
		}
		s.seen[s.nseen%seenSize] = id
		s.nseen++
		s.got = append(s.got, f)
		s.stats.delivered++
	} else {
		s.stats.copies++
	}
	ack := []byte{kindAck << 4, 0, 0}
	binary.BigEndian.PutUint16(ack[1:], id)
	s.l.send(now, s.peer, ack, -1)
}

func (s *station) saw(id uint16) bool {
	for i := range min(s.nseen, seenSize) {
		if s.seen[i] == id {
			return true
		}
	}
	return false
}
