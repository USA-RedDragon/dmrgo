package pdu

// ETSI TS 102 361-3 - Table 7.14: UDP/IPv4 Compressed Header
// This structure lives in the first data continuation block when
// the Data Header SAP = UDPIPHeaderCompression (0b0011).
//
// Extended Header rules:
//   - If SPID == 0: ExtendedHeader1 = Source Port Number
//   - If SPID != 0 && DPID == 0: ExtendedHeader1 = Destination Port Number
//   - If SPID == 0 && DPID == 0: ExtendedHeader2 = Destination Port Number
//   - Otherwise: extended headers carry application data
type UDPIPv4CompressedHeader struct {
	IPv4Identification      uint16 `dmr:"bits:0-15"`
	SAID                    uint8  `dmr:"bits:16-19"`
	DAID                    uint8  `dmr:"bits:20-23"`
	HeaderCompressionOpcode uint8  `dmr:"bits:24+32"`
	SPID                    uint8  `dmr:"bits:25-31"`
	DPID                    uint8  `dmr:"bits:33-39"`
	ExtendedHeader1         uint16 `dmr:"bits:40-55"`
	ExtendedHeader2         uint16 `dmr:"bits:56-71"`
}
