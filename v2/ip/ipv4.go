package ip

// IPv4Header is the fixed 20-octet header of an IPv4 packet (RFC 791
// section 3.1). A packet data message with the SAP IP based packet data
// starts with it (ETSI TS 102 361-3 clause 5.5).
//
//dmr:frame 20
type IPv4Header struct {
	// Version is the IP version, 4.
	Version uint8 `dmr:"bits:0-3"`
	// IHL is the header length in 32-bit words, 5 without options.
	IHL uint8 `dmr:"bits:4-7"`
	// TOS is the type of service octet.
	TOS uint8 `dmr:"bits:8-15"`
	// TotalLength is the length of the packet in octets, header included.
	TotalLength uint16 `dmr:"bits:16-31"`
	// ID identifies the fragments of one packet.
	ID uint16 `dmr:"bits:32-47"`
	// Reserved is the reserved flag, 0.
	Reserved bool `dmr:"bit:48"`
	// DontFragment is the DF flag.
	DontFragment bool `dmr:"bit:49"`
	// MoreFragments is the MF flag.
	MoreFragments bool `dmr:"bit:50"`
	// FragmentOffset is the offset of the fragment in units of 8 octets.
	FragmentOffset uint16 `dmr:"bits:51-63"`
	// TTL is the time to live.
	TTL uint8 `dmr:"bits:64-71"`
	// Protocol is the protocol of the payload.
	Protocol Protocol `dmr:"bits:72-79,enum"`
	// Checksum is the header checksum.
	Checksum uint16 `dmr:"bits:80-95"`
	// Source is the source address.
	Source [4]byte `dmr:"bits:96-127,packed"`
	// Destination is the destination address.
	Destination [4]byte `dmr:"bits:128-159,packed"`
}

// IPv4 is an IPv4 packet: the header, its options and the payload, whose
// length the header gives (RFC 791 section 3.1).
//
//dmr:stream trailing
//dmr:bound Header.TotalLength total
type IPv4 struct {
	// Header is the fixed header.
	Header IPv4Header `dmr:"frame"`
	// Options are the octets of the header after the first 20.
	Options []byte `dmr:"len:Header.IHL*4-20"`
	// Payload is the payload, such as a UDP datagram.
	Payload []byte `dmr:"rest"`
}

// UDPHeader is the 8-octet header of a UDP datagram (RFC 768).
//
//dmr:frame 8
type UDPHeader struct {
	// SourcePort is the source port.
	SourcePort Port `dmr:"bits:0-15,enum"`
	// DestinationPort is the destination port.
	DestinationPort Port `dmr:"bits:16-31,enum"`
	// Length is the length of the datagram in octets, header included.
	Length uint16 `dmr:"bits:32-47"`
	// Checksum is the checksum over the pseudo header and the datagram, or
	// 0 for none.
	Checksum uint16 `dmr:"bits:48-63"`
}

// UDP is a UDP datagram: the header and the payload, whose length the
// header gives (RFC 768).
//
//dmr:stream
//dmr:bound Header.Length total
type UDP struct {
	// Header is the header.
	Header UDPHeader `dmr:"frame"`
	// Payload is the application data.
	Payload []byte `dmr:"rest"`
}
