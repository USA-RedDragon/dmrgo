package ip

import (
	"encoding/binary"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// DefaultNetwork is the network ID that radios use by default for the IPv4
// address of a radio: 12.x.y.z, with the 24-bit radio ID as x.y.z.
const DefaultNetwork = 12

// Datagram is a UDP datagram of a DMR data packet, from an IPv4 header (SAP
// IP based packet data) or a compressed UDP/IPv4 header (SAP UDP/IP header
// compression).
type Datagram struct {
	// Source and Destination are the IPv4 addresses. A compressed header
	// does not carry them, and they are zero.
	Source, Destination [4]byte
	// SourcePort and DestinationPort are the UDP ports.
	SourcePort, DestinationPort Port
	// ID is the IPv4 Identification field.
	ID uint16
	// TTL is the IPv4 time to live, 0 for a compressed header.
	TTL uint8
	// Compressed reports a compressed UDP/IPv4 header.
	Compressed bool
	// SAID and DAID are the address identifiers of a compressed header.
	SAID enums.SAID
	DAID enums.DAID
	// Payload is the application data. It points into the data that
	// Decode got.
	Payload []byte
}

// Decode decodes the UDP datagram at the start of data, the user data of a
// packet with the given SAP. It returns ErrSAP for a SAP that does not
// carry IP, and for an IPv4 header ErrShort, ErrLength, ErrVersion,
// ErrProtocol or ErrFragment. When a checksum does not match it returns
// the Datagram with ErrChecksum. Octets after the length that the IPv4
// header gives are ignored, as DMR pads the last block. It does not
// allocate. (ETSI TS 102 361-3 clauses 5.5 and 5.6)
func Decode(sap enums.ServiceAccessPointID, data []byte) (Datagram, error) {
	switch sap {
	case enums.SAPIPBasedPacketData:
		return decodeIPv4(data)
	case enums.SAPUDPIPHeaderCompression:
		return decodeCompressed(data)
	case enums.SAPUnifiedDataTransport, enums.SAPTCPIPHeaderCompression, enums.SAPAddressResolutionProtocol,
		enums.SAPProprietaryPacketData, enums.SAPShortData:
	}
	return Datagram{}, ErrSAP
}

func decodeIPv4(data []byte) (Datagram, error) {
	if len(data) > 0 && data[0]>>4 != 4 {
		return Datagram{}, ErrVersion
	}
	p, err := DecodeIPv4(data)
	if err != nil {
		return Datagram{}, err
	}
	h := &p.Header
	switch {
	case h.IHL < 5:
		return Datagram{}, ErrLength
	case h.Protocol != ProtocolUDP:
		return Datagram{}, ErrProtocol
	case h.MoreFragments || h.FragmentOffset != 0:
		return Datagram{}, ErrFragment
	}
	u, err := DecodeUDP(p.Payload)
	if err != nil {
		return Datagram{}, err
	}
	d := Datagram{
		Source:          h.Source,
		Destination:     h.Destination,
		SourcePort:      u.Header.SourcePort,
		DestinationPort: u.Header.DestinationPort,
		ID:              h.ID,
		TTL:             h.TTL,
		Payload:         u.Payload,
	}
	hl := int(h.IHL) * 4
	if checksum(0, data[:hl]) != 0xFFFF {
		return d, ErrChecksum
	}
	if u.Header.Checksum != 0 && udpSum(h.Source, h.Destination, p.Payload) != 0xFFFF {
		return d, ErrChecksum
	}
	return d, nil
}

func decodeCompressed(data []byte) (Datagram, error) {
	if len(data) < 5 {
		return Datagram{}, ErrShort
	}
	var raw [9]byte
	copy(raw[:], data)
	h := pdu.DecodeUDPIPv4CompressedHeader(raw)
	n := h.Octets()
	if len(data) < n {
		return Datagram{}, ErrShort
	}
	src, dst := h.Ports()
	return Datagram{
		SourcePort:      Port(src),
		DestinationPort: Port(dst),
		ID:              h.IPv4Identification,
		Compressed:      true,
		SAID:            h.SAID,
		DAID:            h.DAID,
		Payload:         data[n:],
	}, nil
}

func checksum(sum uint32, b []byte) uint16 {
	for i := 0; i+1 < len(b); i += 2 {
		sum += uint32(b[i])<<8 | uint32(b[i+1])
	}
	if len(b)%2 == 1 {
		sum += uint32(b[len(b)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = sum&0xFFFF + sum>>16
	}
	return uint16(sum)
}

func udpSum(src, dst [4]byte, udp []byte) uint16 {
	sum := uint32(src[0])<<8 | uint32(src[1])
	sum += uint32(src[2])<<8 | uint32(src[3])
	sum += uint32(dst[0])<<8 | uint32(dst[1])
	sum += uint32(dst[2])<<8 | uint32(dst[3])
	sum += uint32(ProtocolUDP) + (uint32(udp[4])<<8 | uint32(udp[5]))
	return checksum(sum, udp)
}

// AppendIPv4 appends d to dst as an IPv4 packet with a UDP datagram, and
// computes both checksums. The header has no options, and a TTL of 0 is
// sent as 64. It returns dst unchanged and an error when the datagram is
// longer than 65515 octets or d is nil. It allocates only to grow dst.
// (ETSI TS 102 361-3 clause 5.5)
func AppendIPv4(dst []byte, d *Datagram) ([]byte, error) {
	if d == nil {
		return dst, errNil
	}
	n := len(dst)
	u := UDP{Header: UDPHeader{SourcePort: d.SourcePort, DestinationPort: d.DestinationPort}, Payload: d.Payload}
	dst, err := u.AppendBinary(dst)
	if err != nil {
		return dst[:n], err
	}
	ttl := d.TTL
	if ttl == 0 {
		ttl = 64
	}
	p := IPv4{
		Header: IPv4Header{
			Version: 4, IHL: 5, ID: d.ID, TTL: ttl, Protocol: ProtocolUDP,
			Source: d.Source, Destination: d.Destination,
		},
		Payload: dst[n:],
	}
	m := len(dst)
	dst, err = p.AppendBinary(dst)
	if err != nil {
		return dst[:n], err
	}
	k := copy(dst[n:], dst[m:])
	dst = dst[:n+k]
	ipSum := ^checksum(0, dst[n:n+20])
	binary.BigEndian.PutUint16(dst[n+10:], ipSum)
	udpc := ^udpSum(d.Source, d.Destination, dst[n+20:])
	if udpc == 0 {
		udpc = 0xFFFF
	}
	binary.BigEndian.PutUint16(dst[n+26:], udpc)
	return dst, nil
}

// AppendCompressed appends d to dst with a compressed UDP/IPv4 header. A
// port of 5016 or 5017 is sent as its port identifier, any other port in an
// extended header. It returns dst unchanged and a *dmrgo.FieldRangeError
// when SAID or DAID does not fit, and an error for a nil d. It allocates
// only to grow dst. (ETSI TS 102 361-3 clause 7.2.3)
func AppendCompressed(dst []byte, d *Datagram) ([]byte, error) {
	if d == nil {
		return dst, errNil
	}
	h := pdu.UDPIPv4CompressedHeader{IPv4Identification: d.ID, SAID: d.SAID, DAID: d.DAID}
	var ext [2]uint16
	k := 0
	switch d.SourcePort {
	case PortText:
		h.SPID = enums.SPIDTextMessage
	case PortLocation:
		h.SPID = enums.SPIDLocationProtocol
	default:
		ext[0] = uint16(d.SourcePort)
		k = 1
	}
	switch d.DestinationPort {
	case PortText:
		h.DPID = enums.DPIDTextMessage
	case PortLocation:
		h.DPID = enums.DPIDLocationProtocol
	default:
		ext[k] = uint16(d.DestinationPort)
	}
	h.ExtendedHeader1, h.ExtendedHeader2 = ext[0], ext[1]
	raw, err := h.Encode()
	if err != nil {
		return dst, err
	}
	dst = append(dst, raw[:h.Octets()]...)
	return append(dst, d.Payload...), nil
}

// RadioAddress returns the IPv4 address of the radio with the 24-bit radio
// ID id: network.x.y.z. Radios use DefaultNetwork unless configured
// otherwise. (ETSI TS 102 361-3 clause 5.1.1)
func RadioAddress(network uint8, id uint32) [4]byte {
	var a [4]byte
	binary.BigEndian.PutUint32(a[:], id)
	a[0] = network
	return a
}

// GroupAddress returns the IPv4 multicast address of the group with the
// 24-bit ID group: prefix.x.y.z, where prefix is a class D octet from 224
// to 239, such as 225. (ETSI TS 102 361-3 clause 5.1.1)
func GroupAddress(prefix uint8, group uint32) [4]byte {
	var a [4]byte
	binary.BigEndian.PutUint32(a[:], group)
	a[0] = prefix
	return a
}

// ID returns the 24-bit radio or group ID of the address addr: its last
// three octets. (ETSI TS 102 361-3 clause 5.1.1)
func ID(addr [4]byte) uint32 {
	return uint32(addr[1])<<16 | uint32(addr[2])<<8 | uint32(addr[3])
}
