package ip

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/internal/text"
)

// ICMPMessage is the Type and Code of an ICMP message, as Type<<8 | Code
// (RFC 792). The named values are the ICMP messages that a radio supports
// at least (ETSI TS 102 361-3 clause 5.2, Table 5.1).
type ICMPMessage uint16

// The ICMPMessage values of Table 5.1.
const (
	// ICMPNetworkUnreachable is a Destination Unreachable: the final
	// destination of an IP message that a radio received is unreachable.
	ICMPNetworkUnreachable ICMPMessage = 3 << 8
	// ICMPHostUnreachable is a Destination Unreachable: the sender used all
	// its tries on air, the message overflows the queue of the recipient,
	// or it waited in the queue longer than the limit.
	ICMPHostUnreachable ICMPMessage = 3<<8 | 1
	// ICMPFragmentationNeeded is a Destination Unreachable: the IP message
	// is longer than the MTU of the accessory interface and has the DF flag
	// set.
	ICMPFragmentationNeeded ICMPMessage = 3<<8 | 4
	// ICMPNetworkUnknown is a Destination Unreachable: the IP message is for
	// a network class that the system does not support (RFC 1122 section
	// 3.2.2.1).
	ICMPNetworkUnknown ICMPMessage = 3<<8 | 6
	// ICMPBadHeader is a Parameter Problem: the IP header of the message is
	// not a valid IPv4 header. ICMPHeader.Pointer gives the octet in error.
	ICMPBadHeader ICMPMessage = 12 << 8
)

// Type returns the ICMP Type of m.
func (m ICMPMessage) Type() uint8 { return uint8(m >> 8) }

// Code returns the ICMP Code of m.
func (m ICMPMessage) Code() uint8 { return uint8(m & 0xFF) }

// String returns the Table 5.1 name of m. It allocates only for a value
// without a name.
func (m ICMPMessage) String() string {
	switch m {
	case ICMPNetworkUnreachable:
		return "Network Unreachable"
	case ICMPHostUnreachable:
		return "Host Unreachable"
	case ICMPFragmentationNeeded:
		return "Fragmentation Needed"
	case ICMPNetworkUnknown:
		return "Destination Network Unknown"
	case ICMPBadHeader:
		return "IP Header Bad"
	}
	return "ICMPMessage(type " + text.Int(int64(m.Type())) + ", code " + text.Int(int64(m.Code())) + ")"
}

// ICMPHeader is the 8-octet header of an ICMP error message (RFC 792, RFC
// 1191 section 4). (ETSI TS 102 361-3 clause 5.2)
//
//dmr:frame 8
type ICMPHeader struct {
	// Message is the Type and Code.
	Message ICMPMessage `dmr:"bits:0-15,enum"`
	// Checksum is the checksum over the ICMP message.
	Checksum uint16 `dmr:"bits:16-31"`
	// Pointer is the octet of the original IPv4 header in error, for
	// ICMPBadHeader. Other messages send 0.
	Pointer uint8 `dmr:"bits:32-39"`
	// Reserved is unused, 0.
	Reserved uint8 `dmr:"bits:40-47"`
	// NextHopMTU is the MTU of the next hop, for ICMPFragmentationNeeded.
	// Other messages send 0.
	NextHopMTU uint16 `dmr:"bits:48-63"`
}

// ICMP is an ICMP error message: the header and the start of the IPv4
// packet that caused it (RFC 792). (ETSI TS 102 361-3 clause 5.2)
//
//dmr:stream
type ICMP struct {
	// Header is the header.
	Header ICMPHeader `dmr:"frame"`
	// Original is the IPv4 header and the first 8 octets of the payload of
	// the packet in error.
	Original []byte `dmr:"rest"`
}

// ICMPPacket is an ICMP error message in an IPv4 packet, the user data of
// a packet with the SAP IP based packet data.
type ICMPPacket struct {
	// Source and Destination are the IPv4 addresses of the packet.
	Source, Destination [4]byte
	// ID is the IPv4 Identification field.
	ID uint16
	// TTL is the IPv4 time to live.
	TTL uint8
	// Message is the ICMP Type and Code.
	Message ICMPMessage
	// Pointer is the octet in error of ICMPBadHeader.
	Pointer uint8
	// NextHopMTU is the MTU of ICMPFragmentationNeeded.
	NextHopMTU uint16
	// Original is the start of the IPv4 packet in error. It points into
	// the data that DecodeICMPPacket got.
	Original []byte
}

// DecodeICMPPacket decodes the ICMP message in the IPv4 packet at the
// start of data. It returns ErrShort, ErrLength, ErrVersion or ErrFragment
// for the IPv4 header, and ErrProtocol for a packet that does not carry
// ICMP. When a checksum does not match it returns the ICMPPacket with
// ErrChecksum. Octets after the length that the IPv4 header gives are
// ignored. It does not allocate. (ETSI TS 102 361-3 clause 5.2)
func DecodeICMPPacket(data []byte) (ICMPPacket, error) {
	if len(data) > 0 && data[0]>>4 != 4 {
		return ICMPPacket{}, ErrVersion
	}
	p, err := DecodeIPv4(data)
	if err != nil {
		return ICMPPacket{}, err
	}
	h := &p.Header
	switch {
	case h.IHL < 5:
		return ICMPPacket{}, ErrLength
	case h.Protocol != ProtocolICMP:
		return ICMPPacket{}, ErrProtocol
	case h.MoreFragments || h.FragmentOffset != 0:
		return ICMPPacket{}, ErrFragment
	}
	m, err := DecodeICMP(p.Payload)
	if err != nil {
		return ICMPPacket{}, err
	}
	c := ICMPPacket{
		Source: h.Source, Destination: h.Destination, ID: h.ID, TTL: h.TTL,
		Message: m.Header.Message, Pointer: m.Header.Pointer, NextHopMTU: m.Header.NextHopMTU,
		Original: m.Original,
	}
	if checksum(0, data[:int(h.IHL)*4]) != 0xFFFF || checksum(0, p.Payload) != 0xFFFF {
		return c, ErrChecksum
	}
	return c, nil
}

