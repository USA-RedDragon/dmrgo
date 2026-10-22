package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// DPID is the 7-bit UDP Destination Port Identifier of a UDP/IPv4
// compressed header. Values 3 to 94 are reserved and 95 to 127 are
// manufacturer specific (ETSI TS 102 361-3 Table 7.18).
type DPID uint8

// The DPID values.
const (
	DPIDExtendedHeader   DPID = 0 // port number in the extended header
	DPIDTextMessage      DPID = 1 // Port 5016
	DPIDLocationProtocol DPID = 2 // Port 5017
)

// DPIDToPort returns the UDP port of dpid. It returns 0 for
// DPIDExtendedHeader and for a DPID that has no fixed port.
func DPIDToPort(dpid DPID) uint16 {
	switch dpid {
	case DPIDTextMessage:
		return 5016
	case DPIDLocationProtocol:
		return 5017
	case DPIDExtendedHeader:
		return 0
	default:
		return 0
	}
}

// String returns the name of the DPID. It allocates only for a value
// without a constant.
func (dpid DPID) String() string {
	switch dpid {
	case DPIDExtendedHeader:
		return "Extended Header"
	case DPIDTextMessage:
		return "Text Message (5016)"
	case DPIDLocationProtocol:
		return "Location Protocol (5017)"
	}
	return "DPID(" + text.Int(int64(dpid)) + ")"
}

// IsManufacturerSpecific reports whether dpid is one of the manufacturer
// specific values 95 to 127.
func (dpid DPID) IsManufacturerSpecific() bool {
	return dpid >= 95 && dpid <= 127
}
