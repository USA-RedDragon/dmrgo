package session

import (
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
)

// PeerConfig configures NewPeer.
type PeerConfig struct {
	// ID is the peer ID.
	ID uint32
	// Mode is the mode that the peer sends. 0 means ModeDefault.
	Mode ipsc.Mode
	// Flags is the flags that the peer sends. The zero Flags means voice
	// and data calls. The Peer sets ipsc.FlagAuth with Auth.
	Flags ipsc.Flags
	// Version is the protocol version that the peer sends. Zero means
	// VersionDefault.
	Version [4]byte
	// Master is the address of the master.
	Master Addr
	// Auth turns on the HMAC-SHA1 trailer with Key: the Peer drops a
	// packet without a good trailer and signs what it sends.
	Auth bool
	// Key is the authentication key (see auth.ParseKey).
	Key [20]byte
	// Alive is the keepalive period. 0 means AliveDefault.
	Alive time.Duration
	// MaxMissed is the number of keepalives without a reply after which
	// the Peer takes a link as down. 0 means MaxMissedDefault.
	MaxMissed int
	// MaxPeers is the most other peers that the Peer keeps from the peer
	// list. 0 means MaxPeersDefault.
	MaxPeers int
}

// Peer is a peer of an IPSC system, as a MOTOTRBO repeater and DMRlink
// run it: it registers with the master and sends it keepalives, asks for
// the peer list, registers with each other peer of the list and sends it
// keepalives, answers the registrations and keepalives of the other
// peers, and passes the voice and data packets of the master and the
// peers on. Every Alive it sends what the state needs, as DMRlink does.
// The first Tick sends the registration request. Make it with NewPeer;
// the zero Peer does nothing. A Peer is not safe for concurrent use and
// must not be copied.
type Peer struct {
	l           link
	master      Addr
	alive       time.Duration
	maxMissed   int
	maxPeers    int
	state       State
	started     bool
	masterID    uint32
	outstanding int
	haveList    bool
	next        time.Duration
	tx          []byte
	txList      []byte
	txReg       []byte
	txAlive     []byte
	noCopy      noCopy
}

