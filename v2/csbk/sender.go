package csbk

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/pcg"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

type sendState uint8

const (
	sendIdle sendState = iota
	sendQualify
	sendHoldoff
	sendBursts
)

type sendEvent uint8

const (
	sendNothing sendEvent = iota
	sendSent
	sendDenied
)

const maxTimerSteps = 8

type sender struct {
	cc         enums.ColorCode
	src        layer2.SyncSource
	idleSearch time.Duration
	holdoffMax time.Duration
	rng        pcg.Source
	busy       bool

	state        sendState
	access       enums.Access
	timeCritical bool
	csbk         pdu.CSBKBytes
	target       uint32
	source       uint32
	group        bool
	preambles    int
	sent         int
	timer        time.Duration
	due          time.Duration

	pre  pdu.CSBK
	preP pdu.PreamblePDU
}

func (s *sender) init(cc enums.ColorCode, src layer2.SyncSource, idleSearch, holdoffMax time.Duration, seed uint64) {
	s.cc, s.src, s.rng = cc, src, seedPCG(seed)
	s.idleSearch, s.holdoffMax = idleSearch, holdoffMax
	if s.idleSearch == 0 {
		s.idleSearch = DefaultIdleSearch
	}
	if s.holdoffMax == 0 {
		s.holdoffMax = DefaultHoldoffMax
	}
}

func (s *sender) start(now time.Duration, c pdu.CSBKBytes, target, source uint32, group bool, preambles int) {
	s.csbk, s.target, s.source, s.group, s.preambles, s.sent = c, target, source, group, preambles, 0
	s.qualify(now, now)
}

func (s *sender) qualify(at, now time.Duration) {
	if s.access == enums.AccessImpolite || !s.busy {
		s.state, s.due = sendBursts, now
		return
	}
	s.state, s.timer = sendQualify, at
	if s.idleSearch > 0 {
		s.timer += s.idleSearch
	}
}

func (s *sender) stop() {
	s.state = sendIdle
}

func (s *sender) channel(now time.Duration, busy bool) {
	s.busy = busy
	if s.state == sendQualify && !busy {
		s.state, s.due = sendBursts, now
	}
}

func (s *sender) deadline() (time.Duration, bool) {
	switch s.state {
	case sendQualify, sendHoldoff:
		return s.timer, true
	case sendBursts:
		return s.due, true
	case sendIdle:
	}
	return 0, false
}

func (s *sender) tick(now time.Duration, out []Output) ([]Output, sendEvent) {
	for range maxTimerSteps {
		if (s.state != sendQualify && s.state != sendHoldoff) || now < s.timer {
			break
		}
		switch {
		case s.state == sendHoldoff:
			s.qualify(s.timer, now)
		case s.timeCritical:
			s.state = sendIdle
			return out, sendDenied
		default:
			s.state = sendHoldoff
			s.timer += holdoff(&s.rng, s.holdoffMax)
		}
	}
	if s.state != sendBursts || now < s.due {
		return out, sendNothing
	}
	c := s.csbk
	if s.sent < s.preambles {
		c = s.preamble(s.preambles - s.sent)
	}
	b, err := layer2.BuildDataBurst(c, enums.DataTypeCSBK, s.cc, s.src)
	if err != nil {
		s.state = sendIdle
		return out, sendDenied
	}
	var o *Output
	out, o = appendOutput(out, OutputBurst)
	o.Burst, o.Due = b, s.due
	s.sent++
	if s.sent > s.preambles {
		s.state = sendIdle
		return out, sendSent
	}
	s.due += BurstInterval
	return out, sendNothing
}

func (s *sender) preamble(cbf int) pdu.CSBKBytes {
	s.preP = pdu.PreamblePDU{Group: s.group, CSBKBlocksToFollow: byte(cbf & 0xFF), TargetAddress: s.target, SourceAddress: s.source}
	s.pre = pdu.CSBK{LastBlock: true, Opcode: pdu.CSBKPreamble, PreamblePDU: &s.preP}
	octets, _ := s.pre.Encode()
	return pdu.CSBKBytes(octets)
}
