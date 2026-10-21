package shortdata

import "errors"

// Errors returned by the package. Each is a static value, so a call that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a field.
	ErrShort = errors.New("shortdata: data too short")
	// ErrLength is returned when octets are left after the message.
	ErrLength = errors.New("shortdata: length does not match the data")
	// ErrFormat is returned for a reserved DD format.
	ErrFormat = errors.New("shortdata: DD format is not a supported text coding")
	// ErrCharacter is returned by AppendText for a character that the DD
	// format cannot hold, such as U+20AC in ISO/IEC 8859-1, a letter in BCD
	// or U+00E9 in 7-bit text.
	ErrCharacter = errors.New("shortdata: character does not fit the DD format")
)
