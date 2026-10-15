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
)
