package session

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

// PeerConfig configures NewPeer.
type PeerConfig struct {
	// Dialect is dmrd.DialectMMDVM or dmrd.DialectHBRP.
	Dialect dmrd.Dialect
	// ID is the repeater ID.
	ID uint32
	// Password is the password of the master. The Peer copies it.
	Password []byte
	// Config is the RPTC message: a dmrd.Config (302 octets) for
	// DialectMMDVM, a dmrd.HBRPConfig (306 octets) for DialectHBRP. The
	// Peer copies it.
	Config []byte
	// Options is the RPTO options text, at most MaxOptions octets, which
	// the Peer sends after the RPTC is acknowledged. Empty sends no RPTO.
	// DialectHBRP has no RPTO.
	Options []byte
	// Retry is the time between two sends of a login step, between two
	// pings, and before a new login after the session ended. 0 means
	// RetryDefault.
	Retry time.Duration
	// Timeout is the time without an acknowledgement or a pong after which
	// the Peer ends the session and logs in again. 0 means TimeoutDefault.
	Timeout time.Duration
	// Backoff is the longest wait before a new login. Each session that
	// ends before the login completes doubles the wait from Retry up to
	// Backoff. 0, or a value up to Retry, waits Retry each time, as
	// DMRGateway does.
	Backoff time.Duration
}

// Peer is the repeater side of a session of the MMDVM network protocol or
// of HBRP, as a repeater, a hotspot or DMRGateway runs it. Make it with
// NewPeer; the zero Peer does nothing. The first Tick sends the RPTL. A
// Peer is not safe for concurrent use and must not be copied.
type Peer struct {
	dialect dmrd.Dialect
	id      uint32
	pass    []byte
	config  []byte
	options []byte
	retry   time.Duration
	timeout time.Duration
	backoff time.Duration

	tx        []byte
	state     State
	started   bool
	stopped   bool
	retryAt   time.Duration
	timeoutAt time.Duration
	waitAt    time.Duration
	salt      uint32
	saltText  [8]byte
	fails     uint8
	noCopy    noCopy
}

// NewPeer returns a Peer for cfg. It returns a *ConfigError for a Dialect
// other than DialectMMDVM or DialectHBRP, a Config that is not an RPTC of
// the dialect for ID, Options in DialectHBRP or longer than MaxOptions,
// or a negative timer. It allocates the Peer and its copies of the
// config.
func NewPeer(cfg *PeerConfig) (*Peer, error) {
	if cfg == nil {
		return nil, errDialect
	}
	switch cfg.Dialect {
	case dmrd.DialectMMDVM:
		c, err := dmrd.ConfigOf(cfg.Config)
		if err != nil || c.ID() != cfg.ID {
			return nil, errConfig
		}
		if len(cfg.Options) > MaxOptions {
			return nil, errOptions
		}
	case dmrd.DialectHBRP:
		c, err := dmrd.HBRPConfigOf(cfg.Config)
		if err != nil {
			return nil, errConfig
		}
		if id, err := c.ID(); err != nil || id != cfg.ID {
			return nil, errConfig
		}
		if len(cfg.Options) != 0 {
			return nil, errOptions
		}
	case dmrd.DialectUnknown, dmrd.DialectLocal:
		return nil, errDialect
	default:
		return nil, errDialect
	}
	if cfg.Retry < 0 || cfg.Timeout < 0 || cfg.Backoff < 0 {
		return nil, errTimer
	}
	p := &Peer{
		dialect: cfg.Dialect,
		id:      cfg.ID,
		pass:    append([]byte(nil), cfg.Password...),
		config:  append([]byte(nil), cfg.Config...),
		options: append([]byte(nil), cfg.Options...),
		retry:   orDefault(cfg.Retry, RetryDefault),
		timeout: orDefault(cfg.Timeout, TimeoutDefault),
		backoff: cfg.Backoff,
		tx:      make([]byte, 0, MaxPacket),
	}
	return p, nil
}

// State returns the state of the Peer.
func (p *Peer) State() State {
	return p.state
}

