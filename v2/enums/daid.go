package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// DAID is the 4-bit Destination IP Address Identifier of a UDP/IPv4
// compressed header. Values 3 to 11 are reserved and 12 to 15 are
// manufacturer specific (ETSI TS 102 361-3 Table 7.16).
type DAID uint8

// The DAID values.
const (
	DAIDRadioNetwork       DAID = 0 // radio network
	DAIDUSBEthernetNetwork DAID = 1 // USB or Ethernet network
	DAIDGroupNetwork       DAID = 2 // group network
)

// String returns the name of the DAID. It allocates only for a value
// without a constant.
func (daid DAID) String() string {
	switch daid {
	case DAIDRadioNetwork:
		return "Radio Network"
	case DAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	case DAIDGroupNetwork:
		return "Group Network"
	}
	return "DAID(" + text.Int(int64(daid)) + ")"
}

// IsManufacturerSpecific reports whether daid is one of the manufacturer
// specific values 12 to 15.
func (daid DAID) IsManufacturerSpecific() bool {
	return daid >= 12 && daid <= 15
}
