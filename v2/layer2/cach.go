package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

const (
	// CACHBits is the total number of bits in a CACH field.
	CACHBits = 24
	// TACTBits is the number of bits in the TACT field.
	TACTBits = 7
	// CACHPayloadBits is the number of signalling payload bits.
	CACHPayloadBits = 17
)

// CACH is a decoded Common Announcement Channel field (ETSI TS 102 361-1
// clause 6.3).
type CACH struct {
	// TACT is the decoded Timing, Activity, Channel, Type PDU.
	TACT pdu.TACT

	// Payload contains the 17-bit CACH signalling payload, which is one row
	// of the Short LC BPTC transmit matrix, packed, the first bit in the most
	// significant bit of byte 0. The low 7 bits of byte 2 are 0.
	Payload [3]byte
}

// DecodeCACH decodes a 24-bit CACH field into its TACT and signalling
// payload. bits holds the field in logical order, packed, the first bit in
// the most significant bit of byte 0; use CACHDeinterleave on a field as
// received.
func DecodeCACH(bits [3]byte) CACH {
	v := uint32(bits[0])<<16 | uint32(bits[1])<<8 | uint32(bits[2])
	p := v << 7
	return CACH{
		TACT:    pdu.DecodeTACT([1]byte{bits[0]}),
		Payload: [3]byte{byte(p >> 16 & 0xFF), byte(p >> 8 & 0xFF), byte(p & 0x80)},
	}
}

// DecodeCACHField decodes a 24-bit CACH field in transmit order, as it is
// received, into its TACT and signalling payload. onAir holds the bits
// packed as DecodeCACH takes them.
func DecodeCACHField(onAir [3]byte) CACH {
	return DecodeCACH(CACHDeinterleave(onAir))
}

// errCACHPayloadRange is the error of an Encode whose Payload has a padding
// bit set.
var errCACHPayloadRange = &bit.FieldRangeError{Type: "CACH", Field: "Payload", Bits: CACHPayloadBits}

// Encode encodes a CACH struct into a 24-bit field in logical order, packed
// as DecodeCACH takes it. Use CACHInterleave to get the bits in transmit
// order. It returns an error when a TACT field does not fit, or when one of
// the low 7 bits of Payload[2] is set.
func (c *CACH) Encode() ([3]byte, error) {
	tact, err := c.TACT.Encode()
	if err != nil {
		return [3]byte{}, err
	}
	if c.Payload[2]&0x7F != 0 {
		return [3]byte{}, errCACHPayloadRange
	}
	return [3]byte{tact[0] | c.Payload[0]>>7, c.Payload[0]<<1 | c.Payload[1]>>7, c.Payload[1]<<1 | c.Payload[2]>>7}, nil
}
