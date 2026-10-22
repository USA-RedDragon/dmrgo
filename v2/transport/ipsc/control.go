package ipsc

import "github.com/USA-RedDragon/dmrgo/v2"

// Control is a peer control packet, opcodes 0x90 to 0x9B. Fields that the
// opcode does not carry are zero.
type Control struct {
	// Opcode is the packet.
	Opcode Opcode
	// ID is the peer or master ID of the sender.
	ID uint32
	// Mode is the mode of the sender (0x90, 0x91, 0x96 to 0x99).
	Mode Mode
	// Flags is the flags of the sender (0x90, 0x91, 0x96 to 0x99).
	Flags Flags
	// Peers is the number of peers in an OpRegisterReply.
	Peers uint16
	// ListLen is the length in octets of the peer list of an
	// OpPeerListReply, 11 for each peer.
	ListLen uint16
	// Version is the protocol version (0x90, 0x91, 0x94 to 0x97), for
	// example 04 02 04 01.
	Version [4]byte
}

// DecodeControl decodes the peer control packet p, without the
// authentication trailer (see package auth). An OpPeerListReply decodes
// its length; PeerListOf reads its entries. It returns ErrCommand for
// another opcode, and ErrShort or ErrLength for a length that does not
// match the opcode. It does not allocate.
func DecodeControl(p []byte) (Control, error) {
	if len(p) == 0 {
		return Control{}, ErrShort
	}
	switch op := Opcode(p[0]); op {
	case OpRegisterRequest, OpAliveRequest, OpAliveReply:
		m, err := decodeCtlFull(p)
		if err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: m.ID, Mode: m.Mode, Flags: m.Flags, Version: m.Version}, nil
	case OpRegisterReply:
		m, err := decodeCtlRegisterReply(p)
		if err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: m.ID, Mode: m.Mode, Flags: m.Flags, Peers: m.Peers, Version: m.Version}, nil
	case OpPeerListRequest, OpDeregisterRequest, OpDeregisterReply:
		m, err := decodeCtlID(p)
		if err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: m.ID}, nil
	case OpPeerRegisterRequest, OpPeerRegisterReply:
		m, err := decodeCtlVersion(p)
		if err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: m.ID, Version: m.Version}, nil
	case OpPeerAliveRequest, OpPeerAliveReply:
		m, err := decodeCtlModeFlags(p)
		if err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: m.ID, Mode: m.Mode, Flags: m.Flags}, nil
	case OpPeerListReply:
		if _, err := PeerListOf(p); err != nil {
			return Control{}, err
		}
		return Control{Opcode: op, ID: uint32(p[1])<<24 | uint32(p[2])<<16 | uint32(p[3])<<8 | uint32(p[4]), ListLen: uint16(p[5])<<8 | uint16(p[6])}, nil
	default:
	}
	return Control{}, ErrCommand
}

// AppendBinary appends the packet c to dst, without the authentication
// trailer. An OpPeerListReply appends only its header, with ListLen; use
// AppendPeerListReply for the entries. It returns dst unchanged and an
// error for an opcode that is not a peer control packet, and
// dmrgo.ErrNilReceiver for a nil c. It allocates only to grow dst.
func (c *Control) AppendBinary(dst []byte) ([]byte, error) {
	if c == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	return c.appendBinary(dst)
}

func (c Control) appendBinary(dst []byte) ([]byte, error) {
	switch c.Opcode {
	case OpRegisterRequest, OpAliveRequest, OpAliveReply:
		m := ctlFull{Opcode: c.Opcode, ID: c.ID, Mode: c.Mode, Flags: c.Flags, Version: c.Version}
		return m.AppendBinary(dst)
	case OpRegisterReply:
		m := ctlRegisterReply{Opcode: c.Opcode, ID: c.ID, Mode: c.Mode, Flags: c.Flags, Peers: c.Peers, Version: c.Version}
		return m.AppendBinary(dst)
	case OpPeerListRequest, OpDeregisterRequest, OpDeregisterReply:
		m := ctlID{Opcode: c.Opcode, ID: c.ID}
		return m.AppendBinary(dst)
	case OpPeerRegisterRequest, OpPeerRegisterReply:
		m := ctlVersion{Opcode: c.Opcode, ID: c.ID, Version: c.Version}
		return m.AppendBinary(dst)
	case OpPeerAliveRequest, OpPeerAliveReply:
		m := ctlModeFlags{Opcode: c.Opcode, ID: c.ID, Mode: c.Mode, Flags: c.Flags}
		return m.AppendBinary(dst)
	case OpPeerListReply:
		return append(dst, byte(OpPeerListReply), byte(c.ID>>24&0xFF), byte(c.ID>>16&0xFF), byte(c.ID>>8&0xFF), byte(c.ID&0xFF),
			byte(c.ListLen>>8), byte(c.ListLen&0xFF)), nil
	default:
	}
	return dst, errOpcode
}

