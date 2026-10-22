package dmrd

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a packet shorter than its message.
	ErrShort = errors.New("dmrd: packet too short")
	// ErrLength is returned for a packet whose length is not one of the
	// lengths of its message, and wrapped by the encoders for a Size or Text
	// that does not fit the message.
	ErrLength = errors.New("dmrd: packet length does not match its message")
	// ErrSignature is returned when the fixed octets of a message, such as
	// its tag, do not match, and wrapped by Bridge.AppendBinary for a
	// Version other than 1, 4 and 5.
	ErrSignature = errors.New("dmrd: packet does not have the octets of its message")
	// ErrText is returned when a number sent as text holds a character that
	// is not a digit.
	ErrText = errors.New("dmrd: number holds a character that is not a digit")
	// ErrCommand is returned when a packet is not a message that the call
	// decodes in the dialect, for example DecodeControl of a DMRD packet.
	// The encoders, and dmrdauth for a dialect without a login token or a
	// packet that is not OpenBridge, return it wrapped.
	ErrCommand = errors.New("dmrd: packet is not a message of this kind")
)

var (
	errDialectCommand error = &wrapError{"dmrd: the dialect has no such message", ErrCommand}
	errTextLength     error = &wrapError{"dmrd: Text is too long or has the wrong length for the message", ErrLength}
	errBridgeVersion  error = &wrapError{"dmrd: Bridge.Version is not 1, 4 or 5", ErrSignature}
	errBridgeData     error = &wrapError{"dmrd: Bridge.Data.Size does not fit Bridge.Version", ErrLength}
	errKindVoice      error = &wrapError{"dmrd: SetHint: voice position is not A to F", layer2.ErrVoicePosition}
	errKindData       error = &wrapError{"dmrd: SetHint: no data type, or it does not fit in 4 bits", layer2.ErrDataType}
	errKind           error = &wrapError{"dmrd: SetHint: kind is not KindVoice or KindData", layer2.ErrKind}

	errTalkerAliasTypeRange      = &dmrgo.FieldRangeError{Type: "TalkerAlias", Field: "Type", Bits: 2}
	errLocalTalkerAliasTypeRange = &dmrgo.FieldRangeError{Type: "LocalTalkerAlias", Field: "Type", Bits: 2}
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
