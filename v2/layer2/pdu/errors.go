package pdu

import "errors"

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrTalkerAliasIncomplete is returned when the blocks that the header
	// length calls for have not all been received.
	ErrTalkerAliasIncomplete = errors.New("pdu: talker alias blocks missing")
	// ErrTalkerAliasLength is returned when an alias does not fit in a
	// header and three blocks.
	ErrTalkerAliasLength = errors.New("pdu: talker alias too long")
	// ErrTalkerAliasInvalidUTF8 is returned when the Code Units of a UTF-8
	// talker alias are not valid UTF-8.
	ErrTalkerAliasInvalidUTF8 = errors.New("pdu: talker alias is not valid UTF-8")
	// ErrNoAddresses is returned by the SetAddresses methods when the PDU
	// has no address fields: its FLCO, opcode or format has none, or the
	// payload field that holds them is nil.
	ErrNoAddresses = errors.New("pdu: PDU has no address fields")
	errNil         = errors.New("pdu: nil pointer argument")
	// ErrWrongOpcode is returned by a SetX method that takes the opcode,
	// such as FullLinkControl.SetTalkerAliasBlock, when the opcode does not
	// select the payload X.
	ErrWrongOpcode = errors.New("the opcode does not select the payload")
	// ErrNoFullLCMask is returned by FullLinkControl.Encode and
	// LCBytes.Codeword for a data type other than DataTypeVoiceLCHeader and
	// DataTypeTerminatorWithLC, which have no RS(12,9) Full LC mask.
	ErrNoFullLCMask = errors.New("the data type has no RS(12,9) Full LC mask")
	// ErrNoGroupFlag is returned by the SetGroup methods when the PDU has
	// no group or individual flag: its FLCO, opcode or format has none.
	ErrNoGroupFlag = errors.New("PDU has no group or individual flag")
	// ErrNoServiceOptions is returned by LCBytes.SetServiceOptions when the
	// FLCO of the LC has no Service Options field.
	ErrNoServiceOptions = errors.New("pdu: PDU has no Service Options field")
)
