package pdu

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
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

// DecodeReverseChannel decodes an RC PDU from its 11 information bits, after
// the Single Burst BPTC decode: the 4-bit RC command and the masked CRC-7.
// info holds the bits packed, the first bit in the most significant bit of
// info[0], and the low 5 bits of info[1] are ignored. The FEC field reports
// the CRC-7 check.
func DecodeReverseChannel(info [2]byte) ReverseChannel {
	var result ReverseChannel
	result.RCCommand = enums.RCCommand(info[0] >> 4)
	cmd := [1]byte{info[0] & 0xF0}
	result.FEC.Uncorrectable = !crc.CheckCRC7(cmd[:], 4, (info[0]<<3|info[1]>>5)&0x7F^CRC7MaskRC)
	return result
}

// errReverseChannelRCCommandRange is the error of an Encode whose RCCommand
// does not fit in 4 bits.
var errReverseChannelRCCommandRange = &dmrgo.FieldRangeError{Type: "ReverseChannel", Field: "RCCommand", Bits: 4}

// Encode encodes an RC PDU into its 11 information bits: the 4-bit RC command
// and the CRC-7 masked with CRC7MaskRC, packed as DecodeReverseChannel takes
// them, with the low 5 bits of byte 1 0. It returns an error if RCCommand
// does not fit in 4 bits.
func (rc *ReverseChannel) Encode() ([2]byte, error) {
	if rc.RCCommand > 0x0F {
		return [2]byte{}, errReverseChannelRCCommandRange
	}
	cmd := [1]byte{byte(rc.RCCommand) << 4}
	c := crc.CRC7(cmd[:], 4) ^ CRC7MaskRC
	return [2]byte{cmd[0] | c>>3, c << 5}, nil
}

// String returns a human-readable string representation of the RC PDU. It
// allocates the string.
func (rc *ReverseChannel) String() string {
	return fmt.Sprintf("ReverseChannel{ RCCommand: %s, FEC: {BitsChecked: %d, ErrorsCorrected: %d, Uncorrectable: %t} }",
		rc.RCCommand.String(), rc.FEC.BitsChecked, rc.FEC.ErrorsCorrected, rc.FEC.Uncorrectable)
}
