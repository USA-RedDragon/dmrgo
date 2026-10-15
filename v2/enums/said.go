package enums

// SAID (Source IP Address Identifier) identifies the source IP address type
// in a UDP/IPv4 compressed header.
// ETSI TS 102 361-3 — Table 7.15
type SAID uint8

const (
	SAIDRadioNetwork         SAID = 0
	SAIDUSBEthernetNetwork   SAID = 1
	SAIDManufacturerSpecific SAID = 12
)

func (said SAID) String() string {
	switch {
	case said == SAIDRadioNetwork:
		return "Radio Network"
	case said == SAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	case said >= 2 && said <= 11:
		return reservedName
	case said >= 12:
		return manufacturerSpecificName
	}
	return unknownName
}

// Valid reports whether said fits the 4-bit SAID field.
func (said SAID) Valid() bool {
	return said <= 15
}
