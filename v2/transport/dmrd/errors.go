package dmrd

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a packet shorter than its message.
	ErrShort = errors.New("dmrd: packet too short")
	// ErrLength is returned for a packet whose length is not one of the
	// lengths of its message.
	ErrLength = errors.New("dmrd: packet length does not match its message")
	// ErrSignature is returned when the fixed octets of a message, such as
	// its tag, do not match.
	ErrSignature = errors.New("dmrd: packet does not have the octets of its message")
	// ErrText is returned when a number sent as text holds a character that
	// is not a digit.
	ErrText = errors.New("dmrd: number holds a character that is not a digit")
	// ErrCommand is returned when a packet is not a message that the call
	// decodes in the dialect, for example DecodeControl of a DMRD packet.
	ErrCommand = errors.New("dmrd: packet is not a message of this kind")
)

var (
	errDialectCommand       = errors.New("dmrd: the dialect has no such message")
	errTextLength           = errors.New("dmrd: Text is too long or has the wrong length for the message")
	errBridgeVersion        = errors.New("dmrd: Bridge.Version is not 1, 4 or 5")
	errBridgeData           = errors.New("dmrd: Bridge.Data does not encode")
	errKindVoice      error = &wrapError{"dmrd: SetHint: voice position is not A to F", layer2.ErrVoicePosition}
	errKindData       error = &wrapError{"dmrd: SetHint: no data type, or it does not fit in 4 bits", layer2.ErrDataType}
	errKind                 = errors.New("dmrd: SetHint: kind is not KindVoice or KindData")
)

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}
