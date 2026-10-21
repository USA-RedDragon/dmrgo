package session

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

// MasterConfig configures NewMaster.
type MasterConfig struct {
	// ID is the peer ID of the master.
	ID uint32
	// Mode is the mode that the master sends. 0 means ModeDefault.
	Mode ipsc.Mode
	// Flags is the flags that the master sends. The zero Flags means voice
	// and data calls. The Master sets ipsc.FlagMasterPeer, and
	// ipsc.FlagAuth with Auth.
	Flags ipsc.Flags
	// Version is the protocol version that the master sends. Zero means
	// VersionDefault.
	Version [4]byte
	// Auth turns on the HMAC-SHA1 trailer with Key: the Master drops a
	// packet without a good trailer and signs what it sends.
	Auth bool
	// Key is the authentication key (see auth.ParseKey).
	Key [20]byte
	// Timeout is the time without a packet from a peer after which the
	// Master drops it. 0 means TimeoutDefault.
	Timeout time.Duration
	// MaxPeers is the most peers that the Master registers. 0 means
	// MaxPeersDefault.
	MaxPeers int
}

// Master is the master of an IPSC system, as DMRlink and ipsc2hbrp run
// it: it registers peers, answers their keepalives, sends the peer list
// to every peer when one asks for it and when a peer leaves or times out,
// and passes the voice and data packets of registered peers on. Make it
// with NewMaster; the zero Master takes no packet. A Master is not safe
// for concurrent use and must not be copied.
type Master struct {
	l        link
	timeout  time.Duration
	maxPeers int
	stopped  bool
	tx       []byte
	noCopy   noCopy
}

// NewMaster returns a Master for cfg. It returns a *ConfigError for a nil
// cfg, a negative Timeout or a MaxPeers above MaxPeersLimit. It allocates
// the Master, its peer table and its key.
func NewMaster(cfg *MasterConfig) (*Master, error) {
	if cfg == nil {
		return nil, errID
	}
	if cfg.Timeout < 0 {
		return nil, errTimer
	}
	if cfg.MaxPeers < 0 || cfg.MaxPeers > MaxPeersLimit {
		return nil, errMaxPeers
	}
	n := cfg.MaxPeers
	if n == 0 {
		n = MaxPeersDefault
	}
	flags := cfg.Flags
	if flags == (ipsc.Flags{}) {
		flags.Set(ipsc.FlagVoice, true)
		flags.Set(ipsc.FlagData, true)
	}
	flags.Set(ipsc.FlagMasterPeer, true)
	return &Master{
		l:        newLink(cfg.ID, cfg.Mode, flags, cfg.Version, cfg.Auth, cfg.Key, n),
		timeout:  orDefault(cfg.Timeout, TimeoutDefault),
		maxPeers: n,
		tx:       make([]byte, 0, MaxPacket),
	}, nil
}

// MaxOutputs returns the most outputs that one call of the Master appends.
func (m *Master) MaxOutputs() int {
	return 2*m.maxPeers + 2
}

// State returns StateRunning, or StateIdle after Close and for a Master
// that NewMaster did not make.
func (m *Master) State() State {
	if m.stopped || m.maxPeers == 0 {
		return StateIdle
	}
	return StateRunning
}

// Peers returns the number of registered peers.
func (m *Master) Peers() int {
	return m.l.n
}

// PeerAt returns registered peer i, or the zero PeerInfo when i is not
// below Peers.
func (m *Master) PeerAt(i int) PeerInfo {
	if i < 0 || i >= m.l.n {
		return PeerInfo{}
	}
	return m.l.peers[i].info
}

// Sign appends the authentication trailer of the packet in dst to dst
// when the config has Auth, and returns dst as it is without. It
// allocates only when dst has no room for auth.TrailerLen more octets.
func (m *Master) Sign(dst []byte) []byte {
	return m.l.sign(dst)
}

// Reset forgets every peer without a packet and runs the Master again
// after Close.
func (m *Master) Reset() {
	m.l.n = 0
	m.stopped = false
}

// Close sends a de-registration request to every registered peer, as
// DMRlink does when it shuts down, reports OutputDown with ReasonStopped
// for each, and stops the Master until Reset. It allocates only when dst
// has no room for MaxOutputs outputs.
func (m *Master) Close(_ time.Duration, dst []Output) []Output {
	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	p := m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: m.l.id})
	for i := range m.l.n {
		e := &m.l.peers[i].info
		dst = send(dst, p, e.Addr)
		dst = append(dst, Output{Kind: OutputDown, ID: e.ID, Addr: e.Addr, Reason: ReasonStopped})
	}
	m.l.n = 0
	m.stopped = true
	return dst
}

// Deadline returns the time at which the Master needs a Tick: when the
// first peer times out. ok is false without peers.
func (m *Master) Deadline() (at time.Duration, ok bool) {
	if m.stopped {
		return 0, false
	}
	for i := range m.l.n {
		t := after(m.l.peers[i].last, m.timeout)
		if !ok || t < at {
			at, ok = t, true
		}
	}
	return at, ok
}

