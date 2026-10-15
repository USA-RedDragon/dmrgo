package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §6.3: Common Announcement Channel (CACH)
//
// The CACH is a 24-bit field on the outbound channel that carries timing
// and activity information common to both TDMA channels.
//
// Logical layout after CACHDeinterleave (Figure 6.6, Figure B.9):
//
//	Bits 0 to 6:   TACT (7 bits: AT + TC + LCSS + Hamming(7,4,3) parity)
//	Bits 7 to 23:  CACH Signalling payload (17 bits: one row of Short LC BPTC)

const (
	// CACHBits is the total number of bits in a CACH field.
	CACHBits = 24
	// TACTBits is the number of bits in the TACT field.
	TACTBits = 7
	// CACHPayloadBits is the number of signalling payload bits.
	CACHPayloadBits = 17
)

// CACH represents a decoded Common Announcement Channel field.
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

	// Extract TACT (bits 0..6) and decode with Hamming(7,4,3) FEC
	var tactBits [TACTBits]bit.Bit
	copy(tactBits[:], bits[:TACTBits])
	result.TACT = pdu.DecodeTACT(tactBits)

	// Extract signalling payload (bits 7..23)
	copy(result.Payload[:], bits[TACTBits:CACHBits])

	return result
}

// DecodeCACHField decodes a 24-bit CACH field in transmit order, as it is
// received, into its TACT and signalling payload: it is DecodeCACH of
// CACHDeinterleave(onAir).
func DecodeCACHField(onAir [CACHBits]bit.Bit) CACH {
	return DecodeCACH(CACHDeinterleave(onAir))
}

// Encode encodes a CACH struct into a 24-bit field in logical order.
// Use CACHInterleave to get the bits in transmit order.
func (c *CACH) Encode() ([CACHBits]bit.Bit, error) {
	var bits [CACHBits]bit.Bit

	// Encode TACT into bits 0..6
	tactBits, err := c.TACT.Encode()
	if err != nil {
		return bits, err
	}
	copy(bits[:TACTBits], tactBits[:])

	// Copy signalling payload into bits 7..23
	copy(bits[TACTBits:CACHBits], c.Payload[:])

	return bits, nil
}
