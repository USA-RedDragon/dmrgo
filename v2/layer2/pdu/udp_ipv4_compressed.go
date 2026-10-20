package pdu

import "github.com/USA-RedDragon/dmrgo/v2/enums"

// UDPIPv4CompressedHeader is the compressed UDP/IPv4 header. It starts the
// first data block when the data header SAP is UDP/IP header compression
// (0b0011). (ETSI TS 102 361-3 clause 7.2.3, Table 7.14)
type UDPIPv4CompressedHeader struct {
	IPv4Identification      uint16     `dmr:"bits:0-15"`  // Identification field of the IPv4 header
	SAID                    enums.SAID `dmr:"bits:16-19"` // source IP address identifier
	DAID                    enums.DAID `dmr:"bits:20-23"` // destination IP address identifier
	HeaderCompressionOpcode uint8      `dmr:"bits:24+32"` // 2-bit header compression opcode; bit 24 is the MSB
	SPID                    enums.SPID `dmr:"bits:25-31"` // source port identifier
	DPID                    enums.DPID `dmr:"bits:33-39"` // destination port identifier
	ExtendedHeader1         uint16     `dmr:"bits:40-55"` // source port if SPID is 0, else destination port if DPID is 0, else payload
	ExtendedHeader2         uint16     `dmr:"bits:56-71"` // destination port if SPID and DPID are 0, else payload
}

// Octets returns the length of the compressed header: 5 octets, plus 2 for
// each port that an Extended Header carries because its SPID or DPID is 0.
// The application data starts after it. (ETSI TS 102 361-3 clause 5.6,
// Tables 7.20 and 7.21)
func (s *UDPIPv4CompressedHeader) Octets() int {
	n := 5
	if s.SPID == enums.SPIDExtendedHeader {
		n += 2
	}
	if s.DPID == enums.DPIDExtendedHeader {
		n += 2
	}
	return n
}

// Ports returns the UDP source and destination ports: the port of the SPID
// or DPID, or the port in an Extended Header when the SPID or DPID is 0. A
// manufacturer specific or reserved SPID or DPID gives port 0, as the radio
// configures its port. (ETSI TS 102 361-3 clauses 7.2.1.1 and 7.2.1.2)
func (s *UDPIPv4CompressedHeader) Ports() (source, destination uint16) {
	source = enums.SPIDToPort(s.SPID)
	destination = enums.DPIDToPort(s.DPID)
	switch {
	case s.SPID == enums.SPIDExtendedHeader && s.DPID == enums.DPIDExtendedHeader:
		source, destination = s.ExtendedHeader1, s.ExtendedHeader2
	case s.SPID == enums.SPIDExtendedHeader:
		source = s.ExtendedHeader1
	case s.DPID == enums.DPIDExtendedHeader:
		destination = s.ExtendedHeader1
	}
	return source, destination
}