// Reset ends the session without a packet and makes the Peer as
// NewPeer returned it, so the next Tick sends the RPTL.
func (p *Peer) Reset() {
	p.state, p.started, p.stopped, p.fails, p.salt = StateIdle, false, false, 0, 0
}

// Close ends the session: it sends RPTCL when the Peer is logged in, as
// DMRGateway does when it closes, and reports OutputDown with
// ReasonStopped. The Peer then does nothing until Reset. It allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Peer) Close(_ time.Duration, dst []Output) []Output {
	if p.stopped {
		return dst
	}
	if p.state == StateRunning {
		dst = send(dst, appendControl(p.tx[:0], p.dialect, dmrd.Control{Command: dmrd.CommandRPTCL, ID: p.id}))
	}
	if p.started && p.state != StateWait {
		dst = append(dst, Output{Kind: OutputDown, Reason: ReasonStopped, State: p.state})
	}
	p.state, p.stopped = StateIdle, true
	return dst
}

// Deadline returns the time at which the Peer needs a Tick. ok is false
// after Close, and for a Peer that NewPeer did not make.
func (p *Peer) Deadline() (at time.Duration, ok bool) {
	if p.stopped || p.retry == 0 {
		return 0, false
	}
	if !p.started {
		return 0, true
	}
	if p.state == StateWait {
		return p.waitAt, true
	}
	return min(p.retryAt, p.timeoutAt), true
}

// Tick runs the timers that are due at now: the first RPTL, the sends
// again of the login step, the pings, the timeout and the new login. It
// appends the outputs to dst, and allocates only when dst has no room
// for MaxOutputs outputs.
func (p *Peer) Tick(now time.Duration, dst []Output) []Output {
	for range 2 {
		at, ok := p.Deadline()
		if !ok || now < at {
			return dst
		}
		switch {
		case !p.started, p.state == StateWait:
			p.started = true
			return p.login(now, dst)
		case now >= p.timeoutAt:
			dst = append(dst, Output{Kind: OutputDown, Reason: ReasonTimeout, State: p.state})
			p.wait(now)
		default:
			p.retryAt = now + p.retry
			return p.send(dst)
		}
	}
	return dst
}

func (p *Peer) login(now time.Duration, dst []Output) []Output {
	p.state = StateLogin
	p.retryAt, p.timeoutAt = now+p.retry, now+p.timeout
	return p.send(dst)
}

func (p *Peer) wait(now time.Duration) {
	d := p.retry
	for i := uint8(0); i < p.fails && d < p.backoff; i++ {
		d *= 2
	}
	if p.backoff > p.retry {
		d = min(d, p.backoff)
	}
	if p.fails < 63 {
		p.fails++
	}
	p.state, p.waitAt = StateWait, now+d
}

func (p *Peer) send(dst []Output) []Output {
	switch p.state {
	case StateLogin:
		return send(dst, appendControl(p.tx[:0], p.dialect, dmrd.Control{Command: dmrd.CommandRPTL, ID: p.id}))
	case StateAuth:
		var tok [32]byte
		if p.dialect == dmrd.DialectHBRP {
			var h digest.SHA256
			h.Reset()
			h.Write(p.saltText[:])
			h.Write(p.pass)
			tok = h.Sum()
		} else if t, err := auth.Token(p.dialect, p.salt, p.pass); err == nil {
			tok = t
		}
		return send(dst, appendControl(p.tx[:0], p.dialect, dmrd.Control{Command: dmrd.CommandRPTK, ID: p.id, Token: tok}))
	case StateConfig:
		return send(dst, p.config)
	case StateOptions:
		return send(dst, appendControl(p.tx[:0], p.dialect, dmrd.Control{Command: dmrd.CommandRPTO, ID: p.id, Text: p.options}))
	case StateRunning:
		c := dmrd.CommandRPTPING
		if p.dialect == dmrd.DialectHBRP {
			c = dmrd.CommandMSTPING
		}
		return send(dst, appendControl(p.tx[:0], p.dialect, dmrd.Control{Command: c, ID: p.id}))
	case StateIdle, StateWait:
	}
	return dst
}

