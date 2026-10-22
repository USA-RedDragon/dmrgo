package dmrdsession

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

// MasterConfig configures NewMaster.
type MasterConfig struct {
	// Dialect is dmrd.DialectMMDVM or dmrd.DialectHBRP.
	Dialect dmrd.Dialect
	// Timeout is the time without a packet from the repeater after which
	// the Master ends the dmrdsession. 0 means TimeoutDefault.
	Timeout time.Duration
	// LoginTimeout is the time after the first RPTL of a login, and
	// after the RPTK that proves the password, in which the repeater must
	// finish its login, or the Master ends the dmrdsession. A repeated RPTL
	// does not restart it. 0 means LoginTimeoutDefault.
	LoginTimeout time.Duration
}

// Master is the master side of the session of one repeater of the MMDVM
// network protocol or of HBRP. The consumer keeps one Master for each
// address that sends it packets, and passes it the packets of that
// address. Make it with NewMaster; the zero Master takes no packet. A
// Master is not safe for concurrent use and must not be copied.
type Master struct {
	dialect      dmrd.Dialect
	timeout      time.Duration
	loginTimeout time.Duration

	tx        []byte
	state     State
	id        uint32
	salt      uint32
	tok       [32]byte
	asked     bool
	timeoutAt time.Duration
	noCopy    noCopy
}

// NewMaster returns a Master for cfg in StateIdle. It returns
// dmrgo.ErrNilArgument for a nil cfg and a *dmrgo.ConfigError for a
// Dialect other than DialectMMDVM or DialectHBRP, or a negative Timeout or
// LoginTimeout. It allocates the
// Master.
func NewMaster(cfg *MasterConfig) (*Master, error) {
	switch {
	case cfg == nil:
		return nil, dmrgo.ErrNilArgument
	case cfg.Dialect != dmrd.DialectMMDVM && cfg.Dialect != dmrd.DialectHBRP:
		return nil, errDialect
	case cfg.Timeout < 0:
		return nil, errTimeout
	case cfg.LoginTimeout < 0:
		return nil, errLoginTimeout
	}
	return &Master{
		dialect:      cfg.Dialect,
		timeout:      orDefault(cfg.Timeout, TimeoutDefault),
		loginTimeout: orDefault(cfg.LoginTimeout, LoginTimeoutDefault),
		tx:           make([]byte, 0, MaxPacket),
	}, nil
}

// State returns the state of the dmrdsession.
func (m *Master) State() State {
	if m == nil {
		return 0
	}

	return m.state
}

// ID returns the repeater ID of the session, from its last RPTL.
func (m *Master) ID() uint32 {
	if m == nil {
		return 0
	}

	return m.id
}

// Reset ends the session without a packet or an output.
func (m *Master) Reset() {
	if m == nil {
		return
	}

	m.state, m.id, m.salt, m.tok, m.asked = StateIdle, 0, 0, [32]byte{}, false
}

// Deadline returns the time at which the Master needs a Tick. ok is false
// in StateIdle.
func (m *Master) Deadline() (at time.Duration, ok bool) {
	if m == nil {
		return 0, false
	}

	if m.state == StateIdle {
		return 0, false
	}
	return m.timeoutAt, true
}

// Tick ends the session with OutputDown and ReasonTimeout when the
// repeater sent nothing for Timeout. It sends nothing: a packet of the
// repeater after that gets an MSTNAK. It allocates only when dst has no
// room for MaxOutputs outputs.
func (m *Master) Tick(now time.Duration, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if at, ok := m.Deadline(); ok && now >= at {
		dst = m.down(dst, ReasonTimeout)
	}
	return dst
}

// Resume starts the logged-in session of the repeater id without a login,
// for a repeater that logged in to another Master, such as one of another
// replica of a server, and now sends its packets here. It ends a session
// of another state or ID with OutputDown and ReasonLogin, and reports
// OutputUp; for the session of id that runs it only restarts Timeout. It
// sends nothing. The zero Master does nothing. It allocates
// only when dst has no room for MaxOutputs outputs.
func (m *Master) Resume(now time.Duration, id uint32, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.dialect == dmrd.DialectUnknown {
		return dst
	}
	if m.state == StateRunning && m.id == id {
		m.timeoutAt = after(now, m.timeout)
		return dst
	}
	if m.state != StateIdle {
		dst = m.down(dst, ReasonLogin)
	}
	m.id, m.state, m.tok, m.asked, m.timeoutAt = id, StateRunning, [32]byte{}, false, after(now, m.timeout)
	return append(dst, Output{Kind: OutputUp, ID: id})
}

