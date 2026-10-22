package ipscsession

import (
	"math"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc"
	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
)

// The defaults of the config fields.
const (
	// AliveDefault is the default PeerConfig.Alive, the ALIVE_TIMER of
	// DMRlink.
	AliveDefault = 5 * time.Second
	// MaxMissedDefault is the default PeerConfig.MaxMissed, the
	// MAX_MISSED of DMRlink.
	MaxMissedDefault = 20
	// TimeoutDefault is the default MasterConfig.Timeout, after which
	// DMRlink drops a silent peer.
	TimeoutDefault = 120 * time.Second
	// MaxPeersDefault is the default MaxPeers: an IPSC system has at most
	// 15 peers.
	MaxPeersDefault = 15
	// ModeDefault is the mode that a Peer or Master sends when its config
	// leaves Mode zero: operational, digital, and both slots linked.
	ModeDefault ipsc.Mode = 0x6A
)

// VersionDefault returns the protocol version that a Peer or Master sends
// when its config leaves Version zero, as DMRlink sends it.
func VersionDefault() [4]byte {
	return [4]byte{0x04, 0x02, 0x04, 0x01}
}

// MaxPacket is the longest packet in octets that the machines take or
// send, with its authentication trailer. They drop a longer packet.
const MaxPacket = 1500

// MaxControl is the longest link control packet in octets other than a
// peer list, with its authentication trailer.
const MaxControl = 16 + ipscauth.TrailerLen

// MaxPeersLimit is the largest MaxPeers: the most peers whose peer list,
// with its authentication trailer, fits in MaxPacket.
const MaxPeersLimit = (MaxPacket - 7 - ipscauth.TrailerLen) / 11

// Addr is the IPv4 address and UDP port of an IPSC peer, as a peer list
// carries it.
type Addr struct {
	// IP is the IPv4 address.
	IP [4]byte
	// Port is the UDP port.
	Port uint16
}

// OutputKind is the kind of an Output.
type OutputKind uint8

// The OutputKind values.
const (
	// OutputSend is a packet in Output.Packet to send to Output.Addr, with
	// its authentication trailer when the config has Auth.
	OutputSend OutputKind = iota + 1
	// OutputUp reports that the link with Output.ID at Output.Addr is up:
	// at a Peer, the registration with the master or with another peer;
	// at a Master, the registration of a peer.
	OutputUp
	// OutputDown reports that the link with Output.ID is down.
	// Output.Reason tells why.
	OutputDown
	// OutputPeers reports a new peer list at a Peer, which Peers and
	// PeerAt return.
	OutputPeers
	// OutputData is a packet other than a link control packet, such as a
	// voice or data packet, from Output.ID at Output.Addr, in
	// Output.Packet without the authentication trailer.
	OutputData
	// OutputDropped reports a packet from Output.Addr that a Master
	// dropped, with the sender ID in Output.ID when the packet has one.
	// Output.Reason tells why.
	OutputDropped
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
	case OutputPeers:
		return "Peers"
	case OutputData:
		return "Data"
	case OutputDropped:
		return "Dropped"
	}
	return unknownName
}

// Reason tells why a link went down.
type Reason uint8

// The Reason values.
const (
	// ReasonNone is the zero Reason.
	ReasonNone Reason = iota
	// ReasonTimeout is MaxMissed keepalives without a reply at a Peer, or
	// Timeout without a packet at a Master.
	ReasonTimeout
	// ReasonClosed is a de-registration request of the other side.
	ReasonClosed
	// ReasonStopped is a Close of the consumer.
	ReasonStopped
	// ReasonList is a peer that a new peer list left out.
	ReasonList
	// ReasonAuth is a packet without a good authentication trailer, with
	// Auth.
	ReasonAuth
	// ReasonUnregistered is a packet that needs the registration of its
	// sender, from a peer that is not registered.
	ReasonUnregistered
	// ReasonMalformed is a packet that is empty, longer than MaxPacket or
	// does not decode as its opcode.
	ReasonMalformed
	// ReasonFull is a registration that MaxPeers leaves no room for.
	ReasonFull
	// ReasonAddress is a de-registration request from an address other
	// than the one the peer registered from.
	ReasonAddress
)

