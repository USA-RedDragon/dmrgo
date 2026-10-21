package ars

import "errors"

// Errors returned by the package. Each is a static value, so a decode that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a field or before the
	// length that the message gives.
	ErrShort = errors.New("ars: message too short")
	// ErrLength is returned when the length of the message does not match
	// the data.
	ErrLength = errors.New("ars: length does not match the message")
)
