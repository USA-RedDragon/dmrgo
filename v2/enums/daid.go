package enums

import "fmt"

// DAID (Destination IP Address Identifier) identifies the destination IP address type
// in a UDP/IPv4 compressed header.
// ETSI TS 102 361-3 — Table 7.16
type DAID uint8

const (
	DAIDRadioNetwork         DAID = 0
	DAIDUSBEthernetNetwork   DAID = 1
	DAIDGroupNetwork         DAID = 2
	DAIDManufacturerSpecific DAID = 12
)

func DAIDToName(daid DAID) string {
	switch {
	case daid == DAIDRadioNetwork:
		return "Radio Network"
	case daid == DAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	case daid == DAIDGroupNetwork:
		return "Group Network"
	case daid >= 3 && daid <= 11:
		return "Reserved"
	case daid >= 12:
		return "Manufacturer Specific"
	}
	return "Unknown"
}

func DAIDFromInt(i int) (DAID, error) {
	if i < 0 || i > 15 {
		return 0, fmt.Errorf("invalid DAID value: %d", i)
	}
	return DAID(i), nil
}
