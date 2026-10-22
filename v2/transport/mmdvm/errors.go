package mmdvm

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a payload shorter than its message.
	ErrShort = errors.New("mmdvm: payload too short")
	// ErrLength is returned for a payload whose length is not one of the
	// lengths of its message, and wrapped by the encoders for a Size that
	// is not one of them or a frame longer than MaxFrameLen.
	ErrLength = errors.New("mmdvm: payload length does not match its message")
	// ErrCommand is returned when a frame is not a message that the call
	// decodes, for example DecodeDMRData of a status reply, and wrapped by
	// Debug.AppendBinary for a Command that is not a debug command.
	ErrCommand = errors.New("mmdvm: frame is not a message of this kind")
)

var (
	errFrameLength  error = &wrapError{"mmdvm: frame longer than MaxFrameLen", ErrLength}
	errDebugCommand error = &wrapError{"mmdvm: Debug.Command is not a debug command", ErrCommand}
	errHintVoice    error = &wrapError{"mmdvm: SetHint: voice position is not A to F", layer2.ErrVoicePosition}
	errHintData     error = &wrapError{"mmdvm: SetHint: no data type, or it does not fit in 4 bits", layer2.ErrDataType}
	errHintKind     error = &wrapError{"mmdvm: SetHint: kind is not KindVoice or KindData", layer2.ErrKind}

	errDMRStartTXRange = &dmrgo.FieldRangeError{Type: "DMRStart", Field: "TX", Bits: 8, Min: 0, Max: 1}
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
