package enums

const (
	unknownName              = "Unknown"
	reservedName             = "Reserved"
	manufacturerSpecificName = "Manufacturer Specific"
)

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

func (daid DAID) String() string {
	switch {
	case daid == DAIDRadioNetwork:
		return "Radio Network"
	case daid == DAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	case daid == DAIDGroupNetwork:
		return "Group Network"
	case daid >= 3 && daid <= 11:
		return reservedName
	case daid >= 12:
		return manufacturerSpecificName
	}
	return unknownName
}

// Valid reports whether daid fits the 4-bit DAID field.
func (daid DAID) Valid() bool {
	return daid <= 15
}
