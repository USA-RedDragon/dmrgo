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
