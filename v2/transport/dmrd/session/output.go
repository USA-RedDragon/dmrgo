package session

import (
	"math"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
)

// MaxOutputs is the most outputs that one call of a Peer, Master or Bridge
// method appends.
const MaxOutputs = 4

// The defaults of the timers, as DMRGateway sets them.
const (
	// RetryDefault is the default PeerConfig.Retry: the time between two
	// sends of a login step, and between two pings.
	RetryDefault = 10 * time.Second
	// TimeoutDefault is the default PeerConfig.Timeout and
	// MasterConfig.Timeout.
	TimeoutDefault = 60 * time.Second
	// LoginTimeoutDefault is the default MasterConfig.LoginTimeout: the
	// PING_TIME of 5 s times the MAX_MISSED of 3 after which an HBlink3
	// master drops a repeater, logged in or not. It is longer than the
	// RetryDefault after which DMRGateway sends a login step again.
	LoginTimeoutDefault = 15 * time.Second
	// KeepaliveDefault is the default BridgeConfig.Keepalive, the BCKA
	// period of FreeDMR.
	KeepaliveDefault = 10 * time.Second
)

// MaxOptions is the longest RPTO options text in octets that DMRGateway
// sends.
const MaxOptions = 291

// MaxPacket is the longest packet in octets that the machines take. They
// drop a longer packet. The longest message is the 306-octet RPTC of
// DialectHBRP.
const MaxPacket = 512

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputSend is a packet in Output.Packet to send to the other side.
	OutputSend OutputKind = iota + 1
	// OutputUp reports that the login is complete. Traffic can flow.
	OutputUp
	// OutputDown reports that the session ended. Output.Reason tells why
	// and Output.State gives the state in which it ended.
	OutputDown
	// OutputLogin reports the RPTL of the repeater in Output.ID at a
	// Master. Call Master.Admit or Master.Refuse.
	OutputLogin
	// OutputConfig reports the RPTC that the Master accepted, in
	// Output.Packet.
	OutputConfig
	// OutputOptions reports the RPTO options text that the Master
	// accepted, in Output.Packet.
	OutputOptions
	// OutputData is a DMRD, DMRA or DMRG packet, or for a Bridge a DMRD or
	// DMRE packet with a good signature, in Output.Packet, with its
	// command in Output.Command.
	OutputData
	// OutputMessage is another message of a logged-in session for the
	// consumer, in Output.Packet with its command in Output.Command: an
	// RPTSBKN at a Peer, an RPTG, RPTRSSI or RPTINTR at a Master, or a
	// BCSQ, BCST or BCVE at a Bridge.
	OutputMessage
)

// String returns the name of the OutputKind.
func (k OutputKind) String() string {
	switch k {
	case OutputSend:
		return "Send"
	case OutputUp:
		return "Up"
	case OutputDown:
		return "Down"
	case OutputLogin:
		return nameLogin
	case OutputConfig:
		return nameConfig
	case OutputOptions:
		return nameOptions
	case OutputData:
		return "Data"
	case OutputMessage:
		return "Message"
	}
	return unknownName
}

// Reason tells why a session ended.
type Reason uint8

// The Reason values.
const (
	// ReasonNone is the zero Reason.
	ReasonNone Reason = iota
	// ReasonTimeout is Timeout without a reply or a packet.
	ReasonTimeout
	// ReasonRefused is an MSTNAK from the master, or a login that the
	// Master refused.
	ReasonRefused
	// ReasonClosed is an MSTCL from the master or an RPTCL from the
	// repeater.
	ReasonClosed
	// ReasonStopped is a Close of the consumer.
	ReasonStopped
	// ReasonLogin is a new RPTL from a repeater that was logged in.
	ReasonLogin
)

// String returns the name of the Reason.
func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "None"
	case ReasonTimeout:
		return "Timeout"
	case ReasonRefused:
		return "Refused"
	case ReasonClosed:
		return "Closed"
	case ReasonStopped:
		return "Stopped"
	case ReasonLogin:
		return nameLogin
	}
	return unknownName
}

// State is the state of a session.
type State uint8

// The State values.
const (
	// StateIdle has no session: a Peer after Close, a Master before an
	// RPTL or after the end of a session, a Bridge that has not heard
	// its peer within Timeout.
	StateIdle State = iota
	// StateWait is a Peer that waits to log in again.
	StateWait
	// StateLogin is a Peer that sent RPTL, or a Master that waits for
	// Admit or Refuse.
	StateLogin
	// StateAuth is a Peer that sent RPTK, or a Master that sent the salt.
	StateAuth
	// StateConfig is a Peer that sent RPTC, or a Master that waits for it.
	StateConfig
	// StateOptions is a Peer that sent RPTO.
	StateOptions
	// StateRunning is a logged-in session, or a Bridge that heard its
	// peer within Timeout.
	StateRunning
)

// String returns the name of the State.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateWait:
		return "Wait"
	case StateLogin:
		return nameLogin
	case StateAuth:
		return "Auth"
	case StateConfig:
		return nameConfig
	case StateOptions:
		return nameOptions
	case StateRunning:
		return "Running"
	}
	return unknownName
}

const (
	unknownName = "Unknown"
	nameLogin   = "Login"
	nameConfig  = "Config"
	nameOptions = "Options"
)

// Output is one result of a Peer, Master or Bridge. Fields that do not
// apply to Kind are zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Packet is the packet of OutputSend, OutputConfig, OutputData and
	// OutputMessage, or the options text of OutputOptions. It holds at
	// most MaxPacket octets.
	Packet []byte
	// Command is the command of OutputData and OutputMessage.
	Command dmrd.Command
	// ID is the repeater ID of OutputLogin, OutputUp and OutputDown at a
	// Master.
	ID uint32
	// Reason tells why for OutputDown.
	Reason Reason
	// State is the state in which the session ended, for OutputDown.
	State State
}

type noCopy struct{}

func (*noCopy) Lock() {}

func (*noCopy) Unlock() {}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

func appendControl(dst []byte, d dmrd.Dialect, c dmrd.Control) []byte {
	b, err := c.AppendBinary(dst, d)
	if err != nil {
		return dst
	}
	return b
}

func send(dst []Output, p []byte) []Output {
	if len(p) == 0 {
		return dst
	}
	return append(dst, Output{Kind: OutputSend, Packet: p})
}

func after(t, d time.Duration) time.Duration {
	if d > 0 && t > math.MaxInt64-d {
		return math.MaxInt64
	}
	return t + d
}
