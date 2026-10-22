package ars

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Type is the 4-bit PDU type of a message.
type Type uint8

// The Type values.
const (
	TypeDeviceRegistration   Type = 0x0 // a radio registers with the server
	TypeDeviceDeregistration Type = 0x1 // a radio leaves
	TypeQuery                Type = 0x4 // the server asks a radio to register
	TypeUserRegistration     Type = 0x5 // a user logs on at a radio
	TypeUserDeregistration   Type = 0x6 // a user logs off
	TypeUserAck              Type = 0x7 // the answer to a user registration or deregistration
	TypeAck                  Type = 0xF // the answer to a device registration or a query
)

// String returns the name of t. It allocates only for a value without a
// name.
func (t Type) String() string {
	switch t {
	case TypeDeviceRegistration:
		return "Device Registration"
	case TypeDeviceDeregistration:
		return "Device Deregistration"
	case TypeQuery:
		return "Query"
	case TypeUserRegistration:
		return "User Registration"
	case TypeUserDeregistration:
		return "User Deregistration"
	case TypeUserAck:
		return "User Ack"
	case TypeAck:
		return "Ack"
	}
	return "Type(" + text.Int(int64(t)) + ")"
}

// Event is the 2-bit event of a device registration.
type Event uint8

// The Event values.
const (
	EventNone    Event = 0 // no event, as in a user registration
	EventInitial Event = 1 // the radio registers after power on or a channel change
	EventRefresh Event = 2 // the radio registers again before its registration ends
)

// String returns the name of e. It allocates only for a value without a
// name.
func (e Event) String() string {
	switch e {
	case EventNone:
		return "None"
	case EventInitial:
		return "Initial"
	case EventRefresh:
		return "Refresh"
	}
	return "Event(" + text.Int(int64(e)) + ")"
}

// Encoding is the 5-bit coding of the identifiers of a registration.
type Encoding uint8

// The Encoding values.
const (
	EncodingASCII Encoding = 0 // ASCII characters
)

// String returns the name of e. It allocates only for a value without a
// name.
func (e Encoding) String() string {
	if e == EncodingASCII {
		return "ASCII"
	}
	return "Encoding(" + text.Int(int64(e)) + ")"
}

// FailureReason is the reason in Response.Value of an answer whose
// Header.AckRequired marks a failure. The field has 7 bits, and the meaning
// of a value depends on Header.Type (MOTOTRBO ARS specification 02.00). In
// the answer to a device registration, TypeAck, only 0 is defined (clause
// 3.4.3). In the answer to a user registration, TypeUserAck, 1 and 2 are
// defined and each other value, 0 included, is a transmission failure
// (clause 3.4.9).
type FailureReason uint8

// The FailureReason values. ok-dmrlib sends 0xFF in the answer to a device
// registration, which sets the Extension bit; its 7-bit value 127 is not
// defined there.
const (
	FailureDeviceNotAuthorized   FailureReason = 0   // in TypeAck: the radio may not register
	FailureUserIDNotValid        FailureReason = 1   // in TypeUserAck: the user ID is not known
	FailureUserValidationTimeout FailureReason = 2   // in TypeUserAck: the user validation timed out
	FailureTransmission          FailureReason = 127 // in TypeUserAck: a transmission failure, as each value from 3 to 127
)

// String returns the name of r: "Device Not Authorized" for 0, its meaning
// in a TypeAck, though in a TypeUserAck 0 is a transmission failure, and
// "Transmission Failure" for each value from 3 to 127. It allocates only for
// a value above 127.
func (r FailureReason) String() string {
	switch {
	case r == FailureDeviceNotAuthorized:
		return "Device Not Authorized"
	case r == FailureUserIDNotValid:
		return "User ID Not Valid"
	case r == FailureUserValidationTimeout:
		return "User Validation Timeout"
	case r <= FailureTransmission:
		return "Transmission Failure"
	}
	return "FailureReason(" + text.Int(int64(r)) + ")"
}

// Header is the first header octet of a message.
//
//dmr:frame 1
type Header struct {
	// Extension reports that a second header octet follows.
	Extension bool `dmr:"bit:0"`
	// AckRequired asks for an answer in a request. In an answer it marks a
	// failure.
	AckRequired bool `dmr:"bit:1"`
	// Priority is the priority bit.
	Priority bool `dmr:"bit:2"`
	// Control is the control bit.
	Control bool `dmr:"bit:3"`
	// Type is the PDU type.
	Type Type `dmr:"bits:4-7,enum"`
}

// Registration is the second header octet of a registration.
//
//dmr:frame 1
type Registration struct {
	// Extension reports that a further header octet follows.
	Extension bool `dmr:"bit:0"`
	// Event is the event of a device registration.
	Event Event `dmr:"bits:1-2,enum"`
	// Encoding is the coding of the identifiers.
	Encoding Encoding `dmr:"bits:3-7,enum"`
}

// Response is the second header octet of an answer.
//
//dmr:frame 1
type Response struct {
	// Extension reports that a further header octet follows.
	Extension bool `dmr:"bit:0"`
	// Value is the refresh time of a successful device registration, in
	// units of 30 minutes, 0 for none. When Header.AckRequired marks a
	// failure it is the FailureReason.
	Value uint8 `dmr:"bits:1-7"`
}

// Message is a MOTOTRBO Automatic Registration Service message, the
// payload of a UDP datagram to port 4005.
//
//dmr:stream
type Message struct {
	// Length is the number of octets after it. AppendBinary sets it.
	Length uint16 `dmr:"u16,length"`
	// Header is the first header octet.
	Header Header `dmr:"frame"`
	// Registration is the second header octet of a device or user
	// registration with Header.Extension set.
	Registration Registration `dmr:"frame,if:Header.Extension&Header.Type==TypeDeviceRegistration|TypeUserRegistration"`
	// Device is the identifier of the radio in a registration: its radio
	// ID in decimal ASCII digits.
	Device []byte `dmr:"lv8,if:Header.Type==TypeDeviceRegistration|TypeUserRegistration"`
	// User is the user identifier of a registration.
	User []byte `dmr:"lv8,if:Header.Type==TypeDeviceRegistration|TypeUserRegistration"`
	// Password is the password of a registration.
	Password []byte `dmr:"lv8,if:Header.Type==TypeDeviceRegistration|TypeUserRegistration"`
	// Response is the second header octet of an answer with
	// Header.Extension set.
	Response Response `dmr:"frame,if:Header.Extension&Header.Type==TypeAck|TypeUserAck"`
	// Extra holds the octets after the fields that the package knows.
	Extra []byte `dmr:"rest"`
}
