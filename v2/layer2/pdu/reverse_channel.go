package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// CRC7MaskRC is the CRC-7 data type mask for Reverse Channel PDUs (ETSI TS
// 102 361-1 Table B.21).
const CRC7MaskRC uint8 = 0x7A

// ReverseChannel is the Reverse Channel (RC) PDU: 4 information bits and a
// CRC-7 masked with CRC7MaskRC, carried in Single Burst BPTC (ETSI TS 102
// 361-1 clause 9.1.5).
type ReverseChannel struct {
	// RCCommand is the 4-bit RC command (ETSI TS 102 361-2 clause 7.2.20).
	RCCommand enums.RCCommand

	// FEC reports the CRC-7 check.
	FEC fec.Result
}

// DecodeReverseChannel decodes an 11-bit RC PDU (after Single Burst BPTC decode).
// The input is the 11 information bits: 4-bit payload + 7-bit CRC-7. The
// FEC field reports the CRC-7 check.
func DecodeReverseChannel(infoBits [11]bit.Bit) ReverseChannel {
	var result ReverseChannel
	lowbit.Mask(infoBits[:])
	result.RCCommand = enums.RCCommand(bit.Uint(infoBits[:4]) & 0xFF)
	cmd := [1]byte{byte(result.RCCommand) << 4}
	result.FEC.Uncorrectable = !crc.CheckCRC7(cmd[:], 4, uint8(bit.Uint(infoBits[4:])&0xFF)^CRC7MaskRC)
	return result
}

// errReverseChannelRCCommandRange is the error of an Encode whose RCCommand
// does not fit in 4 bits.
var errReverseChannelRCCommandRange = &bit.FieldRangeError{Type: "ReverseChannel", Field: "RCCommand", Bits: 4}

// Encode encodes an RC PDU into 11 information bits.
// The output contains the 4-bit payload followed by 7-bit CRC-7 (masked).
// It returns an error if RCCommand does not fit in 4 bits.
func (rc *ReverseChannel) Encode() ([11]bit.Bit, error) {
	var bits [11]bit.Bit

	if rc.RCCommand > 0x0F {
		return bits, errReverseChannelRCCommandRange
	}

	bit.PutUint(bits[0:4], uint64(uint8(rc.RCCommand)))

	cmd := [1]byte{byte(rc.RCCommand) << 4}
	crcVal := crc.CRC7(cmd[:], 4) ^ CRC7MaskRC
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