// NewPeer returns a Peer for cfg. It returns a *ConfigError for a nil
// cfg, a negative Alive or MaxMissed, or a MaxPeers above MaxPeersLimit. It
// allocates the Peer, its peer table and its key.
func NewPeer(cfg *PeerConfig) (*Peer, error) {
	if cfg == nil {
		return nil, errID
	}
	if cfg.Alive < 0 || cfg.MaxMissed < 0 {
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
	missed := cfg.MaxMissed
	if missed == 0 {
		missed = MaxMissedDefault
	}
	return &Peer{
		l:         newLink(cfg.ID, cfg.Mode, flags, cfg.Version, cfg.Auth, cfg.Key, n),
		master:    cfg.Master,
		alive:     orDefault(cfg.Alive, AliveDefault),
		maxMissed: missed,
		maxPeers:  n,
		state:     StateRegistering,
		tx:        make([]byte, 0, MaxControl),
		txList:    make([]byte, 0, MaxControl),
		txReg:     make([]byte, 0, MaxControl),
		txAlive:   make([]byte, 0, MaxControl),
	}, nil
}

// MaxOutputs returns the most outputs that one call of the Peer appends.
func (p *Peer) MaxOutputs() int {
	return 2*p.maxPeers + 4
}

// State returns the state of the Peer.
func (p *Peer) State() State {
	return p.state
}

// Master returns the ID of the master, which its registration reply gave.
// ok is false before that.
func (p *Peer) Master() (id uint32, ok bool) {
	return p.masterID, p.state == StateRunning
}

// Peers returns the number of other peers that the Peer knows from the
// peer list.
func (p *Peer) Peers() int {
	return p.l.n
}

// PeerAt returns peer i of the list, or the zero PeerInfo when i is not
// below Peers.
func (p *Peer) PeerAt(i int) PeerInfo {
	if i < 0 || i >= p.l.n {
		return PeerInfo{}
	}
	return p.l.peers[i].info
}

// Sign appends the authentication trailer of the packet in dst to dst
// when the config has Auth, and returns dst as it is without. It
// allocates only when dst has no room for auth.TrailerLen more octets.
func (p *Peer) Sign(dst []byte) []byte {
	return p.l.sign(dst)
}

// Reset forgets the master and the peers without a packet, so the next
// Tick registers again.
func (p *Peer) Reset() {
	p.l.n = 0
	p.state, p.started, p.masterID, p.outstanding, p.haveList = StateRegistering, false, 0, 0, false
}

// Close sends a de-registration request to the master and to every peer
// whose link is up, as DMRlink does when it leaves, reports OutputDown
// with ReasonStopped for the master, and stops the Peer until Reset. It
// allocates only when dst has no room for MaxOutputs outputs.
func (p *Peer) Close(_ time.Duration, dst []Output) []Output {
	if p.state == StateIdle {
		return dst
	}
	pk := p.l.appendControl(p.tx[:0], ipsc.Control{Opcode: ipsc.OpDeregisterRequest, ID: p.l.id})
	if p.state == StateRunning {
		dst = send(dst, pk, p.master)
		dst = append(dst, Output{Kind: OutputDown, ID: p.masterID, Addr: p.master, Reason: ReasonStopped})
	}
	for i := range p.l.n {
		if p.l.peers[i].info.Up {
			dst = send(dst, pk, p.l.peers[i].info.Addr)
		}
	}
	p.l.n = 0
	p.state = StateIdle
	return dst
}

// Deadline returns the time at which the Peer needs a Tick. ok is false
// after Close.
func (p *Peer) Deadline() (at time.Duration, ok bool) {
	if p.state == StateIdle {
		return 0, false
	}
	if !p.started {
		return 0, true
	}
	return p.next, true
}

// Tick sends, when Alive is due, the registration request or the
// keepalive to the master, the peer list request until the list arrives,
// and the registration request or the keepalive to each peer of the
// list. A link with MaxMissed keepalives without a reply goes down with
// ReasonTimeout. It allocates only when dst has no room for MaxOutputs
// outputs.
func (p *Peer) Tick(now time.Duration, dst []Output) []Output {
	if at, ok := p.Deadline(); !ok || now < at {
		return dst
	}
	p.started, p.next = true, now+p.alive
	if p.state == StateRunning && p.outstanding >= p.maxMissed {
		dst = append(dst, Output{Kind: OutputDown, ID: p.masterID, Addr: p.master, Reason: ReasonTimeout})
		p.state, p.outstanding = StateRegistering, 0
	}
	op := ipsc.OpAliveRequest
	if p.state == StateRegistering {
		op = ipsc.OpRegisterRequest
	}
	dst = send(dst, p.l.appendControl(p.tx[:0], ipsc.Control{Opcode: op, ID: p.l.id, Mode: p.l.mode, Flags: p.l.flags, Version: p.l.version}), p.master)
	if p.state == StateRunning {
		p.outstanding++
		if !p.haveList {
			dst = send(dst, p.l.appendControl(p.txList[:0], ipsc.Control{Opcode: ipsc.OpPeerListRequest, ID: p.l.id}), p.master)
		}
	}
	var reg, alive []byte
	for i := range p.l.n {
		e := &p.l.peers[i]
		if e.info.Up && e.outstanding >= p.maxMissed {
			e.info.Up, e.outstanding = false, 0
			dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonTimeout})
		}
		if !e.info.Up {
			if reg == nil {
				reg = p.l.appendControl(p.txReg[:0], ipsc.Control{Opcode: ipsc.OpPeerRegisterRequest, ID: p.l.id, Version: p.l.version})
			}
			dst = send(dst, reg, e.info.Addr)
			continue
		}
		if alive == nil {
			alive = p.l.appendControl(p.txAlive[:0], ipsc.Control{Opcode: ipsc.OpPeerAliveRequest, ID: p.l.id, Mode: p.l.mode, Flags: p.l.flags})
		}
		dst = send(dst, alive, e.info.Addr)
		e.outstanding++
	}
	return dst
}

// Packet takes the packet p from the address from. The registration reply
// of the master, from the address of the master, registers the Peer with
// OutputUp. The keepalive reply of the master, the peer to peer replies
// and any packet of the master or of a peer of the list count as a reply
// to the keepalive. A peer list replaces the peers, with OutputDown
// and ReasonList for each peer that it leaves out, and OutputPeers. The
// registration request and keepalive of a peer of the list get their
// replies, and its registration reply brings its link up with OutputUp.
// A de-registration request of the master or of a peer takes its link
// down with ReasonClosed. Any other packet of the master or of a peer of
// the list gives OutputData. The Peer drops a packet without a good
// trailer with Auth, and a packet of a sender it does not know. It
// allocates only when dst has no room for MaxOutputs outputs.
func (p *Peer) Packet(_ time.Duration, from Addr, b []byte, dst []Output) []Output {
	if p.state == StateIdle || !p.started {
		return dst
	}
	body, ok := p.l.open(b)
	if !ok {
		return dst
	}
	raw, err := ipsc.DecodeRaw(body)
	if err != nil {
		return dst
	}
	if raw.Opcode == ipsc.OpRegisterReply {
		return p.registered(from, &raw, body, dst)
	}
	if p.state == StateRunning && raw.ID == p.masterID {
		return p.fromMaster(from, &raw, body, dst)
	}
	i := p.l.find(raw.ID)
	if i < 0 || p.state != StateRunning {
		return dst
	}
	return p.fromPeer(i, from, &raw, body, dst)
}

func (p *Peer) registered(from Addr, raw *ipsc.Raw, body []byte, dst []Output) []Output {
	if _, err := ipsc.DecodeControl(body); err != nil || from != p.master {
		return dst
	}
	p.outstanding = 0
	if p.state == StateRunning && raw.ID == p.masterID {
		return dst
	}
	if p.state == StateRunning {
		dst = append(dst, Output{Kind: OutputDown, ID: p.masterID, Addr: p.master, Reason: ReasonClosed})
	}
	p.state, p.masterID, p.haveList = StateRunning, raw.ID, false
	return append(dst, Output{Kind: OutputUp, ID: raw.ID, Addr: from})
}