// String returns the name of the Reason.
func (r Reason) String() string {
	switch r {
	case ReasonNone:
		return "None"
	case ReasonTimeout:
		return "Timeout"
	case ReasonClosed:
		return "Closed"
	case ReasonStopped:
		return "Stopped"
	case ReasonList:
		return "List"
	case ReasonAuth:
		return "Auth"
	case ReasonUnregistered:
		return "Unregistered"
	case ReasonMalformed:
		return "Malformed"
	case ReasonFull:
		return "Full"
	case ReasonAddress:
		return "Address"
	}
	return unknownName
}

// State is the state of a Peer or Master.
type State uint8

// The State values.
const (
	// StateIdle is a Peer or Master after Close.
	StateIdle State = iota
	// StateRegistering is a Peer that is not registered with the master.
	StateRegistering
	// StateRunning is a registered Peer, or a running Master.
	StateRunning
)

// String returns the name of the State.
func (s State) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateRegistering:
		return "Registering"
	case StateRunning:
		return "Running"
	}
	return unknownName
}

const unknownName = "Unknown"

// Output is one result of a Peer or Master. Fields that do not apply to
// Kind are zero.
type Output struct {
	// Kind is the kind of the output.
	Kind OutputKind
	// Packet is the packet of OutputSend and OutputData. It holds at most
	// MaxPacket octets.
	Packet []byte
	// Addr is where to send OutputSend, and the address of the peer or
	// master of every other output but OutputPeers.
	Addr Addr
	// ID is the peer or master ID of OutputUp, OutputDown and OutputData,
	// and of OutputDropped when the packet has one.
	ID uint32
	// Reason tells why for OutputDown and OutputDropped.
	Reason Reason
}

// PeerInfo is one peer that a Peer or Master knows.
type PeerInfo struct {
	// ID is the peer ID.
	ID uint32
	// Addr is the address of the peer.
	Addr Addr
	// Mode is the last mode of the peer.
	Mode ipsc.Mode
	// Flags is the last flags of the peer, zero when not known.
	Flags ipsc.Flags
	// Up reports whether the link with the peer is up.
	Up bool
}

type entry struct {
	info        PeerInfo
	last        time.Duration
	outstanding int
	keyed       bool
	key         [20]byte
	bye         [5 + ipscauth.TrailerLen]byte
}

func (e *entry) keyOf() *[20]byte {
	if !e.keyed {
		return nil
	}
	return &e.key
}

type noCopy struct{}

func (*noCopy) Lock() {}

func (*noCopy) Unlock() {}

type link struct {
	id      uint32
	mode    ipsc.Mode
	flags   ipsc.Flags
	version [4]byte
	key     *ipscauth.Key
	peers   []entry
	n       int
	list    []ipsc.PeerEntry
	mac     digest.HMACSHA1
}

func newLink(id uint32, mode ipsc.Mode, flags ipsc.Flags, version [4]byte, useAuth bool, key [20]byte, maxPeers int) link {
	l := link{id: id, mode: mode, flags: flags, version: version}
	if l.mode == 0 {
		l.mode = ModeDefault
	}
	if l.version == [4]byte{} {
		l.version = VersionDefault()
	}
	if useAuth {
		l.key = ipscauth.NewKey(key)
		l.flags.Set(ipsc.FlagAuth, true)
	}
	l.peers = make([]entry, maxPeers)
	l.list = make([]ipsc.PeerEntry, maxPeers)
	return l
}

