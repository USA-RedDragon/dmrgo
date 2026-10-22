package hdap

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Opcode is the 16-bit opcode of a message. Its values depend on the
// service. In the text message protocol (TMP) the high octet is the
// TMPFlags and the low octet is a TMPType: TMP splits it and TMPOpcode
// builds it.
type Opcode uint16

// The opcodes of the radio registration service (RRS) and the location
// protocol (LP).
const (
	RRSDeregistration     Opcode = 0x0001 // the radio goes offline
	RRSOnlineCheck        Opcode = 0x0002 // the server asks if the radio is online
	RRSRegistration       Opcode = 0x0003 // the radio registers
	RRSRegistrationAnswer Opcode = 0x0080 // the answer to a registration
	RRSOnlineCheckAnswer  Opcode = 0x0082 // the answer to an online check
	LPStandardRequest     Opcode = 0xA001 // ask for one position
	LPStandardAnswer      Opcode = 0xA002 // the answer to a standard request
	LPEmergencyReport     Opcode = 0xB003 // a position sent in an emergency
	LPConditionReport     Opcode = 0xD003 // a position on a condition, such as a period
)

// String returns the name of o. It allocates only for a value without a
// name, such as a TMP opcode.
func (o Opcode) String() string {
	switch o {
	case RRSDeregistration:
		return "RRSDeregistration"
	case RRSOnlineCheck:
		return "RRSOnlineCheck"
	case RRSRegistration:
		return "RRSRegistration"
	case RRSRegistrationAnswer:
		return "RRSRegistrationAnswer"
	case RRSOnlineCheckAnswer:
		return "RRSOnlineCheckAnswer"
	case LPStandardRequest:
		return "LPStandardRequest"
	case LPStandardAnswer:
		return "LPStandardAnswer"
	case LPEmergencyReport:
		return "LPEmergencyReport"
	case LPConditionReport:
		return "LPConditionReport"
	}
	return "Opcode(" + text.Int(int64(o)) + ")"
}

// TMP returns the TMPFlags and the TMPType of a text message protocol
// (TMP) opcode: its high and low octets.
func (o Opcode) TMP() (TMPFlags, TMPType) {
	b := [1]byte{byte(o >> 8)}
	f, _ := DecodeTMPFlags(b[:])
	return f, TMPType(o & 0xFF)
}

// TMPOpcode returns the text message protocol (TMP) opcode of the flags f and
// the service type t, the inverse of Opcode.TMP. It returns a
// *dmrgo.FieldRangeError when f.Reserved does not fit in 6 bits.
func TMPOpcode(f TMPFlags, t TMPType) (Opcode, error) {
	if f.Reserved>>6 != 0 {
		return 0, errTMPFlagsReservedRange
	}
	o := Opcode(f.Reserved)<<8 | Opcode(t)
	if f.NoAck {
		o |= 0x8000
	}
	if f.Option {
		o |= 0x4000
	}
	return o, nil
}

// TMPType is the service type of the text message protocol (TMP), the low
// octet of its Opcode.
type TMPType uint8

// The TMPType values.
const (
	TMPPrivateMessage       TMPType = 0xA1 // a text to a radio
	TMPPrivateMessageAnswer TMPType = 0xA2 // the answer to a private text
	TMPGroupMessage         TMPType = 0xB1 // a text to a talkgroup
	TMPGroupMessageAnswer   TMPType = 0xB2 // the answer to a group text
)

// String returns the name of t. It allocates only for a value without a
// name.
func (t TMPType) String() string {
	switch t {
	case TMPPrivateMessage:
		return "TMPPrivateMessage"
	case TMPPrivateMessageAnswer:
		return "TMPPrivateMessageAnswer"
	case TMPGroupMessage:
		return "TMPGroupMessage"
	case TMPGroupMessageAnswer:
		return "TMPGroupMessageAnswer"
	}
	return "TMPType(" + text.Int(int64(t)) + ")"
}

// RRSRadio is the payload of an RRS registration, deregistration and online
// check: the radio IP of the radio.
//
//dmr:stream
type RRSRadio struct {
	// Radio is the radio IP: the subnet, then the 24-bit radio ID.
	Radio [4]byte `dmr:"bytes:4"`
}

// RRSAnswer is the payload of the answer to an RRS registration.
//
//dmr:stream
type RRSAnswer struct {
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
	// Result is 0 for success, 1 for another failure and 2 for a wrong
	// password.
	Result uint8 `dmr:"u8"`
	// ValidTime is the time in seconds after which the radio registers
	// again.
	ValidTime uint32 `dmr:"u32"`
}

// RRSCheckAnswer is the payload of the answer to an RRS online check.
//
//dmr:stream
type RRSCheckAnswer struct {
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
	// Result is 0 when the radio is online and 1 when it is offline.
	Result uint8 `dmr:"u8"`
}

// TMPFlags is the high octet of the opcode of a text message protocol
// (TMP) message.
//
//dmr:frame 1
type TMPFlags struct {
	// NoAck reports that the sender wants no answer.
	NoAck bool `dmr:"bit:0"`
	// Option reports an option field, which the package does not decode.
	Option bool `dmr:"bit:1"`
	// Reserved holds the reserved bits.
	Reserved uint8 `dmr:"bits:2-7"`
}

