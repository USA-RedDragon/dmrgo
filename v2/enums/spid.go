package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// SPID is the 7-bit UDP Source Port Identifier of a UDP/IPv4 compressed
// header. Values 3 to 94 are reserved and 95 to 127 are manufacturer
// specific (ETSI TS 102 361-3 Table 7.17).
type SPID uint8

// The SPID values.
const (
	SPIDExtendedHeader   SPID = 0 // port number in the extended header
	SPIDTextMessage      SPID = 1 // Port 5016
	SPIDLocationProtocol SPID = 2 // Port 5017
)

// String returns the name of the SPID. It allocates only for a value
// without a constant.
func (spid SPID) String() string {
	switch spid {
	case SPIDExtendedHeader:
		return "Extended Header"
	case SPIDTextMessage:
		return "Text Message (5016)"
	case SPIDLocationProtocol:
		return "Location Protocol (5017)"
	}
	return "SPID(" + text.Int(int64(spid)) + ")"
}

// IsManufacturerSpecific reports whether spid is one of the manufacturer
// specific values 95 to 127.
func (spid SPID) IsManufacturerSpecific() bool {
	return spid >= 95 && spid <= 127
}
