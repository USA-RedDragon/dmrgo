package lrrp

import "errors"

// Errors returned by the package. Each is a static value, so a decode that
// fails does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned when the data ends inside a field or before the
	// length that the message gives.
	ErrShort = errors.New("lrrp: message too short")
	// ErrLength is returned when the length of the message does not match
	// the data.
	ErrLength = errors.New("lrrp: length does not match the message")
	// ErrToken is returned for a token that the package does not decode,
	// as its length is not known.
	ErrToken = errors.New("lrrp: unknown token")
	// ErrVarint is returned for a number that does not fit its field.
	ErrVarint = errors.New("lrrp: number does not fit its field")
)
