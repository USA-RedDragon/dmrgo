package enums

// SPID is the 7-bit UDP Source Port Identifier of a UDP/IPv4 compressed
// header. Values 3 to 94 are reserved and 95 to 127 are manufacturer
// specific (ETSI TS 102 361-3 Table 7.17).
type SPID uint8

// The SPID values.
const (
	SPIDExtendedHeader       SPID = 0  // port number in the extended header
	SPIDTextMessage          SPID = 1  // Port 5016
	SPIDLocationProtocol     SPID = 2  // Port 5017
	SPIDManufacturerSpecific SPID = 95 // first manufacturer specific value
)

// SPIDToPort returns the UDP port of spid. It returns 0 for
// SPIDExtendedHeader and for a SPID that has no fixed port.
func SPIDToPort(spid SPID) uint16 {
	switch spid {
	case SPIDTextMessage:
		return 5016
	case SPIDLocationProtocol:
		return 5017
	case SPIDExtendedHeader, SPIDManufacturerSpecific:
		return 0
	default:
		return 0
	}
}

func (spid SPID) String() string {
	switch {
	case spid == SPIDExtendedHeader:
		return "Extended Header"
	case spid == SPIDTextMessage:
		return "Text Message (5016)"
	case spid == SPIDLocationProtocol:
		return "Location Protocol (5017)"
	case spid >= 3 && spid <= 94:
		return reservedName
	case spid >= 95 && spid <= 127:
		return manufacturerSpecificName
	}
	return unknownName
}
