package ipscsession

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
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
	// RegisterVersion is the protocol version that the master sends in a
	// register request or reply (0x90, 0x91, 0x94, 0x95). Zero means
	// RegisterVersionDefault.
	RegisterVersion [4]byte
	// AliveVersion is the protocol version that the master sends in a
	// keepalive request or reply (0x96, 0x97). Zero means
	// AliveVersionDefault.
	AliveVersion [4]byte
	// Auth turns on the HMAC-SHA1 trailer with Key: the Master drops a
	// packet without a good trailer and signs what it sends.
	Auth bool
	// Key is the authentication key (see ipscauth.ParseKey).
	Key [20]byte
	// Timeout is the time without a packet from a peer after which the
	// Master drops it. 0 means TimeoutDefault.
	Timeout time.Duration
	// MaxPeers is the most peers that the Master registers. 0 means
	// MaxPeersDefault.
	MaxPeers int
	// AdoptKnown makes Packet adopt a peer that is not registered, as
	// Adopt does, when the trailer of its packet proves the key of the
	// config or the key that the caller passed, so that a peer that
	// registered with another Master needs no Adopt call. Without a key it
	// does nothing.
	AdoptKnown bool
}

// Master is the master of an IPSC system, as DMRlink and ipsc2hbrp run
// it: it registers peers, answers their keepalives, sends the peer list
// to every peer when one asks for it and when a peer leaves, times out or
// is dropped, and passes the voice and data packets of registered peers
// on. Make it with NewMaster; the zero Master takes no packet. A Master is
// not safe for concurrent use and must not be copied.
type Master struct {
	l          link
	timeout    time.Duration
	maxPeers   int
	adoptKnown bool
	stopped    bool
	tx         []byte
	noCopy     noCopy
}

// NewMaster returns a Master for cfg. It returns dmrgo.ErrNilArgument for a
// nil cfg and a *dmrgo.ConfigError for a negative Timeout or a MaxPeers
// outside [0, MaxPeersLimit]. It allocates
// the Master, its peer table and its key.
func NewMaster(cfg *MasterConfig) (*Master, error) {
	if cfg == nil {
		return nil, dmrgo.ErrNilArgument
	}
	if cfg.Timeout < 0 {
		return nil, errTimeout
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
		l:          newLink(cfg.ID, cfg.Mode, flags, cfg.RegisterVersion, cfg.AliveVersion, cfg.Auth, cfg.Key, n),
		timeout:    orDefault(cfg.Timeout, TimeoutDefault),
		maxPeers:   n,
		adoptKnown: cfg.AdoptKnown,
		tx:         make([]byte, 0, MaxPacket),
	}, nil
}

// MaxOutputs returns the most outputs that one call of the Master appends.
func (m *Master) MaxOutputs() int {
	if m == nil {
		return 0
	}

	return 2*m.maxPeers + 2
}

// State returns StateRunning, or StateIdle after Close and for a Master
// that NewMaster did not make.
func (m *Master) State() State {
	if m == nil {
		return 0
	}

	if m.stopped || m.maxPeers == 0 {
		return StateIdle
	}
	return StateRunning
}

// Peers returns the number of registered peers.
func (m *Master) Peers() int {
	if m == nil {
		return 0
	}

	return m.l.n
}

// PeerAt returns registered peer i, or the zero PeerInfo when i is not
// below Peers.
func (m *Master) PeerAt(i int) PeerInfo {
	if m == nil {
		return PeerInfo{}
	}

	if i < 0 || i >= m.l.n {
		return PeerInfo{}
	}
	return m.l.peers[i].info
}

// Peer returns the registered peer id. ok is false when id is not
// registered.
func (m *Master) Peer(id uint32) (info PeerInfo, ok bool) {
	if m == nil {
		return PeerInfo{}, false
	}

	if i := m.l.find(id); i >= 0 {
		return m.l.peers[i].info, true
	}
	return PeerInfo{}, false
}

