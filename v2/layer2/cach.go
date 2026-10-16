package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
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
	// of the Short LC BPTC transmit matrix.
	Payload [CACHPayloadBits]bit.Bit
}

// DecodeCACH decodes a 24-bit CACH field into its TACT and signalling payload.
// The input is in logical order; use CACHDeinterleave on bits as received.
func DecodeCACH(bits [CACHBits]bit.Bit) CACH {
	lowbit.Mask(bits[:])
	var result CACH

	var tactBits [TACTBits]bit.Bit
	copy(tactBits[:], bits[:TACTBits])
	result.TACT = pdu.DecodeTACT(tactBits)

	copy(result.Payload[:], bits[TACTBits:CACHBits])

	return result
}

// DecodeCACHField decodes a 24-bit CACH field in transmit order, as it is
// received, into its TACT and signalling payload.
func DecodeCACHField(onAir [CACHBits]bit.Bit) CACH {
	return DecodeCACH(CACHDeinterleave(onAir))
}

// Encode encodes a CACH struct into a 24-bit field in logical order.
// Use CACHInterleave to get the bits in transmit order.
func (c *CACH) Encode() ([CACHBits]bit.Bit, error) {
	var bits [CACHBits]bit.Bit

	tactBits, err := c.TACT.Encode()
	if err != nil {
		return bits, err
	}
	copy(bits[:TACTBits], tactBits[:])

	for i, b := range c.Payload {
		bits[TACTBits+i] = b & 1
	}

	return bits, nil
}
