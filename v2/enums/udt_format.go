package enums

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// UDTFormat is the 4-bit UDT Format of a UDT header: the format of the
// appended data. Values that have no constant are reserved (ETSI TS 102
// 361-4 V1.12.1 clause 7.2.27, Table 7.88).
type UDTFormat uint8

// The UDTFormat values.
const (
	UDTBinary        UDTFormat = 0b0000 // binary
	UDTAddress       UDTFormat = 0b0001 // MS or talkgroup addresses
	UDTBCD           UDTFormat = 0b0010 // 4-bit BCD digits
	UDTISO7          UDTFormat = 0b0011 // ISO/IEC 646 7-bit characters
	UDTISO8          UDTFormat = 0b0100 // ISO/IEC 8859 8-bit characters
	UDTNMEA          UDTFormat = 0b0101 // NMEA (IEC 61162-1) location
	UDTIP            UDTFormat = 0b0110 // IP address
	UDTUTF16BE       UDTFormat = 0b0111 // UTF-16BE characters
	UDTManufacturer1 UDTFormat = 0b1000 // manufacturer specific
	UDTManufacturer2 UDTFormat = 0b1001 // manufacturer specific
	UDTMixed         UDTFormat = 0b1010 // one address and UTF-16BE characters
	UDTLIP           UDTFormat = 0b1011 // Location Information Protocol (LIP)
)

// String returns the name of the UDT Format. It allocates only for a
// reserved value.
func (f UDTFormat) String() string {
	switch f {
	case UDTBinary:
		return "Binary"
	case UDTAddress:
		return "Address"
	case UDTBCD:
		return "BCD"
	case UDTISO7:
		return "ISO 7 bit"
	case UDTISO8:
		return "ISO 8 bit"
	case UDTNMEA:
		return "NMEA"
	case UDTIP:
		return "IP"
	case UDTUTF16BE:
		return "UTF-16BE"
	case UDTManufacturer1, UDTManufacturer2:
		return "Manufacturer Specific"
	case UDTMixed:
		return "Mixed"
	case UDTLIP:
		return "LIP"
	}
	return "Reserved UDT Format(" + text.Int(int64(f)) + ")"
}
