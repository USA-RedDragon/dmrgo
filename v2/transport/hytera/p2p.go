package hytera

// HeartbeatLen is the length of the heartbeat of the P2P service in octets.
const HeartbeatLen = 20

// Heartbeat is the 20-octet "ZZZZ" heartbeat of the P2P service, or the
// 1-octet keepalive that each side sends on the DMR and RDAC services.
type Heartbeat struct {
	// Keepalive reports the 1-octet keepalive. Only Code is then used.
	Keepalive bool
	// Code is octet 4: 0x0A on a heartbeat (0x0C is an acknowledgement in
	// Hytera_Homebrew_Bridge), or the octet of a keepalive: 0x00 from a
	// repeater, 0x41 from Hytera_Homebrew_Bridge.
	Code uint8
	// Seq is octet 12: 0x5A from the repeater and 0x5B in the reply of the
	// master in the DO1KBL capture; Hytera_Homebrew_Bridge replies 0xFF.
	Seq uint8
	// Status is octets 13 to 15: 5A 59 5A, or 5A 01 5A from
	// Hytera_Homebrew_Bridge.
	Status [3]byte
	// Reserved is octets 5 to 11 and 16 to 19: 00 00 00 14 00 00 00 and 00
	// 00 00 00 in every capture.
	Reserved [11]byte
}

// DecodeHeartbeat decodes a heartbeat (20 octets) or a keepalive (1
// octet). It returns ErrShort, ErrLength, or ErrSignature when a heartbeat
// does not start with "ZZZZ".
func DecodeHeartbeat(p []byte) (Heartbeat, error) {
	if len(p) == 1 {
		return Heartbeat{Keepalive: true, Code: p[0]}, nil
	}
	l, err := decodeHeartbeatLayout(p)
	if err != nil {
		return Heartbeat{}, err
	}
	h := Heartbeat{Code: l.Code, Seq: l.Seq, Status: l.Status}
	copy(h.Reserved[:7], l.R5[:])
	copy(h.Reserved[7:], l.R16[:])
	return h, nil
}

// AppendBinary appends the heartbeat, or the keepalive, to dst. It
// allocates only when dst has no room for HeartbeatLen more octets.
func (h Heartbeat) AppendBinary(dst []byte) ([]byte, error) {
	if h.Keepalive {
		return append(dst, h.Code), nil
	}
	l := heartbeatLayout{Code: h.Code, Seq: h.Seq, Status: h.Status}
	copy(l.R5[:], h.Reserved[:7])
	copy(l.R16[:], h.Reserved[7:])
	return l.AppendBinary(dst)
}

// P2PHeaderLen is the length of a P2P command without its body, in octets.
const P2PHeaderLen = 20

// P2P is a "P2P" command of the P2P service: a repeater asks the master for
// registration or for the DMR or RDAC service, the master accepts it and
// redirects the repeater to the port of the service, and the repeater
// acknowledges the redirect. The layout comes from the Hytera_Homebrew_Bridge
// code and its captures with a real repeater.
type P2P struct {
	// Op is octet 3: 0x38 from the repeater, 0x50 from the master.
	Op uint8
	// Code is octet 4: 0x01 on a request, one more on its accept, 0x0B on
	// a redirect, 0x0C on an acknowledgement.
	Code uint8
	// Seq is octet 12, a number that the repeater counts up for each
	// request; an acknowledgement has the Seq of the redirect.
	Seq uint8
	// Status is octets 13 to 15: FF 11 02 or 01 11 02 on a request, 01 01
	// 5A on an accept, FF 01 00 on a redirect, 01 59 5A on an
	// acknowledgement.
	Status [3]byte
	// Reserved is octets 5 to 11 and 16 to 19: 00 00 00 14 00 00 00 and 00
	// 00 00 00 in every capture.
	Reserved [11]byte
	// Body is the octets after octet 19: the service, 00 00 00, and then 01
	// on an accept or FF 01 and the port on a redirect. An acknowledgement
	// has none. A decode points it into the packet.
	Body []byte
}

// DecodeP2P decodes the P2P command p. Body points into p. It returns
// ErrShort, or ErrSignature when p does not start with "P2P".
func DecodeP2P(p []byte) (P2P, error) {
	if len(p) < P2PHeaderLen {
		return P2P{}, ErrShort
	}
	l, err := decodeP2pLayout(p[:P2PHeaderLen])
	if err != nil {
		return P2P{}, err
	}
	c := P2P{Op: l.Op, Code: l.Code, Seq: l.Seq, Status: l.Status, Body: p[P2PHeaderLen:]}
	copy(c.Reserved[:7], l.R5[:])
	copy(c.Reserved[7:], l.R16[:])
	return c, nil
}

// AppendBinary appends the command and its body to dst. It allocates only
// when dst has no room for P2PHeaderLen+len(c.Body) more octets.
func (c P2P) AppendBinary(dst []byte) ([]byte, error) {
	l := p2pLayout{Op: c.Op, Code: c.Code, Seq: c.Seq, Status: c.Status}
	copy(l.R5[:], c.Reserved[:7])
	copy(l.R16[:], c.Reserved[7:])
	dst, err := l.AppendBinary(dst)
	if err != nil {
		return dst, err
	}
	return append(dst, c.Body...), nil
}

// Service returns octet 20, the service that the command is about. ok is
// false for a body shorter than 4 octets, such as that of an
// acknowledgement.
func (c P2P) Service() (s Service, ok bool) {
	if len(c.Body) < 4 {
		return 0, false
	}
	return Service(c.Body[0]), true
}

// RedirectPort returns the UDP port of a redirect: a body that ends in FF
// 01 and the port, little-endian. ok is false for another body.
func (c P2P) RedirectPort() (port uint16, ok bool) {
	if len(c.Body) != 8 || c.Body[4] != 0xFF || c.Body[5] != 0x01 {
		return 0, false
	}
	return uint16(c.Body[6]) | uint16(c.Body[7])<<8, true
}
