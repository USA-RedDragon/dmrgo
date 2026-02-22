package enums

import "fmt"

// SPID (UDP Source Port Identifier) maps to well-known UDP source ports
// in a UDP/IPv4 compressed header.
// ETSI TS 102 361-3 — Table 7.17
type SPID uint8

const (
	SPIDExtendedHeader       SPID = 0  // Port number in Extended Header
	SPIDTextMessage          SPID = 1  // Port 5016
	SPIDLocationProtocol     SPID = 2  // Port 5017
	SPIDManufacturerSpecific SPID = 95 // 95–127 are manufacturer specific
)

// SPIDToPort returns the well-known UDP port for a SPID, or 0 if extended header or unknown.
func SPIDToPort(spid SPID) uint16 {
	switch spid {
	case SPIDTextMessage:
		return 5016
	case SPIDLocationProtocol:
		return 5017
	default:
		return 0
	}
}

func SPIDToName(spid SPID) string {
	switch {
	case spid == SPIDExtendedHeader:
		return "Extended Header"
	case spid == SPIDTextMessage:
		return "Text Message (5016)"
	case spid == SPIDLocationProtocol:
		return "Location Protocol (5017)"
	case spid >= 3 && spid <= 94:
		return "Reserved"
	case spid >= 95:
		return "Manufacturer Specific"
	}
	return "Unknown"
}

func SPIDFromInt(i int) (SPID, error) {
	if i < 0 || i > 127 {
		return 0, fmt.Errorf("invalid SPID value: %d", i)
	}
	return SPID(i), nil
}