// Sign appends the authentication trailer of the packet in dst to dst
// when the config has Auth, and returns dst as it is without. It returns
// dst unchanged and dmrgo.ErrNilReceiver for a nil m. It allocates only
// when dst has no room for ipscauth.TrailerLen more octets.
func (m *Master) Sign(dst []byte) ([]byte, error) {
	if m == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	return m.l.sign(dst, nil), nil
}

// SignFor appends the authentication trailer of the packet in dst for the
// registered peer id to dst: with the own key of the peer when it
// registered with one, and else as Sign does. It
// returns dst unchanged and dmrgo.ErrNilReceiver for a nil m. It allocates
// only when dst has no room for ipscauth.TrailerLen more octets.
func (m *Master) SignFor(id uint32, dst []byte) ([]byte, error) {
	if m == nil {
		return dst, dmrgo.ErrNilReceiver
	}

	if i := m.l.find(id); i >= 0 {
		return m.l.sign(dst, m.l.peers[i].keyOf()), nil
	}
	return m.l.sign(dst, nil), nil
}

// Reset forgets every peer without a packet and runs the Master again
// after Close.
func (m *Master) Reset() {
	if m == nil {
		return
	}

	m.l.n = 0
	m.stopped = false
}

// Close sends a de-registration request to every registered peer, as
// DMRlink does when it shuts down, reports OutputDown with ReasonStopped
// for each, and stops the Master until Reset. It allocates only when dst
// has no room for MaxOutputs outputs.
func (m *Master) Close(now time.Duration, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	p := m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: m.l.id}, nil)
	for i := range m.l.n {
		e := &m.l.peers[i]
		if e.keyed {
			dst = send(dst, e.bye[:], e.info.Addr)
		} else {
			dst = send(dst, p, e.info.Addr)
		}
		dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonStopped})
	}
	m.l.n = 0
	m.stopped = true
	return dst
}

// Deadline returns the time at which the Master needs a Tick: when the
// first peer times out. ok is false without peers.
func (m *Master) Deadline() (at time.Duration, ok bool) {
	if m == nil {
		return 0, false
	}

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
	if m == nil {
		return dst
	}

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
		dropped = dropped || !e.keyed
		m.l.remove(i)
	}
	if dropped {
		dst = m.listToAll(dst)
	}
	return dst
}

// Drop removes the registered peer id at once, as Tick does when the peer
// times out: it reports OutputDown with ReasonDropped and sends the new
// peer list to the peers left, but sends nothing to the peer, as DMRlink
// sends nothing to a peer that times out. Packet then takes the peer as
// one that is not registered, so after a change of the key of the peer
// its packets with the old key get ReasonAuth until it registers with the
// new one. ok is false and dst is unchanged when id is not registered. It
// allocates only when dst has no room for MaxOutputs outputs.
func (m *Master) Drop(id uint32, dst []Output) (out []Output, ok bool) {
	if m == nil {
		return dst, false
	}

	if m.stopped || m.maxPeers == 0 {
		return dst, false
	}
	i := m.l.find(id)
	if i < 0 {
		return dst, false
	}
	e := &m.l.peers[i]
	dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonDropped})
	keyed := e.keyed
	m.l.remove(i)
	if !keyed {
		dst = m.listToAll(dst)
	}
	return dst, true
}

func (m *Master) listToAll(dst []Output) []Output {
	p := m.l.appendPeerList(m.tx[:0])
	for i := range m.l.n {
		if !m.l.peers[i].keyed {
			dst = send(dst, p, m.l.peers[i].info.Addr)
		}
	}
	return dst
}