// Tick drops the peers that sent nothing for Timeout, with OutputDown and
// ReasonTimeout, and then sends the new peer list to the peers left. It
// allocates only when dst has no room for MaxOutputs outputs.
func (m *Master) Tick(now time.Duration, dst []Output) []Output {
	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	dropped := false
	for i := 0; i < m.l.n; {
		e := &m.l.peers[i]
		if now < after(e.last, m.timeout) {
			i++
			continue
		}
		dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonTimeout})
		m.l.remove(i)
		dropped = true
	}
	if dropped {
		dst = m.listToAll(dst)
	}
	return dst
}

func (m *Master) listToAll(dst []Output) []Output {
	p := m.l.appendPeerList(m.tx[:0])
	for i := range m.l.n {
		dst = send(dst, p, m.l.peers[i].info.Addr)
	}
	return dst
}

// Packet takes the packet p from the address from. A registration request
// registers the peer and gets the registration reply, with OutputUp for a
// new peer. A keepalive of a registered peer gets the keepalive reply;
// with Auth, the keepalive of a peer that is not registered, whose
// trailer proves the key, registers it, as a repeater keeps sending
// keepalives for a while after its master restarts. A peer list request
// sends the peer list to every peer, as DMRlink does. A de-registration
// request from the address of the peer drops it with ReasonClosed and
// sends the new list to the rest. The peer to peer registration and
// keepalive of a registered peer get their replies. Any other packet of a
// registered peer gives OutputData. Each packet of a registered peer
// restarts its Timeout. A reply or status packet needs no answer. Any
// other packet that the Master does not take gives OutputDropped, with
// ReasonAuth, ReasonMalformed, ReasonUnregistered, ReasonFull or
// ReasonAddress. It allocates only when dst has no room for MaxOutputs
// outputs.
func (m *Master) Packet(now time.Duration, from Addr, p []byte, dst []Output) []Output {
	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	body, why := m.l.open(p)
	if why != ReasonNone {
		return dropped(dst, from, 0, why)
	}
	raw, err := ipsc.DecodeRaw(body)
	if err != nil {
		return dropped(dst, from, 0, ReasonMalformed)
	}
	i := m.l.find(raw.ID)
	switch raw.Opcode {
	case ipsc.OpRegisterRequest, ipsc.OpAliveRequest:
		c, err := ipsc.DecodeControl(body)
		if err != nil {
			return dropped(dst, from, raw.ID, ReasonMalformed)
		}
		if raw.Opcode == ipsc.OpRegisterRequest {
			return m.register(now, from, &c, i, dst, ipsc.OpRegisterReply)
		}
		if i < 0 && m.l.key == nil {
			return dropped(dst, from, raw.ID, ReasonUnregistered)
		}
		return m.register(now, from, &c, i, dst, ipsc.OpAliveReply)
	case ipsc.OpRegisterReply, ipsc.OpPeerListReply, ipsc.OpAliveReply, ipsc.OpDeregisterReply, ipsc.OpStatus:
		if i >= 0 {
			m.l.peers[i].last = now
		}
		return dst
	default:
	}
	if i < 0 {
		return dropped(dst, from, raw.ID, ReasonUnregistered)
	}
	return m.registered(now, from, i, &raw, body, dst)
}

func (m *Master) registered(now time.Duration, from Addr, i int, raw *ipsc.Raw, body []byte, dst []Output) []Output {
	e := &m.l.peers[i]
	e.last = now
	if isLinkControl(raw.Opcode) {
		if _, err := ipsc.DecodeControl(body); err != nil {
			return dropped(dst, from, raw.ID, ReasonMalformed)
		}
	}
	switch raw.Opcode {
	case ipsc.OpPeerListRequest:
		dst = m.listToAll(dst)
	case ipsc.OpDeregisterRequest:
		if e.info.Addr != from {
			return dropped(dst, from, raw.ID, ReasonAddress)
		}
		dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: from, Reason: ReasonClosed})
		m.l.remove(i)
		dst = m.listToAll(dst)
	case ipsc.OpPeerRegisterRequest:
		dst = send(dst, m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerRegisterReply, ID: m.l.id, Version: m.l.version}), from)
	case ipsc.OpPeerAliveRequest:
		dst = send(dst, m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerAliveReply, ID: m.l.id, Mode: m.l.mode, Flags: m.l.flags}), from)
	case ipsc.OpPeerRegisterReply, ipsc.OpPeerAliveReply:
	default:
		dst = append(dst, Output{Kind: OutputData, Packet: body, Addr: from, ID: raw.ID})
	}
	return dst
}

func (m *Master) register(now time.Duration, from Addr, c *ipsc.Control, i int, dst []Output, op ipsc.Opcode) []Output {
	isNew := i < 0
	if isNew {
		if i = m.l.add(); i < 0 {
			return dropped(dst, from, c.ID, ReasonFull)
		}
	}
	e := &m.l.peers[i]
	e.info = PeerInfo{ID: c.ID, Addr: from, Mode: c.Mode, Flags: c.Flags, Up: true}
	e.last = now
	r := ipsc.Control{Opcode: op, ID: m.l.id, Mode: m.l.mode, Flags: m.l.flags, Version: m.l.version}
	if op == ipsc.OpRegisterReply {
		r.Peers = uint16(m.l.n & 0xFFFF)
	}
	dst = send(dst, m.l.appendControl(m.tx[:0], r), from)
	if isNew {
		dst = append(dst, Output{Kind: OutputUp, ID: c.ID, Addr: from})
	}
	return dst
}
