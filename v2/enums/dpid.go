package enums

import "fmt"

// DPID (UDP Destination Port Identifier) maps to well-known UDP destination ports
// in a UDP/IPv4 compressed header.
// ETSI TS 102 361-3 — Table 7.18
type DPID uint8

const (
	DPIDExtendedHeader       DPID = 0  // Port number in Extended Header
	DPIDTextMessage          DPID = 1  // Port 5016
	DPIDLocationProtocol     DPID = 2  // Port 5017
	DPIDManufacturerSpecific DPID = 95 // 95–127 are manufacturer specific
)

// DPIDToPort returns the well-known UDP port for a DPID, or 0 if extended header or unknown.
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
	case dpid >= 95:
		return manufacturerSpecificName
	}
	return unknownName
}

func DPIDFromInt(i int) (DPID, error) {
	if i < 0 || i > 127 {
		return 0, fmt.Errorf("invalid DPID value: %d", i)
	}
	return DPID(i), nil
}
