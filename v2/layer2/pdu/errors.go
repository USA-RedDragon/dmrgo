package pdu

import "errors"

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrTalkerAliasIncomplete is returned when the blocks that the header
	// length calls for have not all been received.
	ErrTalkerAliasIncomplete = errors.New("talker alias blocks missing")
	// ErrTalkerAliasLength is returned when an alias does not fit in a
	// header and three blocks.
	ErrTalkerAliasLength = errors.New("talker alias too long")
	// ErrTalkerAliasInvalidUTF8 is returned when the Code Units of a UTF-8
	// talker alias are not valid UTF-8.
	ErrTalkerAliasInvalidUTF8 = errors.New("talker alias is not valid UTF-8")
	// ErrNoAddresses is returned by the SetAddresses methods when the PDU
	// has no address fields: its FLCO, opcode or format has none, or the
	// payload field that holds them is nil.
	ErrNoAddresses = errors.New("the PDU has no address fields")
)
