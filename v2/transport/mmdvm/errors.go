package mmdvm

import "errors"

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a payload shorter than its message.
	ErrShort = errors.New("mmdvm: payload too short")
	// ErrLength is returned for a payload whose length is not one of the
	// lengths of its message.
	ErrLength = errors.New("mmdvm: payload length does not match its message")
	// ErrCommand is returned when a frame is not a message that the call
	// decodes, for example DecodeDMRData of a status reply.
	ErrCommand = errors.New("mmdvm: frame is not a message of this kind")
)

var (
	errFrameLength  = errors.New("mmdvm: frame longer than MaxFrameLen")
	errDebugCommand = errors.New("mmdvm: Debug.Command is not a debug command")
	errHintVoice    = errors.New("mmdvm: SetHint: voice position is not A to F")
	errHintData     = errors.New("mmdvm: SetHint: no data type, or it does not fit in 4 bits")
	errHintKind     = errors.New("mmdvm: SetHint: kind is not KindVoice or KindData")
	errNilDMRData   = errors.New("mmdvm: nil *DMRData")
	errNilShortLC   = errors.New("mmdvm: NewDMRShortLC: nil *pdu.ShortLC")
)
