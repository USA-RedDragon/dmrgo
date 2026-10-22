package ars

import "github.com/USA-RedDragon/dmrgo/v2"

var errValue = &dmrgo.FieldRangeError{Type: "ars", Field: "Value", Bits: 7}

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
// points Device at them and returns dst. It allocates only to grow dst.
func (m *Message) SetDeviceID(dst []byte, id uint32) []byte {
	if m == nil {
		return dst
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
	return dst
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
// the reason reason. It returns a *dmrgo.FieldRangeError when reason does
// not fit in 7 bits.
func NewFailure(reason FailureReason) (Message, error) {
	if reason > 127 {
		return Message{}, errValue
	}
	return Message{
		Header:   Header{Extension: true, AckRequired: true, Priority: true, Control: true, Type: TypeAck},
		Response: Response{Value: uint8(reason)},
	}, nil
}

// NewQuery returns the query that asks a radio to register again.
func NewQuery() Message {
	return Message{Header: Header{Control: true, Type: TypeQuery}}
}
