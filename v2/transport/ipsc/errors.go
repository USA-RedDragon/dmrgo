package ipsc

import "errors"

// Errors of the decoders. Each is a static value, so a decode that fails
// does not allocate. Match them with errors.Is.
var (
	// ErrShort is returned for a packet shorter than its message.
	ErrShort = errors.New("ipsc: packet too short")
	// ErrLength is returned for a packet whose length does not match its
	// message.
	ErrLength = errors.New("ipsc: packet length does not match its message")
	// ErrCommand is returned when the opcode is not one that the call
	// decodes, for example DecodeUser of a keepalive.
	ErrCommand = errors.New("ipsc: packet is not a message of this kind")
)

var (
	errOpcode   = errors.New("ipsc: the opcode does not take this message")
	errLayout   = errors.New("ipsc: Voice.Layout is not A, BCDF or E")
	errPayload  = errors.New("ipsc: Data.Bits or Data.TrailerLen does not fit")
	errBurst    = errors.New("ipsc: the burst cannot be rebuilt from this packet")
	errKind     = errors.New("ipsc: SetRawBurst: kind is not KindVoice or KindData")
	errDataType = errors.New("ipsc: SetRawBurst: IPSC carries no burst of this data type")
)
