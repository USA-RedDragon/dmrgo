//go:build ignore

package dmrd

//dmr:view Config 302
//dmr:magic 0-31="RPTC"
type configLayout struct {
	// ID is the repeater ID.
	ID uint32 `dmr:"bits:32-63"`
	// Callsign is the callsign of the repeater.
	Callsign [8]byte `dmr:"bits:64-127,packed,text"`
	// RXFrequency is the receive frequency in Hz, 9 digits.
	RXFrequency uint32 `dmr:"bits:128-199,dec"`
	// TXFrequency is the transmit frequency in Hz, 9 digits.
	TXFrequency uint32 `dmr:"bits:200-271,dec"`
	// Power is the transmit power in dBm, 2 digits.
	Power uint8 `dmr:"bits:272-287,dec"`
	// ColorCode is the color code, 2 digits.
	ColorCode enums.ColorCode `dmr:"bits:288-303,dec"`
	// Latitude is the latitude as text, 8 characters, North positive.
	Latitude [8]byte `dmr:"bits:304-367,packed,text"`
	// Longitude is the longitude as text, 9 characters, East positive.
	Longitude [9]byte `dmr:"bits:368-439,packed,text"`
	// Height is the antenna height above ground in meters, 3 digits.
	Height uint16 `dmr:"bits:440-463,dec"`
	// Location is the location of the repeater as free text.
	Location [20]byte `dmr:"bits:464-623,packed,text"`
	// Description is the description of the repeater as free text.
	Description [19]byte `dmr:"bits:624-775,packed,text"`
	// Slots is the slots digit: 1 or 2 for one slot, 3 for both in duplex,
	// 4 for simplex (DMO).
	Slots uint8 `dmr:"bits:776-783,dec"`
	// URL is the web page of the repeater.
	URL [124]byte `dmr:"bits:784-1775,packed,text"`
	// SoftwareID is the software and its version.
	SoftwareID [40]byte `dmr:"bits:1776-2095,packed,text"`
	// PackageID is the package and its version.
	PackageID [40]byte `dmr:"bits:2096-2415,packed,text"`
}

//dmr:view HBRPConfig 306
//dmr:magic 0-31="RPTC"
type hbrpConfigLayout struct {
	// Callsign is the callsign of the repeater.
	Callsign [8]byte `dmr:"bits:32-95,packed,text"`
	// ID is the repeater ID, 8 hex digits.
	ID uint32 `dmr:"bits:96-159,hex"`
	// RXFrequency is the receive frequency in Hz, 9 digits.
	RXFrequency uint32 `dmr:"bits:160-231,dec"`
	// TXFrequency is the transmit frequency in Hz, 9 digits.
	TXFrequency uint32 `dmr:"bits:232-303,dec"`
	// Power is the transmit power in dBm, 2 digits.
	Power uint8 `dmr:"bits:304-319,dec"`
	// ColorCode is the color code, 2 digits.
	ColorCode enums.ColorCode `dmr:"bits:320-335,dec"`
	// Latitude is the latitude as text, 8 characters, North positive.
	Latitude [8]byte `dmr:"bits:336-399,packed,text"`
	// Longitude is the longitude as text, 9 characters, East positive.
	Longitude [9]byte `dmr:"bits:400-471,packed,text"`
	// Height is the antenna height above ground in meters, 3 digits.
	Height uint16 `dmr:"bits:472-495,dec"`
	// Location is the location of the repeater as free text.
	Location [20]byte `dmr:"bits:496-655,packed,text"`
	// Description is the description of the repeater as free text.
	Description [20]byte `dmr:"bits:656-815,packed,text"`
	// URL is the web page of the repeater.
	URL [124]byte `dmr:"bits:816-1807,packed,text"`
	// SoftwareID is the software and its version.
	SoftwareID [40]byte `dmr:"bits:1808-2127,packed,text"`
	// PackageID is the package and its version.
	PackageID [40]byte `dmr:"bits:2128-2447,packed,text"`
}
