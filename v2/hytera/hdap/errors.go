package hdap

import "errors"

// Errors returned by the package. Each is a static value, so a decode that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a field.
	ErrShort = errors.New("hdap: message too short")
	// ErrLength is returned when the length of a field does not match the
	// data.
	ErrLength = errors.New("hdap: length does not match the message")
	// ErrSignature is returned when a fixed octet, such as the end octet
	// 0x03, does not hold its value.
	ErrSignature = errors.New("hdap: fixed octet does not match")
	// ErrText is returned when a number sent as text holds a character
	// that is not a digit.
	ErrText = errors.New("hdap: number holds a character that is not a digit")
)
