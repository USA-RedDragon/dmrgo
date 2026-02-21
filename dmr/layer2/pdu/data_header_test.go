package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/dmr/layer2/pdu"
)

func TestDataHeader_UnconfirmedDecode(t *testing.T) {
	// Build 96 info bits for an Unconfirmed data header
	var infoBits [96]bit.Bit

	// Format = 0b0010 (Unconfirmed) at bits 4-7
	// Format decoded as: (bit4<<3)|(bit5<<2)|(bit6<<1)|bit7 = 2
	infoBits[6] = 1 // bit 6 = 1 → 0b0010 = FormatUnconfirmed

	// Group = 1 (bit 0)
	infoBits[0] = 1
	// ResponseRequested = 0 (bit 1)
	// Reserved = 0 (bit 2)
	// PadOctetCount MSB = 0 (bit 3), LSBs at bits 12-15 = 3
	infoBits[14] = 1
	infoBits[15] = 1

	// FullMessage = 1 (bit 64)
	infoBits[64] = 1
	// BlocksToFollow = 10 (bits 65-71)
	infoBits[65+3] = 1 // 8
	infoBits[65+5] = 1 // 2 → total 10

	// FragmentSequenceNumber = 0 (bits 76-79)

	// Compute CRC-CCITT over the first 10 bytes and place it in bits 80-95
	var dataBytes [10]byte
	for i := range 10 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(infoBits[i*8+j])
		}
	}
	crc := pdu.CalculateCRCCCITT(dataBytes[:])
	// CRC stored big-endian with MMDVM byte-swap: crc8[0] → last byte, crc8[1] → second-to-last
	crcHigh := byte(crc >> 8)
	crcLow := byte(crc)
	// MMDVM check: byte(crc) == data[11] && byte(crc>>8) == data[10]
	// So data[10] = crcHigh, data[11] = crcLow
	for b := 7; b >= 0; b-- {
		infoBits[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
		infoBits[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
	}

	var dh pdu.DataHeader
	ok := dh.DecodeFromBits(infoBits[:], elements.DataTypeDataHeader)
	if !ok {
		t.Fatal("DecodeFromBits failed for Unconfirmed DataHeader")
	}
	if dh.GetDataType() != elements.DataTypeDataHeader {
		t.Errorf("GetDataType() = %d, want DataTypeDataHeader", dh.GetDataType())
	}
	if dh.UnconfirmedDataHeader == nil {
		t.Fatal("UnconfirmedDataHeader should not be nil")
	}
	if !dh.UnconfirmedDataHeader.Group {
		t.Error("Group should be true")
	}
	if !dh.UnconfirmedDataHeader.FullMessage {
		t.Error("FullMessage should be true")
	}
	if dh.UnconfirmedDataHeader.BlocksToFollow != 10 {
		t.Errorf("BlocksToFollow = %d, want 10", dh.UnconfirmedDataHeader.BlocksToFollow)
	}

	// ToString should not panic
	s := dh.ToString()
	if s == "" {
		t.Error("ToString() should not be empty")
	}
}

func TestDataHeader_InvalidLength(t *testing.T) {
	var dh pdu.DataHeader
	ok := dh.DecodeFromBits(make([]bit.Bit, 10), elements.DataTypeDataHeader)
	if ok {
		t.Error("DecodeFromBits should return false for invalid length")
	}
}

func TestDataHeader_FormatDispatch(t *testing.T) {
	// Test that non-Unconfirmed formats return false (not yet implemented)
	formats := []struct {
		name string
		bits [4]bit.Bit // bits at positions 4-7
	}{
		{"UDT", [4]bit.Bit{0, 0, 0, 0}},
		{"Response", [4]bit.Bit{0, 0, 0, 1}},
		{"Confirmed", [4]bit.Bit{0, 0, 1, 1}},
		{"ShortDataDefined", [4]bit.Bit{1, 1, 0, 1}},
		{"Proprietary", [4]bit.Bit{1, 1, 1, 1}},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			var infoBits [96]bit.Bit
			infoBits[4] = tt.bits[0]
			infoBits[5] = tt.bits[1]
			infoBits[6] = tt.bits[2]
			infoBits[7] = tt.bits[3]

			var dh pdu.DataHeader
			ok := dh.DecodeFromBits(infoBits[:], elements.DataTypeDataHeader)
			if ok {
				t.Errorf("format %s should return false (unimplemented)", tt.name)
			}
		})
	}
}