func (l *link) open(p []byte) ([]byte, Reason) {
	if len(p) > MaxPacket {
		return nil, ReasonMalformed
	}
	if l.key == nil {
		if len(p) == 0 {
			return nil, ReasonMalformed
		}
		return p, ReasonNone
	}
	body, ok := l.key.Verify(p)
	if !ok {
		return nil, ReasonAuth
	}
	if len(body) == 0 {
		return nil, ReasonMalformed
	}
	return body, ReasonNone
}

func (l *link) openWith(p []byte, key *[20]byte) ([]byte, Reason) {
	if key == nil {
		return l.open(p)
	}
	if len(p) > MaxPacket {
		return nil, ReasonMalformed
	}
	if len(p) < ipscauth.TrailerLen {
		return nil, ReasonAuth
	}
	body := p[:len(p)-ipscauth.TrailerLen]
	t := l.trailer(body, key)
	if !digest.Equal(t[:ipscauth.TrailerLen], p[len(body):]) {
		return nil, ReasonAuth
	}
	if len(body) == 0 {
		return nil, ReasonMalformed
	}
	return body, ReasonNone
}

func (l *link) trailer(p []byte, key *[20]byte) [20]byte {
	l.mac.Init(key[:])
	l.mac.Reset()
	l.mac.Write(p)
	return l.mac.Sum()
}

func dropped(dst []Output, from Addr, id uint32, r Reason) []Output {
	return append(dst, Output{Kind: OutputDropped, Addr: from, ID: id, Reason: r})
}

func (l *link) sign(dst []byte, key *[20]byte) []byte {
	if key != nil {
		t := l.trailer(dst, key)
		return append(dst, t[:ipscauth.TrailerLen]...)
	}
	if l.key == nil {
		return dst
	}
	out, _ := l.key.Sign(dst)
	return out
}

func (l *link) appendControl(dst []byte, c ipsc.Control, key *[20]byte) []byte {
	b, err := c.AppendBinary(dst)
	if err != nil {
		return dst
	}
	return l.sign(b, key)
}

func (l *link) appendPeerList(dst []byte) []byte {
	k := 0
	for i := range l.n {
		e := &l.peers[i]
		if e.keyed {
			continue
		}
		l.list[k] = ipsc.PeerEntry{ID: e.info.ID, IP: e.info.Addr.IP, Port: e.info.Addr.Port, Mode: e.info.Mode}
		k++
	}
	return l.sign(ipsc.AppendPeerListReply(dst, l.id, l.list[:k]), nil)
}

func (l *link) appendOwnList(dst []byte, i int) []byte {
	e := &l.peers[i]
	l.list[0] = ipsc.PeerEntry{ID: e.info.ID, IP: e.info.Addr.IP, Port: e.info.Addr.Port, Mode: e.info.Mode}
	return l.sign(ipsc.AppendPeerListReply(dst, l.id, l.list[:1]), e.keyOf())
}

func (l *link) shared() int {
	k := 0
	for i := range l.n {
		if !l.peers[i].keyed {
			k++
		}
	}
	return k
}

func (l *link) find(id uint32) int {
	for i := range l.n {
		if l.peers[i].info.ID == id {
			return i
		}
	}
	return -1
}

func (l *link) add() int {
	if l.n == len(l.peers) {
		return -1
	}
	l.peers[l.n] = entry{}
	l.n++
	return l.n - 1
}

func (l *link) remove(i int) {
	copy(l.peers[i:l.n], l.peers[i+1:l.n])
	l.n--
}

func send(dst []Output, p []byte, to Addr) []Output {
	if len(p) == 0 {
		return dst
	}
	return append(dst, Output{Kind: OutputSend, Packet: p, Addr: to})
}

func orDefault(d, def time.Duration) time.Duration {
	if d == 0 {
		return def
	}
	return d
}

func after(t, d time.Duration) time.Duration {
	if d > 0 && t > math.MaxInt64-d {
		return math.MaxInt64
	}
	return t + d
}
