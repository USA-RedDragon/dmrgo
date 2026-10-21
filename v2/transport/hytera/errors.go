package hytera

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a packet shorter than its message.
	ErrShort = errors.New("hytera: packet too short")
	// ErrLength is returned for a packet whose length does not match its
	// message.
	ErrLength = errors.New("hytera: packet length does not match its message")
	// ErrSignature is returned when the fixed octets of a message, such as
	// "ZZZZ" or "P2P", do not match.
	ErrSignature = errors.New("hytera: packet does not have the octets of its message")
	// ErrCode is returned when a slot or color code field is not one of its
	// values: 1111 or 2222 for the slot, one nibble four times for the color
	// code.
	ErrCode = errors.New("hytera: slot or color code field is not one of its values")
)

var (
	errSlot  = errors.New("hytera: Frame.Slot is not Slot1 or Slot2")
	errColor = errors.New("hytera: Frame.ColorCode is above 15")
	errKind  = errors.New("hytera: SetHint: kind is not KindVoice or KindData")

	errKindVoice error = &wrapError{"hytera: SetHint: voice position is not A to F", layer2.ErrVoicePosition}
	errKindData  error = &wrapError{"hytera: SetHint: no data type, or one that has no slot type", layer2.ErrDataType}
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