// PeerEntry is one peer of a peer list.
type PeerEntry struct {
	// ID is the peer ID.
	ID uint32
	// IP is the IPv4 address of the peer.
	IP [4]byte
	// Port is the UDP port of the peer.
	Port uint16
	// Mode is the mode of the peer.
	Mode Mode
}

// PeerList is the entries of an OpPeerListReply, 11 octets each, as a view
// over the packet.
type PeerList struct {
	b []byte
}

// PeerListOf returns the entries of the OpPeerListReply p without a copy.
// It returns ErrCommand for another opcode, ErrShort, and ErrLength when the
// length field does not match the packet or is not a multiple of 11.
func PeerListOf(p []byte) (PeerList, error) {
	if len(p) < 7 {
		return PeerList{}, ErrShort
	}
	if Opcode(p[0]) != OpPeerListReply {
		return PeerList{}, ErrCommand
	}
	n := int(p[5])<<8 | int(p[6])
	if len(p) != 7+n || n%11 != 0 {
		return PeerList{}, ErrLength
	}
	return PeerList{b: p[7:]}, nil
}

// Len returns the number of entries, 0 for a nil l.
func (l *PeerList) Len() int {
	if l == nil {
		return 0
	}
	return len(l.b) / 11
}

// Entry returns entry i, or the zero PeerEntry when i is not below Len.
func (l *PeerList) Entry(i int) PeerEntry {
	if i < 0 || i >= l.Len() {
		return PeerEntry{}
	}
	m, _ := decodePeerEntry(l.b[11*i : 11*i+11])
	return PeerEntry(m)
}

// AppendPeerListReply appends an OpPeerListReply of master with entries to
// dst. It allocates only to grow dst; entries beyond what the 16-bit length
// holds (5957) are left out.
func AppendPeerListReply(dst []byte, master uint32, entries []PeerEntry) []byte {
	n := min(len(entries), 0xFFFF/11)
	size := 11 * n
	dst = append(dst, byte(OpPeerListReply), byte(master>>24&0xFF), byte(master>>16&0xFF), byte(master>>8&0xFF), byte(master&0xFF),
		byte(size>>8&0xFF), byte(size&0xFF))
	for i := range n {
		e := peerEntry(entries[i])
		dst, _ = e.AppendBinary(dst)
	}
	return dst
}

// Raw is a packet that this package does not decode further, such as
// OpWakeup, OpStatus or OpXNL: its opcode, sender ID and the octets after
// them.
type Raw struct {
	// Opcode is the packet.
	Opcode Opcode
	// ID is octets 1 to 4, the sender ID.
	ID uint32
	// Body is the rest of the packet. A decode points it into the packet.
	Body []byte
}

// DecodeRaw splits p into its opcode, sender ID and body. It returns
// ErrShort for a packet shorter than 5 octets.
func DecodeRaw(p []byte) (Raw, error) {
	m, err := decodeCtlID(p[:min(len(p), 5)])
	if err != nil {
		return Raw{}, err
	}
	return Raw{Opcode: m.Opcode, ID: m.ID, Body: p[5:]}, nil
}

// AppendBinary appends r to dst. A nil r appends nothing. It allocates
// only to grow dst.
func (r *Raw) AppendBinary(dst []byte) []byte {
	if r == nil {
		return dst
	}
	m := ctlID{Opcode: r.Opcode, ID: r.ID}
	dst, _ = m.AppendBinary(dst)
	return append(dst, r.Body...)
}
