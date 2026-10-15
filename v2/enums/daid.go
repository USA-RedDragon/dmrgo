package enums

const (
	unknownName              = "Unknown"
	reservedName             = "Reserved"
	manufacturerSpecificName = "Manufacturer Specific"
)

// DAID is the 4-bit Destination IP Address Identifier of a UDP/IPv4
// compressed header. Values 3 to 11 are reserved and 12 to 15 are
// manufacturer specific (ETSI TS 102 361-3 Table 7.16).
type DAID uint8

// The DAID values.
const (
	DAIDRadioNetwork         DAID = 0  // radio network
	DAIDUSBEthernetNetwork   DAID = 1  // USB or Ethernet network
	DAIDGroupNetwork         DAID = 2  // group network
	DAIDManufacturerSpecific DAID = 12 // first manufacturer specific value
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
