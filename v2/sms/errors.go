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
	// message in a format that the package decodes.
	ErrNotText = errors.New("sms: packet is not a text message")
	// ErrTextLength is returned by AppendPacket for a FormatUDT text that
	// is empty or does not fit in four UDT blocks: 52 ISO 7-bit
	// characters, 46 ISO 8-bit characters or 23 UTF-16 code units. (ETSI
	// TS 102 361-4 clauses B.3.4, B.3.5 and B.3.8)
	ErrTextLength = errors.New("sms: text length out of range for its format")
	// ErrFormat is returned by AppendPacket for an unknown Format, for
	// FormatDefinedData with DD format binary, for a FormatUDT text with a
	// UDT Format other than ISO 7-bit, ISO 8-bit or UTF-16BE characters,
	// and for a FormatHytera or FormatIP text that Decode would read as
	// another format, and by AppendAck for a text that is not an individual
	// FormatTMS text.
	ErrFormat = errors.New("sms: unknown format")
)
