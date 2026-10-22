package dmrdsession

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
)

// BridgeConfig configures NewBridge.
type BridgeConfig struct {
	// Key is the passphrase of the OpenBridge link (see dmrdauth.NewBridgeKey).
	Key []byte
	// Enhanced turns on FreeDMR's enhanced OpenBridge: the Bridge sends a
	// BCKA every Keepalive and takes the peer as down after Timeout
	// without a packet with a good signature. Without it OpenBridge has no
	// keepalive, and the Bridge only checks signatures.
	Enhanced bool
	// Keepalive is the BCKA period. 0 means KeepaliveDefault.
	Keepalive time.Duration
	// Timeout is the time without a packet with a good signature after
	// which an enhanced Bridge reports OutputDown. 0 means TimeoutDefault,
	// the time after which FreeDMR stops sending to a peer.
	Timeout time.Duration
}

// Bridge is one OpenBridge link between two masters. It checks the
// signature of each packet and, with BridgeConfig.Enhanced, runs the BCKA
// keepalive and the liveness of the peer as FreeDMR does. Make it with
// NewBridge; the zero Bridge takes no packet. A Bridge is not safe for
// concurrent use and must not be copied.
type Bridge struct {
	key       *dmrdauth.BridgeKey
	enhanced  bool
	keepalive time.Duration
	timeout   time.Duration

	bcka    []byte
	state   State
	started bool
	sendAt  time.Duration
	lastAt  time.Duration
	noCopy  noCopy
}

// NewBridge returns a Bridge for cfg. It returns dmrgo.ErrNilArgument for a
// nil cfg and a *dmrgo.ConfigError for a negative Keepalive or Timeout. It allocates the Bridge and its key.
func NewBridge(cfg *BridgeConfig) (*Bridge, error) {
	switch {
	case cfg == nil:
		return nil, dmrgo.ErrNilArgument
	case cfg.Keepalive < 0:
		return nil, errKeepalive
	case cfg.Timeout < 0:
		return nil, errTimeout
	}
	b := &Bridge{
		key:       dmrdauth.NewBridgeKey(cfg.Key),
		enhanced:  cfg.Enhanced,
		keepalive: orDefault(cfg.Keepalive, KeepaliveDefault),
		timeout:   orDefault(cfg.Timeout, TimeoutDefault),
	}
	b.bcka, _ = (&dmrd.BridgeControl{Command: dmrd.CommandBCKA}).AppendBinary(nil)
	if err := b.key.SignInPlace(b.bcka); err != nil {
		return nil, err
	}
	return b, nil
}

// State returns StateRunning when the peer sent a packet with a good
// signature within Timeout, and StateIdle before that and after.
func (b *Bridge) State() State {
	if b == nil {
		return 0
	}

	return b.state
}

// SignInPlace writes the signature of the OpenBridge packet p into its last
// octets, as dmrdauth.BridgeKey.SignInPlace does. It returns
// dmrgo.ErrZeroValue for a Bridge that NewBridge did not make. It does not
// allocate.
func (b *Bridge) SignInPlace(p []byte) error {
	if b == nil {
		return dmrgo.ErrNilReceiver
	}

	if b.key == nil {
		return dmrgo.ErrZeroValue
	}
	return b.key.SignInPlace(p)
}

// Deadline returns the time at which the Bridge needs a Tick. ok is false
// without BridgeConfig.Enhanced.
func (b *Bridge) Deadline() (at time.Duration, ok bool) {
	if b == nil {
		return 0, false
	}

	if !b.enhanced || b.key == nil {
		return 0, false
	}
	if !b.started {
		return 0, true
	}
	if b.state == StateRunning {
		return min(b.sendAt, after(b.lastAt, b.timeout)), true
	}
	return b.sendAt, true
}

// Tick sends the BCKA of an enhanced Bridge when it is due, the first at
// the first Tick, and reports OutputDown with ReasonTimeout when the peer
// sent nothing for Timeout. It allocates only when dst has no room for
// MaxOutputs outputs.
func (b *Bridge) Tick(now time.Duration, dst []Output) []Output {
	if b == nil {
		return dst
	}

	if !b.enhanced || b.key == nil {
		return dst
	}
	if b.state == StateRunning && now >= after(b.lastAt, b.timeout) {
		b.state = StateIdle
		dst = append(dst, Output{Kind: OutputDown, Reason: ReasonTimeout, State: StateRunning})
	}
	if !b.started || now >= b.sendAt {
		b.started, b.sendAt = true, after(now, b.keepalive)
		dst = send(dst, b.bcka)
	}
	return dst
}

// Packet takes the packet p from the peer. A packet without a good
// signature, or that is not OpenBridge, gets no output. A DMRD or DMRE
// packet gives OutputData, and BCSQ, BCST and BCVE give OutputMessage. In
// an enhanced Bridge, every packet with a good signature, a BCKA among
// them, is a sign of life: the first after StateIdle reports OutputUp.
// It allocates only when dst has no room for MaxOutputs outputs.
func (b *Bridge) Packet(now time.Duration, p []byte, dst []Output) []Output {
	if b == nil {
		return dst
	}

	if b.key == nil || len(p) > MaxPacket {
		return dst
	}
	if _, ok := b.key.Verify(p); !ok {
		return dst
	}
	var c dmrd.Command
	if m, err := dmrd.DecodeBridge(p); err == nil {
		c = dmrd.CommandDMRD
		if m.Version != 1 {
			c = dmrd.CommandDMRE
		}
	} else if m, err := dmrd.DecodeBridgeControl(p); err == nil {
		c = m.Command
	} else {
		return dst
	}
	if b.enhanced {
		b.lastAt = now
		if b.state != StateRunning {
			b.state = StateRunning
			dst = append(dst, Output{Kind: OutputUp})
		}
	}
	switch c {
	case dmrd.CommandDMRD, dmrd.CommandDMRE:
		dst = append(dst, Output{Kind: OutputData, Packet: p, Command: c})
	case dmrd.CommandBCSQ, dmrd.CommandBCST, dmrd.CommandBCVE:
		dst = append(dst, Output{Kind: OutputMessage, Packet: p, Command: c})
	default:
	}
	return dst
}