// AppendICMPPacket appends c to dst as an IPv4 packet with an ICMP message,
// and computes both checksums. The header has no options, and a TTL of 0
// is sent as 64. Of an Original that starts with an IPv4 header it sends
// the header and the first 8 octets of the payload, as RFC 792 says. It
// returns dst unchanged and ErrNoICMP when RFC 1122 clause 3.2.2 forbids
// an ICMP error message about Original: an ICMP error message (Type 3, 4,
// 5, 11 or 12, or an ICMP Type that Original does not hold), a packet to
// the broadcast address 255.255.255.255 or a multicast address, a fragment
// other than the first, or a packet from an address that is not one host
// (0.0.0.0/8, 127.0.0.0/8, multicast or class E). An ICMP error message
// about another ICMP message, such as an Echo Request, is allowed. The
// Original does not tell whether a packet came as a link layer broadcast,
// a DMR group call; data.Output.ICMP and data.Event.ICMP refuse those. It
// returns another error when the
// packet is longer than 65535 octets. It allocates only to grow dst.
// (ETSI TS 102 361-3 clause 5.2)
func AppendICMPPacket(dst []byte, c *ICMPPacket) ([]byte, error) {
	if c == nil {
		return dst, errNilICMP
	}
	orig := c.Original
	if len(orig) >= 20 && orig[0]>>4 == 4 {
		if !errorAllowed(orig) {
			return dst, ErrNoICMP
		}
		orig = orig[:min(len(orig), int(orig[0]&0xF)*4+8)]
	}
	n := len(dst)
	m := ICMP{Header: ICMPHeader{Message: c.Message, Pointer: c.Pointer, NextHopMTU: c.NextHopMTU}, Original: orig}
	dst, err := m.AppendBinary(dst)
	if err != nil {
		return dst[:n], err
	}
	ttl := c.TTL
	if ttl == 0 {
		ttl = 64
	}
	p := IPv4{
		Header: IPv4Header{
			Version: 4, IHL: 5, ID: c.ID, TTL: ttl, Protocol: ProtocolICMP,
			Source: c.Source, Destination: c.Destination,
		},
		Payload: dst[n:],
	}
	k := len(dst)
	dst, err = p.AppendBinary(dst)
	if err != nil {
		return dst[:n], err
	}
	copy(dst[n:], dst[k:])
	dst = dst[:n+len(dst)-k]
	binary.BigEndian.PutUint16(dst[n+10:], ^checksum(0, dst[n:n+20]))
	binary.BigEndian.PutUint16(dst[n+22:], ^checksum(0, dst[n+20:]))
	return dst, nil
}

// RFC 1122 clause 3.2.2
func errorAllowed(orig []byte) bool {
	src, dst := orig[12], orig[16]
	switch {
	case binary.BigEndian.Uint16(orig[6:])&0x1FFF != 0:
		return false
	case dst >= 224 && dst < 240, dst == 255 && orig[17] == 255 && orig[18] == 255 && orig[19] == 255:
		return false
	case src == 0, src == 127, src >= 224:
		return false
	case Protocol(orig[9]) != ProtocolICMP:
		return true
	}
	hl := int(orig[0]&0xF) * 4
	if hl < 20 || len(orig) <= hl {
		return false
	}
	switch orig[hl] {
	case 3, 4, 5, 11, 12:
		return false
	}
	return true
}

// CheckIPv4 returns the ICMP error message that a radio sends for the
// IPv4 packet data that it received for an accessory interface with the
// MTU mtu: ICMPBadHeader with the octet in error when the header is not a
// valid IPv4 header, or ICMPFragmentationNeeded when the packet is longer
// than mtu and has the DF flag set. ok is false when the packet needs no
// error message. A mtu of 0 or less skips the MTU check. It does not
// allocate. (ETSI TS 102 361-3 clause 5.2, Table 5.1)
func CheckIPv4(data []byte, mtu int) (m ICMPMessage, pointer uint8, ok bool) {
	if len(data) < 20 || data[0]>>4 != 4 || data[0]&0xF < 5 {
		return ICMPBadHeader, 0, true
	}
	hl := int(data[0]&0xF) * 4
	total := int(binary.BigEndian.Uint16(data[2:]))
	switch {
	case hl > len(data):
		return ICMPBadHeader, 0, true
	case total < hl || total > len(data):
		return ICMPBadHeader, 2, true
	case checksum(0, data[:hl]) != 0xFFFF:
		return ICMPBadHeader, 10, true
	case mtu > 0 && total > mtu && data[6]&0x40 != 0:
		return ICMPFragmentationNeeded, 0, true
	}
	return 0, 0, false
}
