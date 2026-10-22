package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// SAID is the 4-bit Source IP Address Identifier of a UDP/IPv4 compressed
// header. Values 2 to 11 are reserved and 12 to 15 are manufacturer specific
// (ETSI TS 102 361-3 Table 7.15).
type SAID uint8

// The SAID values.
const (
	SAIDRadioNetwork       SAID = 0 // radio network
	SAIDUSBEthernetNetwork SAID = 1 // USB or Ethernet network
)

// String returns the name of the SAID. It allocates only for a value
// without a constant.
func (said SAID) String() string {
	switch said {
	case SAIDRadioNetwork:
		return "Radio Network"
	case SAIDUSBEthernetNetwork:
		return "USB/Ethernet Network"
	}
	return "SAID(" + text.Int(int64(said)) + ")"
}

// IsManufacturerSpecific reports whether said is one of the manufacturer
// specific values 12 to 15.
func (said SAID) IsManufacturerSpecific() bool {
	return said >= 12 && said <= 15
}