func (p *Peer) fromMaster(from Addr, raw *ipsc.Raw, body []byte, dst []Output) []Output {
	p.outstanding = 0
	switch raw.Opcode {
	case ipsc.OpPeerListReply:
		return p.peerList(body, dst)
	case ipsc.OpDeregisterRequest:
		if _, err := ipsc.DecodeControl(body); err != nil {
			return dst
		}
		p.state, p.outstanding, p.haveList = StateRegistering, 0, false
		return append(dst, Output{Kind: OutputDown, ID: raw.ID, Addr: from, Reason: ReasonClosed})
	case ipsc.OpPeerAliveRequest, ipsc.OpPeerRegisterRequest:
		return p.answer(raw.Opcode, body, from, dst)
	default:
	}
	if isLinkControl(raw.Opcode) {
		return dst
	}
	return append(dst, Output{Kind: OutputData, Packet: body, Addr: from, ID: raw.ID})
}

func (p *Peer) fromPeer(i int, from Addr, raw *ipsc.Raw, body []byte, dst []Output) []Output {
	e := &p.l.peers[i]
	switch raw.Opcode {
	case ipsc.OpPeerRegisterRequest:
		return p.answer(raw.Opcode, body, from, dst)
	case ipsc.OpPeerAliveRequest:
		c, err := ipsc.DecodeControl(body)
		if err != nil {
			return dst
		}
		e.outstanding, e.info.Mode, e.info.Flags = 0, c.Mode, c.Flags
		return p.answer(raw.Opcode, body, from, dst)
	case ipsc.OpPeerRegisterReply:
		if _, err := ipsc.DecodeControl(body); err != nil {
			return dst
		}
		e.outstanding = 0
		if !e.info.Up {
			e.info.Up = true
			dst = append(dst, Output{Kind: OutputUp, ID: e.info.ID, Addr: e.info.Addr})
		}
		return dst
	case ipsc.OpPeerAliveReply:
		e.outstanding = 0
		return dst
	case ipsc.OpDeregisterRequest:
		if _, err := ipsc.DecodeControl(body); err != nil {
			return dst
		}
		if e.info.Up {
			dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonClosed})
		}
		p.l.remove(i)
		return dst
	default:
	}
	if isLinkControl(raw.Opcode) {
		return dst
	}
	e.outstanding = 0
	return append(dst, Output{Kind: OutputData, Packet: body, Addr: from, ID: raw.ID})
}

func isLinkControl(op ipsc.Opcode) bool {
	return op >= ipsc.OpRegisterRequest && op <= ipsc.OpDeregisterReply || op == ipsc.OpStatus
}

func (p *Peer) answer(op ipsc.Opcode, body []byte, from Addr, dst []Output) []Output {
	if _, err := ipsc.DecodeControl(body); err != nil {
		return dst
	}
	if op == ipsc.OpPeerRegisterRequest {
		return send(dst, p.l.appendControl(p.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerRegisterReply, ID: p.l.id, Version: p.l.version}), from)
	}
	return send(dst, p.l.appendControl(p.tx[:0], ipsc.Control{Opcode: ipsc.OpPeerAliveReply, ID: p.l.id, Mode: p.l.mode, Flags: p.l.flags}), from)
}

func (p *Peer) peerList(body []byte, dst []Output) []Output {
	list, err := ipsc.PeerListOf(body)
	if err != nil {
		return dst
	}
	for i := 0; i < p.l.n; {
		e := &p.l.peers[i]
		pe, ok := listed(list, e.info.ID)
		addr := Addr{IP: pe.IP, Port: pe.Port}
		if e.info.Up && (!ok || e.info.Addr != addr) {
			e.info.Up = false
			dst = append(dst, Output{Kind: OutputDown, ID: e.info.ID, Addr: e.info.Addr, Reason: ReasonList})
		}
		if !ok {
			p.l.remove(i)
			continue
		}
		e.info.Addr, e.info.Mode = addr, pe.Mode
		i++
	}
	for j := range list.Len() {
		pe := list.Entry(j)
		if pe.ID == p.l.id || p.l.find(pe.ID) >= 0 {
			continue
		}
		i := p.l.add()
		if i < 0 {
			break
		}
		p.l.peers[i].info = PeerInfo{ID: pe.ID, Addr: Addr{IP: pe.IP, Port: pe.Port}, Mode: pe.Mode}
	}
	p.haveList = true
	return append(dst, Output{Kind: OutputPeers})
}

func listed(list ipsc.PeerList, id uint32) (ipsc.PeerEntry, bool) {
	for j := range list.Len() {
		if pe := list.Entry(j); pe.ID == id {
			return pe, true
		}
	}
	return ipsc.PeerEntry{}, false
}
