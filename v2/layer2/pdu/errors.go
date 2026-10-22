package pdu

import "errors"

const errType = "pdu"

// Errors returned by the package. Match them with errors.Is.
var (
	// ErrTalkerAliasIncomplete is returned when the blocks that the header
	// length calls for have not all been received.
	ErrTalkerAliasIncomplete = errors.New("pdu: talker alias blocks missing")
	// ErrTalkerAliasLength is returned when an alias does not fit in a
	// header and three blocks.
	ErrTalkerAliasLength = errors.New("pdu: talker alias too long")
	// ErrTalkerAliasInvalidUTF8 is returned when the Code Units of a UTF-8
	// talker alias are not valid UTF-8, except for a last character that
	// is not complete.
	ErrTalkerAliasInvalidUTF8 = errors.New("pdu: talker alias is not valid UTF-8")
	// ErrTalkerAliasCharacter is returned by EncodeTalkerAlias when a
	// character of the alias is not in its 7-bit or ISO 8-bit format.
	ErrTalkerAliasCharacter = errors.New("pdu: a character of the talker alias is not in its format")
	// ErrNoAddresses is returned by the SetAddresses methods when the PDU
	// has no address fields: its FLCO, opcode or format has none.
	ErrNoAddresses = errors.New("pdu: PDU has no address fields")
	// ErrWrongOpcode is returned by a SetX method that takes the opcode,
	// such as FullLinkControl.SetTalkerAliasBlock, when the opcode does not
	// select the payload X.
	ErrWrongOpcode = errors.New("pdu: the opcode does not select the payload")
	// ErrNoFullLCMask is returned by FullLinkControl.Encode and
	// LCBytes.Codeword for a data type other than DataTypeVoiceLCHeader and
	// DataTypeTerminatorWithLC, which have no RS(12,9) Full LC mask.
	ErrNoFullLCMask = errors.New("pdu: the data type has no RS(12,9) Full LC mask")
	// ErrNoBlockCRC is returned, wrapped, by Seal for a data type that has
	// neither an RS(12,9) Full LC mask nor a single-block CRC-CCITT.
	ErrNoBlockCRC = errors.New("pdu: the data type has no single-block CRC-CCITT")
	// ErrNoGroupFlag is returned by the SetGroup methods when the PDU has
	// no group or individual flag: its FLCO, opcode or format has none.
	ErrNoGroupFlag = errors.New("pdu: PDU has no group or individual flag")
	// ErrNoServiceOptions is returned by LCBytes.SetServiceOptions when the
	// FLCO of the LC has no Service Options field.
	ErrNoServiceOptions = errors.New("pdu: PDU has no Service Options field")
)
