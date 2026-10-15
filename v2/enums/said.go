package enums

// SAID is the 4-bit Source IP Address Identifier of a UDP/IPv4 compressed
// header. Values 2 to 11 are reserved and 12 to 15 are manufacturer specific
// (ETSI TS 102 361-3 Table 7.15).
type SAID uint8

const (
	SAIDRadioNetwork         SAID = 0  // radio network
	SAIDUSBEthernetNetwork   SAID = 1  // USB or Ethernet network
	SAIDManufacturerSpecific SAID = 12 // first manufacturer specific value
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
