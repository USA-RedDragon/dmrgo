package enums

// DPID is the 7-bit UDP Destination Port Identifier of a UDP/IPv4
// compressed header. Values 3 to 94 are reserved and 95 to 127 are
// manufacturer specific (ETSI TS 102 361-3 Table 7.18).
type DPID uint8

// The DPID values.
const (
	DPIDExtendedHeader       DPID = 0  // port number in the extended header
	DPIDTextMessage          DPID = 1  // Port 5016
	DPIDLocationProtocol     DPID = 2  // Port 5017
	DPIDManufacturerSpecific DPID = 95 // first manufacturer specific value
)

// DPIDToPort returns the UDP port of dpid. It returns 0 for
// DPIDExtendedHeader and for a DPID that has no fixed port.
func DPIDToPort(dpid DPID) uint16 {
	switch dpid {
	case DPIDTextMessage:
		return 5016
	case DPIDLocationProtocol:
		return 5017
	case DPIDExtendedHeader, DPIDManufacturerSpecific:
		return 0
	default:
		return 0
	}
}

func (dpid DPID) String() string {
	switch {
	case dpid == DPIDExtendedHeader:
		return "Extended Header"
	case dpid == DPIDTextMessage:
		return "Text Message (5016)"
	case dpid == DPIDLocationProtocol:
		return "Location Protocol (5017)"
	case dpid >= 3 && dpid <= 94:
		return reservedName
	case dpid >= 95 && dpid <= 127:
		return manufacturerSpecificName
	}
	return unknownName
}