// Packet takes the packet p from the address from. key is the own key of
// the sending peer, which the caller looks up by the peer ID in octets 1
// to 4 of p, and nil means the key of the config. A peer that registers
// with its own key keeps it: the Master signs what it sends to the peer
// with it, and the peer is in a system of its own, so its peer list holds
// only itself and the lists of the other peers leave it out. This lets
// one Master serve many repeaters with keys of their own.
//
// A registration request registers the peer and gets the registration
// reply, with OutputUp for a new peer. A keepalive of a registered peer
// gets the keepalive reply; with a key, the keepalive of a peer that is
// not registered, whose trailer proves the key, registers it, as a
// repeater keeps sending keepalives for a while after its master
// restarts. With MasterConfig.AdoptKnown, any other packet of a peer that
// is not registered, whose trailer proves the key, adopts the peer as
// Adopt does, but a de-registration request. A peer list request sends
// the peer list to every peer, as DMRlink does. A de-registration request
// from the address of the peer drops it with ReasonClosed and sends the
// new list to the rest. The peer to peer registration and keepalive of a
// registered peer get their replies. Any other packet of a registered
// peer gives OutputData. Each packet of a registered peer restarts its
// Timeout. A reply or status packet needs no answer. Any other packet
// that the Master does not take gives OutputDropped, with ReasonAuth,
// ReasonMalformed, ReasonUnregistered, ReasonFull or ReasonAddress. A
// packet of a peer that registered with its own key gives ReasonAuth for a
// nil key. It allocates only when dst has no room for MaxOutputs outputs.
func (m *Master) Packet(now time.Duration, from Addr, p []byte, key *[20]byte, dst []Output) []Output {
	if m == nil {
		return dst
	}

	return m.packet(now, from, p, key, dst)
}

func (m *Master) packet(now time.Duration, from Addr, p []byte, key *[20]byte, dst []Output) []Output {
	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	body, why := m.l.openWith(p, key)
	if why != ReasonNone {
		return dropped(dst, from, 0, why)
	}
	raw, err := ipsc.DecodeRaw(body)
	if err != nil {
		return dropped(dst, from, 0, ReasonMalformed)
	}
	i := m.l.find(raw.ID)
	if i >= 0 && key == nil && m.l.peers[i].keyed {
		return dropped(dst, from, raw.ID, ReasonAuth)
	}
	switch raw.Opcode {
	case ipsc.OpRegisterRequest, ipsc.OpAliveRequest:
		c, err := ipsc.DecodeControl(body)
		if err != nil {
			return dropped(dst, from, raw.ID, ReasonMalformed)
		}
		if raw.Opcode == ipsc.OpRegisterRequest {
			return m.register(now, from, &c, i, key, dst, ipsc.OpRegisterReply)
		}
		if i < 0 && m.l.key == nil && key == nil {
			return dropped(dst, from, raw.ID, ReasonUnregistered)
		}
		return m.register(now, from, &c, i, key, dst, ipsc.OpAliveReply)
	case ipsc.OpRegisterReply, ipsc.OpPeerListReply, ipsc.OpAliveReply, ipsc.OpDeregisterReply, ipsc.OpStatus:
		if i >= 0 {
			m.l.peers[i].last = now
		}
		return dst
	default:
	}
	if i < 0 {
		if !m.adoptKnown || (key == nil && m.l.key == nil) || raw.Opcode == ipsc.OpDeregisterRequest {
			return dropped(dst, from, raw.ID, ReasonUnregistered)
		}
		if dst, i = m.adopt(now, from, raw.ID, key, dst); i < 0 {
			return dst
		}
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
		if e.keyed {
			return send(dst, m.l.appendOwnList(m.tx[:0], i), from)
		}
		dst = m.listToAll(dst)
	case ipsc.OpDeregisterRequest:
		if e.info.Addr != from {
			return dropped(dst, from, raw.ID, ReasonAddress)
		}
		dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: from, Reason: ReasonClosed})
		keyed := e.keyed
		m.l.remove(i)
		if !keyed {
			dst = m.listToAll(dst)
		}
	case ipsc.OpPeerRegisterRequest:
		dst = send(dst, m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerRegisterReply, ID: m.l.id, Version: m.l.regVersion}, e.keyOf()), from)
	case ipsc.OpPeerAliveRequest:
		dst = send(dst, m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerAliveReply, ID: m.l.id, Mode: m.l.mode, Flags: m.flagsFor(e.keyOf())}, e.keyOf()), from)
	case ipsc.OpPeerRegisterReply, ipsc.OpPeerAliveReply:
	default:
		dst = append(dst, Output{Kind: OutputData, Packet: body, Addr: from, ID: raw.ID})
	}
	return dst
}

