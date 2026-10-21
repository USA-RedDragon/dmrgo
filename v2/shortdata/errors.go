package shortdata

import "errors"

// Errors returned by the package. Each is a static value, so a call that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a field.
	ErrShort = errors.New("shortdata: data too short")
	// ErrLength is returned when octets are left after the message.
	ErrLength = errors.New("shortdata: length does not match the data")
	// ErrFormat is returned for a DD format that is not a text coding that
	// the package supports: binary, BCD, 7-bit, ISO/IEC 8859 other than
	// part 1, UTF-32 and the reserved values.
	ErrFormat = errors.New("shortdata: DD format is not a supported text coding")
	// ErrCharacter is returned by AppendText for a character that the DD
	// format cannot hold, such as U+20AC in ISO/IEC 8859-1.
	ErrCharacter = errors.New("shortdata: character does not fit the DD format")
)