// Admit answers the RPTL of OutputLogin with the salt, which the caller
// draws at random, and keeps the login token of the salt and the password
// of the repeater, of any length, to check the RPTK. It does nothing in
// another state. It allocates only when dst has no room for MaxOutputs
// outputs.
func (m *Master) Admit(now time.Duration, salt uint32, password []byte, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.state != StateLogin {
		return dst
	}
	m.salt = salt
	m.tok, _ = auth.Token(m.dialect, salt, password)
	m.state = StateAuth
	return m.challenge(dst)
}

// Accept answers the RPTC of OutputConfig with an acknowledgement. In
// StateConfig it completes the login with OutputUp and starts Timeout. It
// does nothing when no RPTC waits for an answer. It allocates only when
// dst has no room for MaxOutputs outputs.
func (m *Master) Accept(now time.Duration, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if !m.asked {
		return dst
	}
	m.asked = false
	dst = m.acked(dst)
	if m.state == StateConfig {
		m.state, m.tok, m.timeoutAt = StateRunning, [32]byte{}, after(now, m.timeout)
		dst = append(dst, Output{Kind: OutputUp, ID: m.id})
	}
	return dst
}

// Refuse answers the RPTL of OutputLogin or the RPTC of OutputConfig, or
// ends a session, with MSTNAK and OutputDown with ReasonRefused. It does
// nothing in StateIdle. It allocates only when dst has no room for
// MaxOutputs outputs.
func (m *Master) Refuse(now time.Duration, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.state == StateIdle {
		return dst
	}
	dst = send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: dmrd.CommandMSTNAK, ID: m.id}))
	return m.down(dst, ReasonRefused)
}

// Close ends a session with MSTCL, as a master that shuts down sends it,
// and OutputDown with ReasonStopped. It does nothing in StateIdle. It
// allocates only when dst has no room for MaxOutputs outputs.
func (m *Master) Close(now time.Duration, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.state == StateIdle {
		return dst
	}
	dst = send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: dmrd.CommandMSTCL, ID: m.id}))
	return m.down(dst, ReasonStopped)
}

func (m *Master) down(dst []Output, r Reason) []Output {
	dst = append(dst, Output{Kind: OutputDown, ID: m.id, Reason: r, State: m.state})
	m.state, m.tok, m.asked = StateIdle, [32]byte{}, false
	return dst
}

func (m *Master) ack() dmrd.Command {
	if m.dialect == dmrd.DialectHBRP {
		return dmrd.CommandMSTACK
	}
	return dmrd.CommandRPTACK
}

func (m *Master) challenge(dst []Output) []Output {
	if m.dialect == dmrd.DialectHBRP {
		return send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: dmrd.CommandMSTACK, ID: m.id, Salt: m.salt, HasSalt: true}))
	}
	return send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: dmrd.CommandRPTACK, ID: m.salt}))
}

func (m *Master) acked(dst []Output) []Output {
	return send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: m.ack(), ID: m.id}))
}

func (m *Master) nak(dst []Output, id uint32) []Output {
	return send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: dmrd.CommandMSTNAK, ID: id}))
}