// Adopt registers the peer id at from without a registration request and
// without a reply, for a peer that registered with another Master, such as
// one of another replica of a server, and now sends its packets here. key
// is the own key of the peer, as Packet takes it, and nil means the key of
// the config. A new peer has ModeDefault and zero Flags until its next
// keepalive. It reports OutputUp for a new peer. For a registered peer it
// restarts the Timeout, keeps Mode and Flags, and takes from and key as
// the new address and key, so a nil key moves a peer with its own key
// into the system of the config. It gives OutputDropped with ReasonFull
// when MaxPeers leaves no room. It allocates only when dst has no room for MaxOutputs
// outputs.
func (m *Master) Adopt(now time.Duration, from Addr, id uint32, key *[20]byte, dst []Output) []Output {
	if m == nil {
		return dst
	}

	if m.stopped || m.maxPeers == 0 {
		return dst
	}
	dst, _ = m.adopt(now, from, id, key, dst)
	return dst
}

func (m *Master) adopt(now time.Duration, from Addr, id uint32, key *[20]byte, dst []Output) ([]Output, int) {
	i := m.l.find(id)
	if i >= 0 {
		info := m.l.peers[i].info
		info.Addr, info.Up = from, true
		return m.keep(dst, now, i, info, key), i
	}
	if i = m.l.add(); i < 0 {
		return dropped(dst, from, id, ReasonFull), -1
	}
	dst = m.keep(dst, now, i, PeerInfo{ID: id, Addr: from, Mode: ModeDefault, Up: true}, key)
	return append(dst, Output{Kind: OutputUp, ID: id, Addr: from}), i
}

func (m *Master) keep(dst []Output, now time.Duration, i int, info PeerInfo, key *[20]byte) []Output {
	e := &m.l.peers[i]
	e.info, e.last, e.keyed = info, now, key != nil
	if key != nil {
		e.key = *key
		copy(e.bye[:], m.l.appendControl(m.tx[:0], ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: m.l.id}, key))
	}
	return dst
}

func (m *Master) register(now time.Duration, from Addr, c *ipsc.Control, i int, key *[20]byte, dst []Output, op ipsc.Opcode) []Output {
	isNew := i < 0
	if isNew {
		if i = m.l.add(); i < 0 {
			return dropped(dst, from, c.ID, ReasonFull)
		}
	}
	dst = m.keep(dst, now, i, PeerInfo{ID: c.ID, Addr: from, Mode: c.Mode, Flags: c.Flags, Up: true}, key)
	r := ipsc.Control{Opcode: op, ID: m.l.id, Mode: m.l.mode, Flags: m.flagsFor(key), Version: m.l.aliveVersion}
	if op == ipsc.OpRegisterReply {
		r.Version = m.l.regVersion
		r.Peers = uint16(m.l.shared() & 0xFFFF)
		if key != nil {
			r.Peers = 1
		}
	}
	dst = send(dst, m.l.appendControl(m.tx[:0], r, key), from)
	if isNew {
		dst = append(dst, Output{Kind: OutputUp, ID: c.ID, Addr: from})
	}
	return dst
}

func (m *Master) flagsFor(key *[20]byte) ipsc.Flags {
	f := m.l.flags
	if key != nil {
		f.Set(ipsc.FlagAuth, true)
	}
	return f
}