// TMPMessage is the payload of a TMP private or group text without an
// option field. Opcode.TMP splits the opcode of the message, and
// TMPOpcode builds it.
//
//dmr:stream
type TMPMessage struct {
	// RequestID identifies the text in its answer.
	RequestID uint32 `dmr:"u32"`
	// Destination is the radio IP of the radio or group.
	Destination [4]byte `dmr:"bytes:4"`
	// Source is the radio IP of the sender.
	Source [4]byte `dmr:"bytes:4"`
	// Text is the text in UTF-16LE.
	Text []byte `dmr:"rest,utf16le"`
}

// TMPAnswer is the payload of the answer to a private text. Opcode.TMP splits
// the opcode of the answer, and TMPOpcode builds it.
//
//dmr:stream
type TMPAnswer struct {
	// RequestID is the RequestID of the text.
	RequestID uint32 `dmr:"u32"`
	// Destination is the radio IP of the radio that got the text.
	Destination [4]byte `dmr:"bytes:4"`
	// Source is the radio IP of the sender of the text.
	Source [4]byte `dmr:"bytes:4"`
	// Result is 0 for success; other values name the failure, such as 4
	// for a busy channel.
	Result uint8 `dmr:"u8"`
}

// TMPGroupAnswer is the payload of the answer to a group text.
//
//dmr:stream
type TMPGroupAnswer struct {
	// RequestID is the RequestID of the text.
	RequestID uint32 `dmr:"u32"`
	// Destination is the radio IP of the group.
	Destination [4]byte `dmr:"bytes:4"`
	// Result is 0 for success.
	Result uint8 `dmr:"u8"`
}

// GPSData is the position of the location protocol: 40 ASCII octets that
// hold an NMEA-like fix.
//
//dmr:frame 40
//dmr:magic 144-151="."
//dmr:magic 232-239="."
type GPSData struct {
	// Status is 'A' for a valid fix and 'V' for none.
	Status uint8 `dmr:"bits:0-7"`
	// Hour, Minute and Second are the UTC time of the fix. A radio that
	// has no time sends NUL octets, which give 0.
	Hour   uint8 `dmr:"bits:8-23,dec,nulzero"`
	Minute uint8 `dmr:"bits:24-39,dec,nulzero"`
	Second uint8 `dmr:"bits:40-55,dec,nulzero"`
	// Day, Month and Year are the UTC date of the fix, Year in two digits.
	// A radio that has no date sends NUL octets, which give 0.
	Day   uint8 `dmr:"bits:56-71,dec,nulzero"`
	Month uint8 `dmr:"bits:72-87,dec,nulzero"`
	Year  uint8 `dmr:"bits:88-103,dec,nulzero"`
	// NorthSouth is 'N' or 'S'.
	NorthSouth uint8 `dmr:"bits:104-111"`
	// LatitudeDegrees, LatitudeMinutes and LatitudeFraction are the
	// latitude as DDMM.MMMM.
	LatitudeDegrees  uint8  `dmr:"bits:112-127,dec"`
	LatitudeMinutes  uint8  `dmr:"bits:128-143,dec"`
	LatitudeFraction uint16 `dmr:"bits:152-183,dec"`
	// EastWest is 'E' or 'W'.
	EastWest uint8 `dmr:"bits:184-191"`
	// LongitudeDegrees, LongitudeMinutes and LongitudeFraction are the
	// longitude as DDDMM.MMMM.
	LongitudeDegrees  uint16 `dmr:"bits:192-215,dec"`
	LongitudeMinutes  uint8  `dmr:"bits:216-231,dec"`
	LongitudeFraction uint16 `dmr:"bits:240-271,dec"`
	// Speed is the speed in knots as ASCII text, such as "0.2", or NUL
	// octets when the radio sends no speed.
	Speed [3]byte `dmr:"bits:272-295,packed"`
	// Direction is the course in degrees from north, 0 to 359. A radio
	// that sends no course sends NUL octets, which give 0.
	Direction uint16 `dmr:"bits:296-319,dec,nulzero"`
}

// LPRequest is the payload of a location protocol standard request.
//
//dmr:stream
type LPRequest struct {
	// RequestID identifies the request in its answer.
	RequestID uint32 `dmr:"u32"`
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
}

// LPAnswer is the payload of the answer to a standard request.
//
//dmr:stream
type LPAnswer struct {
	// RequestID is the RequestID of the request.
	RequestID uint32 `dmr:"u32"`
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
	// Result is 0 for success.
	Result uint16 `dmr:"u16"`
	// GPS is the position.
	GPS GPSData `dmr:"frame"`
}

// LPReport is the payload of a condition report: a position that the
// radio sends on a condition, such as a period.
//
//dmr:stream
type LPReport struct {
	// RequestID is the RequestID of the condition request.
	RequestID uint32 `dmr:"u32"`
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
	// GPS is the position.
	GPS GPSData `dmr:"frame"`
}

// LPEmergency is the payload of an emergency report.
//
//dmr:stream
type LPEmergency struct {
	// Radio is the radio IP of the radio.
	Radio [4]byte `dmr:"bytes:4"`
	// Type is the kind of emergency.
	Type uint8 `dmr:"u8"`
	// GPS is the position.
	GPS GPSData `dmr:"frame"`
}
