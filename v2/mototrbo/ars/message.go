package ars

import "github.com/USA-RedDragon/dmrgo/v2"

const typeName = "ars"

var (
	errValue    = &dmrgo.FieldRangeError{Type: typeName, Field: "Value", Bits: 7}
	errDeviceID = &dmrgo.FieldRangeError{Type: typeName, Field: "SetDeviceID", Bits: 24}
	errReason   = &dmrgo.ConfigError{Type: typeName, Field: "reason"}
)

// DeviceID returns the radio ID that Device holds, and false when Device is
// empty, holds a character that is not a digit or a number over 24 bits.
func (m *Message) DeviceID() (uint32, bool) {
	if m == nil {
		return 0, false
	}
	if len(m.Device) == 0 || len(m.Device) > 8 {
		return 0, false
	}
	var v uint32
	for _, c := range m.Device {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + uint32(c-'0')
	}
	return v, v <= 0xFFFFFF
}

// SetDeviceID appends the radio ID id to dst in decimal ASCII digits,
// points Device at them and returns dst. It returns dst unchanged and a
// *dmrgo.FieldRangeError when id does not fit in 24 bits, and
// dmrgo.ErrNilReceiver for a nil m. It allocates only to grow dst.
func (m *Message) SetDeviceID(dst []byte, id uint32) ([]byte, error) {
	if m == nil {
		return dst, dmrgo.ErrNilReceiver
	}
	if id > 0xFFFFFF {
		return dst, errDeviceID
	}
	n := len(dst)
	var b [10]byte
	i := len(b)
	for {
		i--
		b[i] = byte('0' + id%10)
		id /= 10
		if id == 0 {
			break
		}
	}
	dst = append(dst, b[i:]...)
	m.Device = dst[n:]
	return dst, nil
}

// NewRegistration returns a device registration with the header bits that
// radios send. Set Device with SetDeviceID.
func NewRegistration(event Event) Message {
	return Message{
		Header:       Header{Extension: true, AckRequired: true, Priority: true, Control: true, Type: TypeDeviceRegistration},
		Registration: Registration{Event: event},
	}
}

// NewAck returns the answer to a successful device registration, with the
// refresh time in units of 30 minutes, 0 for none. It returns a
// *dmrgo.FieldRangeError when refresh does not fit in 7 bits.
func NewAck(refresh uint8) (Message, error) {
	if refresh > 127 {
		return Message{}, errValue
	}
	return Message{
		Header:   Header{Extension: true, Priority: true, Control: true, Type: TypeAck},
		Response: Response{Value: refresh},
	}, nil
}

// NewFailure returns the answer to a device registration that failed, with
// the reason reason, 00 02 FF 00 on the wire for FailureDeviceNotAuthorized.
// The answer defines only that reason (MOTOTRBO ARS specification 02.00
// clause 3.4.3), so NewFailure returns a *dmrgo.ConfigError for each other
// value.
func NewFailure(reason FailureReason) (Message, error) {
	if reason != FailureDeviceNotAuthorized {
		return Message{}, errReason
	}
	return Message{
		Header:   Header{Extension: true, AckRequired: true, Priority: true, Control: true, Type: TypeAck},
		Response: Response{Value: uint8(reason)},
	}, nil
}

// NewQuery returns the query that requests a radio to register again, with the
// header bits that servers send: acknowledgement required and priority, 00
// 01 74 on the wire.
func NewQuery() Message {
	return Message{Header: Header{AckRequired: true, Priority: true, Control: true, Type: TypeQuery}}
}
