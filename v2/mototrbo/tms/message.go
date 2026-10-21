package tms

import "github.com/USA-RedDragon/dmrgo/v2"

var errSequence = &dmrgo.FieldRangeError{Type: "tms", Field: "Sequence", Bits: 7}

// Sequence returns the 7-bit sequence number of a text message or an
// acknowledgement, and false when the message has none.
func (m *Message) Sequence() (uint8, bool) {
	if m == nil {
		return 0, false
	}
	if !m.Header.Extension || m.Header.Type == TypeAvailability {
		return 0, false
	}
	return m.SequenceHigh.High<<5 | m.SequenceLow.Low, true
}

// NewText returns a text message with sequence number seq, with the header
// bits that radios send: both extension octets and the UTF-16LE encoding.
// Set its text with SetTextUTF8. It returns a *dmrgo.FieldRangeError when
// seq does not fit in 7 bits.
func NewText(seq uint8, ackRequired bool) (Message, error) {
	if seq > 127 {
		return Message{}, errSequence
	}
	return Message{
		Header:       Header{Extension: true, AckRequired: ackRequired, Reserved: true, Type: TypeText},
		SequenceLow:  SequenceLow{Extension: true, Low: seq & 0x1F},
		SequenceHigh: SequenceHigh{High: seq >> 5, Encoding: EncodingUTF16LE},
	}, nil
}

// NewAck returns the acknowledgement of the text message with sequence
// number seq. It returns a *dmrgo.FieldRangeError when seq does not fit in
// 7 bits.
func NewAck(seq uint8) (Message, error) {
	if seq > 127 {
		return Message{}, errSequence
	}
	return Message{
		Header:       Header{Extension: true, Type: TypeAck},
		SequenceLow:  SequenceLow{Extension: seq > 31, Low: seq & 0x1F},
		SequenceHigh: SequenceHigh{High: seq >> 5},
	}, nil
}