// Packet takes the packet b from the master. It answers each
// acknowledgement with the next login step, reports OutputUp when the
// login is complete, takes a pong as a sign of life, reports MSTNAK and
// MSTCL with OutputDown and logs in again, and passes DMRD, DMRA, DMRG
// and RPTSBKN of a logged-in session on. In DialectHBRP it ignores an
// MSTACK with the salt after the RPTK, as a master sends for an RPTL that
// the Peer sent again, and the token is of the salt text as the master
// sent it, in upper or lower case. It ignores
// a packet it cannot use. It appends the outputs to dst, and allocates
// only when dst has no room for MaxOutputs outputs.
func (p *Peer) Packet(now time.Duration, b []byte, dst []Output) []Output {
	if p.stopped || !p.started || len(b) > MaxPacket {
		return dst
	}
	c := dmrd.Classify(b, p.dialect)
	switch c {
	case dmrd.CommandDMRD, dmrd.CommandDMRA, dmrd.CommandDMRG:
		if p.state == StateRunning && dataOK(b, c) {
			dst = append(dst, Output{Kind: OutputData, Packet: b, Command: c})
		}
	case dmrd.CommandRPTACK, dmrd.CommandMSTACK:
		m, err := dmrd.DecodeControl(b, p.dialect)
		if err != nil {
			return dst
		}
		return p.acked(now, &m, b, dst)
	case dmrd.CommandMSTNAK:
		if _, err := dmrd.DecodeControl(b, p.dialect); err != nil || p.state == StateWait {
			return dst
		}
		dst = append(dst, Output{Kind: OutputDown, Reason: ReasonRefused, State: p.state})
		if p.state == StateRunning {
			p.state = StateLogin
			p.retryAt, p.timeoutAt = now+p.retry, now+p.timeout
			return dst
		}
		p.wait(now)
	case dmrd.CommandMSTCL:
		if _, err := dmrd.DecodeControl(b, p.dialect); err != nil || p.state == StateWait {
			return dst
		}
		dst = append(dst, Output{Kind: OutputDown, Reason: ReasonClosed, State: p.state})
		p.wait(now)
	case dmrd.CommandMSTPONG, dmrd.CommandRPTPONG:
		if _, err := dmrd.DecodeControl(b, p.dialect); err == nil && p.state == StateRunning {
			p.timeoutAt = now + p.timeout
		}
	case dmrd.CommandRPTSBKN:
		if _, err := dmrd.DecodeControl(b, p.dialect); err == nil && p.state == StateRunning {
			dst = append(dst, Output{Kind: OutputMessage, Packet: b, Command: c})
		}
	default:
	}
	return dst
}

func (p *Peer) acked(now time.Duration, m *dmrd.Control, b []byte, dst []Output) []Output {
	if p.state != StateLogin && m.HasSalt {
		return dst
	}
	switch p.state {
	case StateLogin:
		switch {
		case p.dialect == dmrd.DialectMMDVM:
			p.salt = m.ID
		case m.HasSalt:
			p.salt = m.Salt
			copy(p.saltText[:], b[14:])
		default:
			return dst
		}
		p.state = StateAuth
	case StateAuth:
		p.state = StateConfig
	case StateConfig:
		if len(p.options) == 0 {
			return p.up(now, dst)
		}
		p.state = StateOptions
	case StateOptions:
		return p.up(now, dst)
	case StateIdle, StateWait, StateRunning:
		return dst
	}
	p.retryAt, p.timeoutAt = now+p.retry, now+p.timeout
	return p.send(dst)
}

func (p *Peer) up(now time.Duration, dst []Output) []Output {
	p.state, p.fails = StateRunning, 0
	p.retryAt, p.timeoutAt = now+p.retry, now+p.timeout
	return append(dst, Output{Kind: OutputUp, ID: p.id})
}

func dataOK(b []byte, c dmrd.Command) bool {
	var err error
	switch c {
	case dmrd.CommandDMRD:
		_, err = dmrd.DecodeData(b)
	case dmrd.CommandDMRA:
		_, err = dmrd.DecodeTalkerAlias(b)
	case dmrd.CommandDMRG:
		_, err = dmrd.DecodePosition(b)
	default:
		return false
	}
	return err == nil
}