// Packet takes the packet b from the repeater. An RPTL starts a login and
// reports OutputLogin, or gets the same salt again while the Master waits
// for the RPTK. An RPTK with the token of the salt and the password gets
// an acknowledgement, and another token MSTNAK. An RPTC for the ID, in
// StateConfig or StateRunning, reports OutputConfig and waits for Accept
// or Refuse; in StateConfig LoginTimeout still runs.
// In StateRunning an RPTO gets an acknowledgement and OutputOptions, a
// ping gets a pong and OutputPong, an RPTCL ends the session with ReasonClosed, and
// DMRD, DMRA, DMRG, RPTG, RPTRSSI and RPTINTR are passed on. A packet of
// a step that the session has not reached gets MSTNAK, as the HBRP
// specification says for a repeater that is not logged in. Each packet
// that the Master takes restarts Timeout. It allocates only when dst has
// no room for MaxOutputs outputs.
func (m *Master) Packet(now time.Duration, b []byte, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.dialect == dmrd.DialectUnknown || len(b) > MaxPacket {
		return dst
	}
	c := dmrd.Classify(b, m.dialect)
	switch c {
	case dmrd.CommandRPTL:
		return m.login(now, b, dst)
	case dmrd.CommandRPTK:
		return m.token(now, b, dst)
	case dmrd.CommandRPTC:
		return m.config(now, b, dst)
	case dmrd.CommandDMRD, dmrd.CommandDMRA, dmrd.CommandDMRG:
		if !dataOK(b, c) {
			return dst
		}
		id, _ := dmrd.RepeaterID(b, m.dialect)
		if m.state != StateRunning || id != m.id {
			return m.nak(dst, id)
		}
		m.timeoutAt = after(now, m.timeout)
		return append(dst, Output{Kind: OutputData, Packet: b, Command: c, ID: m.id})
	case dmrd.CommandRPTCL:
		k, err := dmrd.DecodeControl(b, m.dialect)
		if err != nil || m.state == StateIdle || k.ID != m.id {
			return dst
		}
		return m.down(dst, ReasonClosed)
	case dmrd.CommandRPTO, dmrd.CommandRPTPING, dmrd.CommandMSTPING, dmrd.CommandRPTG, dmrd.CommandRPTRSSI, dmrd.CommandRPTINTR:
		return m.running(now, b, c, dst)
	default:
	}
	return dst
}

func (m *Master) login(now time.Duration, b []byte, dst []Output) []Output {
	k, err := dmrd.DecodeControl(b, m.dialect)
	if err != nil {
		return dst
	}
	if m.state == StateLogin && k.ID == m.id {
		return dst
	}
	if m.state == StateAuth && k.ID == m.id {
		return m.challenge(dst)
	}
	if m.state != StateIdle {
		dst = m.down(dst, ReasonLogin)
	}
	m.id, m.state, m.timeoutAt = k.ID, StateLogin, after(now, m.loginTimeout)
	return append(dst, Output{Kind: OutputLogin, ID: k.ID})
}

func (m *Master) token(now time.Duration, b []byte, dst []Output) []Output {
	k, err := dmrd.DecodeControl(b, m.dialect)
	if err != nil {
		return dst
	}
	if (m.state != StateAuth && m.state != StateConfig) || k.ID != m.id {
		return m.nak(dst, k.ID)
	}
	if !digest.Equal(m.tok[:], k.Token[:]) {
		dst = m.nak(dst, k.ID)
		return m.down(dst, ReasonRefused)
	}
	m.state, m.timeoutAt = StateConfig, after(now, m.loginTimeout)
	return m.acked(dst)
}

func (m *Master) running(now time.Duration, b []byte, c dmrd.Command, dst []Output) []Output {
	k, err := dmrd.DecodeControl(b, m.dialect)
	if err != nil {
		return dst
	}
	if m.state != StateRunning || k.ID != m.id {
		return m.nak(dst, k.ID)
	}
	m.timeoutAt = after(now, m.timeout)
	switch c {
	case dmrd.CommandRPTO:
		dst = m.acked(dst)
		return append(dst, Output{Kind: OutputOptions, Packet: k.Text, ID: m.id})
	case dmrd.CommandRPTPING, dmrd.CommandMSTPING:
		pong := dmrd.CommandMSTPONG
		if m.dialect == dmrd.DialectHBRP {
			pong = dmrd.CommandRPTPONG
		}
		dst = send(dst, appendControl(m.tx[:0], m.dialect, dmrd.Control{Command: pong, ID: m.id}))
		return append(dst, Output{Kind: OutputPong, ID: m.id})
	default:
	}
	return append(dst, Output{Kind: OutputMessage, Packet: b, Command: c, ID: m.id})
}

func (m *Master) config(now time.Duration, b []byte, dst []Output) []Output {
	var id uint32
	var err error
	if m.dialect == dmrd.DialectHBRP {
		var c *dmrd.HBRPConfig
		if c, err = dmrd.HBRPConfigOf(b); err == nil {
			id, err = c.ID()
		}
	} else {
		var c *dmrd.Config
		if c, err = dmrd.ConfigOf(b); err == nil {
			id = c.ID()
		}
	}
	if err != nil {
		return dst
	}
	if (m.state != StateConfig && m.state != StateRunning) || id != m.id {
		return m.nak(dst, id)
	}
	if m.state == StateRunning {
		m.timeoutAt = after(now, m.timeout)
	}
	m.asked = true
	return append(dst, Output{Kind: OutputConfig, Packet: b, ID: m.id})
}
