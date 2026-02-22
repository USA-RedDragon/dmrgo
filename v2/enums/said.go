package enums

import "fmt"

// SAID (Source IP Address Identifier) identifies the source IP address type
// in a UDP/IPv4 compressed header.
// ETSI TS 102 361-3 — Table 7.15
type SAID uint8

const (
	SAIDRadioNetwork         SAID = 0
	SAIDUSBEthernetNetwork   SAID = 1
	SAIDManufacturerSpecific SAID = 12
)

func SAIDToName(said SAID) string {
	switch {
	case said == SAIDRadioNetwork:
		return "Radio Network"
	case said == SAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	case said >= 2 && said <= 11:
		return "Reserved"
	case said >= 12:
		return "Manufacturer Specific"
	}
	return "Unknown"
}

func SAIDFromInt(i int) (SAID, error) {
	if i < 0 || i > 15 {
		return 0, fmt.Errorf("invalid SAID value: %d", i)
	}
	return SAID(i), nil
}
