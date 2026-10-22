package sms

import "errors"

// Errors returned by the package. Each is a static value, so a call that
// fails does not allocate. Match them with errors.Is. Decode also returns
// the errors of the ip, tms and shortdata packages.
var (
	// ErrShort is returned when a text payload ends inside a field.
	ErrShort = errors.New("sms: payload too short")
	// ErrLength is returned when a text payload has octets that its format
	// does not hold.
	ErrLength = errors.New("sms: length does not match the payload")
	// ErrSignature is returned when the prefix of an AnyTone text is not
	// 00 0d 00 0a.
	ErrSignature = errors.New("sms: AnyTone text without its prefix")
	// ErrNotText is returned for a data packet that does not carry a text
	// message in a format that the package knows.
	ErrNotText = errors.New("sms: packet is not a text message")
	// ErrFormat is returned by AppendPacket for an unknown Format, and for
	// FormatDefinedData with DD format binary.
	ErrFormat = errors.New("sms: unknown format")
)
