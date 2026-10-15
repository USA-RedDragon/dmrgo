package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
	// RCCommand is the 4-bit RC information field.
	// Per TS 102 361-2 §7.2.20, this carries an RC command.
	RCCommand enums.RCCommand

	// FEC holds the combined FEC result from BPTC + CRC verification.
	FEC fec.Result
}

// DecodeReverseChannel decodes an 11-bit RC PDU (after Single Burst BPTC decode).
// The input is the 11 information bits: 4-bit payload + 7-bit CRC-7.
func DecodeReverseChannel(infoBits [11]bit.Bit) (ReverseChannel, fec.Result) {
	var result ReverseChannel
	var fecResult fec.Result

	// Extract RC command (bits 0-3)
	result.RCCommand = enums.RCCommand(bit.Uint(infoBits[:4])) //nolint:gosec // 4-bit field

	// CRC-7 verification with mask 0x7A (Table B.21)
	if !crc.CheckCRC7(infoBits[:4], uint8(bit.Uint(infoBits[4:]))^CRC7MaskRC) { //nolint:gosec // 7-bit field
		fecResult.Uncorrectable = true
	}

	result.FEC = fecResult
	return result, fecResult
}

// Encode encodes an RC PDU into 11 information bits.
// The output contains the 4-bit payload followed by 7-bit CRC-7 (masked).
// It returns an error if RCCommand does not fit in 4 bits.
func (rc *ReverseChannel) Encode() ([11]bit.Bit, error) {
	var bits [11]bit.Bit

	if rc.RCCommand > 0x0F {
		return bits, fmt.Errorf("RC command %d out of range 0-15", int(rc.RCCommand))
	}

	// RC command (bits 0-3, MSB-first)
	bit.PutUint(bits[0:4], uint64(uint8(rc.RCCommand)))

	// CRC-7 with mask 0x7A
	crcVal := crc.CRC7(bits[:4]) ^ CRC7MaskRC
	for i := 0; i < 7; i++ {
		bits[4+i] = bit.Bit((crcVal >> (6 - i)) & 1)
	}

	return bits, nil
}

// String returns a human-readable string representation of the RC PDU.
func (rc *ReverseChannel) String() string {
	return fmt.Sprintf("ReverseChannel{ RCCommand: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }",
		rc.RCCommand.String(), rc.FEC.BitsChecked, rc.FEC.ErrorsCorrected, rc.FEC.Uncorrectable)
}
