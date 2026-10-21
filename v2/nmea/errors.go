package nmea

import "errors"

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrSentence is returned for bytes that are not an RMC sentence: no
	// leading $, a sentence type other than RMC, a byte that is not
	// printable ASCII, or a wrong number of fields.
	ErrSentence = errors.New("nmea: not an RMC sentence")
	// ErrChecksum is returned when the *hh checksum is missing or does not
	// match the XOR of the bytes between $ and *.
	ErrChecksum = errors.New("nmea: checksum is missing or does not match")
	// ErrField is returned for a field that is malformed or out of range,
	// such as minute 61 or a latitude over 90 degrees.
	ErrField = errors.New("nmea: field is malformed or out of range")
)
