package ip

import "errors"

// Errors returned by the package. Each is a static value, so a decode that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a header or before the
	// length that a header gives.
	ErrShort = errors.New("ip: packet too short")
	// ErrLength is returned when a length field does not match the data,
	// such as an IHL under 5 or a UDP length beyond the IPv4 packet.
	ErrLength = errors.New("ip: length does not match the packet")
	// ErrVersion is returned for an IP version other than 4.
	ErrVersion = errors.New("ip: not an IPv4 packet")
	// ErrProtocol is returned for an IPv4 packet that does not carry UDP,
	// or ICMP for DecodeICMPPacket, and by AppendICMPPacket for an ICMP
	// message about an ICMP message.
	ErrProtocol = errors.New("ip: packet carries another protocol")
	// ErrFragment is returned for a fragment of an IPv4 packet.
	ErrFragment = errors.New("ip: packet is a fragment")
	// ErrSAP is returned for a SAP other than IP based packet data or UDP/IP
	// header compression.
	ErrSAP = errors.New("ip: SAP does not carry IP")
	// ErrChecksum is returned with the Datagram when the IPv4 header
	// checksum or the UDP checksum does not match.
	ErrChecksum = errors.New("ip: checksum does not match")
)
