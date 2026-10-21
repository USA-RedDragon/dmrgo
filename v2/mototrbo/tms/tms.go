package tms

import "github.com/USA-RedDragon/dmrgo/v2/internal/text"

// Type is the kind of a message: the control bit and the 4-bit PDU type of
// the first header octet.
type Type uint8

// The Type values.
const (
	TypeText         Type = 0x00 // a text message
	TypeAvailability Type = 0x10 // a service availability message
	TypeAck          Type = 0x1F // the acknowledgement of a text message
)

// String returns the name of t. It allocates only for a value without a
// name.
func (t Type) String() string {
	switch t {
	case TypeText:
		return "Text"
	case TypeAvailability:
		return "Availability"
	case TypeAck:
		return "Ack"
	}
	return "Type(" + text.Int(int64(t)) + ")"
}

// Encoding is the 5-bit character coding of the text.
type Encoding uint8

// The Encoding values.
const (
	EncodingNone    Encoding = 0 // no coding given
	EncodingUTF16LE Encoding = 4 // UTF-16LE (UCS-2)
)

// String returns the name of e. It allocates only for a value without a
// name.
func (e Encoding) String() string {
	switch e {
	case EncodingNone:
		return "None"
	case EncodingUTF16LE:
		return "UTF-16LE"
	}
	return "Encoding(" + text.Int(int64(e)) + ")"
}

// Header is the first header octet of a message.
//
//dmr:frame 1
type Header struct {
	// Extension reports that more header octets follow the address.
	Extension bool `dmr:"bit:0"`
	// AckRequired asks the receiver to send an acknowledgement.
	AckRequired bool `dmr:"bit:1"`
	// Reserved is the reserved bit. Radios set it in a text message.
	Reserved bool `dmr:"bit:2"`
	// Type is the kind of the message.
	Type Type `dmr:"bits:3-7,enum"`
}

// SequenceLow is the first extension octet of a text message or an
// acknowledgement.
//
//dmr:frame 1
type SequenceLow struct {
	// Extension reports that SequenceHigh follows.
	Extension bool `dmr:"bit:0"`
	// Reserved holds the reserved bits.
	Reserved uint8 `dmr:"bits:1-2"`
	// Low holds the low 5 bits of the sequence number.
	Low uint8 `dmr:"bits:3-7"`
}

// SequenceHigh is the second extension octet of a text message or an
// acknowledgement.
//
//dmr:frame 1
type SequenceHigh struct {
	// Extension reports that a further header octet follows.
	Extension bool `dmr:"bit:0"`
	// High holds the high 2 bits of the sequence number.
	High uint8 `dmr:"bits:1-2"`
	// Encoding is the coding of the text.
	Encoding Encoding `dmr:"bits:3-7,enum"`
}

// Message is a MOTOTRBO Text Messaging Service message, the payload of a
// UDP datagram to port 4007.
//
//dmr:stream
type Message struct {
	// Length is the number of octets after it. AppendBinary sets it.
	Length uint16 `dmr:"u16,length"`
	// Header is the first header octet.
	Header Header `dmr:"frame"`
	// Address is the address field, empty in the radio to radio messages
	// that are known.
	Address []byte `dmr:"lv8"`
	// Capability is the octet after the address of an availability message
	// with Header.Extension set; its low 2 bits give the capability of the
	// device.
	Capability uint8 `dmr:"u8,if:Header.Extension&Header.Type==TypeAvailability"`
	// SequenceLow is the first extension octet of a text message or an
	// acknowledgement, when Header.Extension is set.
	SequenceLow SequenceLow `dmr:"frame,if:Header.Extension&Header.Type!=TypeAvailability"`
	// SequenceHigh is the second extension octet, when
	// SequenceLow.Extension is set.
	SequenceHigh SequenceHigh `dmr:"frame,if:Header.Extension&Header.Type!=TypeAvailability&SequenceLow.Extension"`
	// Text is the text in UTF-16LE. Radios start it with a CR LF.
	Text []byte `dmr:"rest,utf16le"`
}
