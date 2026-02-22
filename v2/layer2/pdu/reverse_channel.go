package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 — 9.1.5 Reverse Channel (RC) PDU
//
// The RC PDU carries 11 information bits recovered from Single Burst
// Variable Length BPTC (§B.2.2) with odd column parity.
//
// Layout (Table 9.6):
//
//	Bits 0–3:   RC information payload (4 bits)
//	Bits 4–10:  CRC-7 with mask 0x7A (7 bits, per §B.3.13 / Table B.21)
//
// The 4-bit RC payload is defined in TS 102 361-4 (trunking protocol).

// CRC7MaskRC is the CRC-7 data type mask for Reverse Channel PDUs
// per ETSI TS 102 361-1 Table B.21.
const CRC7MaskRC uint8 = 0x7A

// ReverseChannel represents a decoded Reverse Channel PDU.
type ReverseChannel struct {
	// RCPayload is the 4-bit RC information field.
	// The meaning of this field is defined in TS 102 361-4.
	RCPayload byte

	// FEC holds the combined FEC result from BPTC + CRC verification.
	FEC fec.FECResult
}

// DecodeReverseChannel decodes an 11-bit RC PDU (after Single Burst BPTC decode).
// The input is the 11 information bits: 4-bit payload + 7-bit CRC-7.
func DecodeReverseChannel(infoBits [11]bit.Bit) (ReverseChannel, fec.FECResult) {
	var result ReverseChannel
	var fecResult fec.FECResult
	fecResult.BitsChecked = 11

	// Extract RC payload (bits 0-3)
	result.RCPayload = bit.BitsToUint8(infoBits[:], 0, 4)

	// CRC-7 verification with mask 0x7A (Table B.21)
	if !crc.CheckCRC7(infoBits[:], CRC7MaskRC) {
		fecResult.Uncorrectable = true
	}

	result.FEC = fecResult
	return result, fecResult
}

// EncodeReverseChannel encodes an RC PDU into 11 information bits.
// The output contains the 4-bit payload followed by 7-bit CRC-7 (masked).
func EncodeReverseChannel(rc *ReverseChannel) [11]bit.Bit {
	var bits [11]bit.Bit

	// RC payload (bits 0-3, MSB-first)
	copy(bits[0:4], bit.BitsFromUint8(rc.RCPayload&0x0F, 4))

	// CRC-7 with mask 0x7A
	crcVal := crc.CalculateCRC7(bits[:4]) ^ CRC7MaskRC
	for i := 0; i < 7; i++ {
		bits[4+i] = bit.Bit((crcVal >> (6 - i)) & 1)
	}

	return bits
}

// ToString returns a human-readable string representation of the RC PDU.
func (rc *ReverseChannel) ToString() string {
	return fmt.Sprintf("ReverseChannel{ RCPayload: %d, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }",
		rc.RCPayload, rc.FEC.BitsChecked, rc.FEC.ErrorsCorrected, rc.FEC.Uncorrectable)
}
