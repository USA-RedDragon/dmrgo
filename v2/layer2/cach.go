package layer2

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 §6.3 — Common Announcement Channel (CACH)
//
// The CACH is a 24-bit field on the outbound channel that carries timing
// and activity information common to both TDMA channels.
//
// Layout (Figure 6.6):
//
//	Bits 0–6:   TACT (7 bits: AT + TC + LCSS + Hamming(7,4,3) parity)
//	Bits 7–23:  CACH Signalling payload (17 bits: one row of Short LC BPTC)

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

	// FEC accumulates the total FEC result from TACT decoding.
	FEC fec.FECResult
}

// DecodeCACH decodes a 24-bit CACH field into its TACT and signalling payload.
func DecodeCACH(bits [CACHBits]bit.Bit) CACH {
	var result CACH

	// Extract TACT (bits 0..6) and decode with Hamming(7,4,3) FEC
	var tactBits [TACTBits]bit.Bit
	copy(tactBits[:], bits[:TACTBits])
	result.TACT, result.FEC = pdu.DecodeTACT(tactBits)

	// Extract signalling payload (bits 7..23)
	copy(result.Payload[:], bits[TACTBits:CACHBits])

	return result
}

// EncodeCACH encodes a CACH struct into a 24-bit field.
func EncodeCACH(c *CACH) [CACHBits]bit.Bit {
	var bits [CACHBits]bit.Bit

	// Encode TACT into bits 0..6
	tactBits := pdu.EncodeTACT(&c.TACT)
	copy(bits[:TACTBits], tactBits[:])

	// Copy signalling payload into bits 7..23
	copy(bits[TACTBits:CACHBits], c.Payload[:])

	return bits
}
