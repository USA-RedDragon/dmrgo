package ipsc

import (
	"errors"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a packet shorter than its message.
	ErrShort = errors.New("ipsc: packet too short")
	// ErrLength is returned for a packet whose length does not match its
	// message, and wrapped by User.AppendBinary for a data payload that does
	// not fit and by Control.AppendBinary for a peer list with entries.
	ErrLength = errors.New("ipsc: packet length does not match its message")
	// ErrCommand is returned when the opcode is not one that the call
	// decodes, for example DecodeUser of a keepalive. The encoders return it
	// wrapped for an opcode that is not one of their messages, and
	// User.RawBurst for a packet that carries no burst.
	ErrCommand = errors.New("ipsc: packet is not a message of this kind")
)

var (
	errOpcode        error = &wrapError{"ipsc: the opcode does not take this message", ErrCommand}
	errPeerList      error = &wrapError{"ipsc: Control.AppendBinary writes no peer entries: use AppendPeerListReply", ErrLength}
	errLayout        error = &wrapError{"ipsc: Voice.Layout is not A, BCDF or E", layer2.ErrVoicePosition}
	errPayload       error = &wrapError{"ipsc: Data.Bits or Data.TrailerLen does not fit", ErrLength}
	errBurst         error = &wrapError{"ipsc: the packet carries no burst that can be rebuilt", ErrCommand}
	errBurstSync     error = &wrapError{"ipsc: RawBurst: the sync source has no SYNC pattern", layer2.ErrSyncSource}
	errBurstSlotType error = &wrapError{"ipsc: RawBurst: the BurstRaw packet has no slot type", layer2.ErrDataType}
	errKind          error = &wrapError{"ipsc: SetRawBurst: kind is not KindVoice or KindData", layer2.ErrKind}

	errVoicePosition error = &wrapError{"ipsc: SetRawBurst: voice position is not A to F", layer2.ErrVoicePosition}
	errDataType      error = &wrapError{"ipsc: SetRawBurst: IPSC carries no burst of this data type", layer2.ErrDataType}

	errCallInfoSlotRange    = &dmrgo.FieldRangeError{Type: "CallInfo", Field: "SetSlot", Bits: 1}
	errUserSourceRange      = &dmrgo.FieldRangeError{Type: "User", Field: "Source", Bits: 24}
	errUserDestinationRange = &dmrgo.FieldRangeError{Type: "User", Field: "Destination", Bits: 24}
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
