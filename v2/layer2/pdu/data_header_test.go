package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
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
	crcVal := crc.CalculateCRCCCITT(dataBytes[:])
	// Apply Data Header CRC mask per ETSI TS 102 361-1 Table B.3
	crcHigh := byte(crcVal>>8) ^ 0xCC
	crcLow := byte(crcVal) ^ 0xCC
	for b := 7; b >= 0; b-- {
		infoBits[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
		infoBits[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
	}

	dh, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	dh.DataType = elements.DataTypeDataHeader
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

func TestDataHeader_CRCFailure(t *testing.T) {
	// All-zero bits will fail CRC check
	var infoBits [96]bit.Bit
	_, fecResult := pdu.DecodeDataHeader(infoBits)
	if !fecResult.Uncorrectable {
		t.Error("DecodeDataHeader should return uncorrectable for invalid CRC")
	}
}

func TestDataHeader_FormatDispatch(t *testing.T) {
	// Test that non-Unconfirmed formats decode without panic but have nil sub-PDUs
	formats := []struct {
		name string
		bits [4]bit.Bit // bits at positions 4-7
	}{
		{"UDT", [4]bit.Bit{0, 0, 0, 0}},
		{"Proprietary", [4]bit.Bit{1, 1, 1, 1}},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			var infoBits [96]bit.Bit
			infoBits[4] = tt.bits[0]
			infoBits[5] = tt.bits[1]
			infoBits[6] = tt.bits[2]
			infoBits[7] = tt.bits[3]

			// Add valid CRC so the decode proceeds past CRC check
			var dataBytes [10]byte
			for i := range 10 {
				for j := range 8 {
					dataBytes[i] <<= 1
					dataBytes[i] |= byte(infoBits[i*8+j])
				}
			}
			crcVal := crc.CalculateCRCCCITT(dataBytes[:])
			// Apply Data Header CRC mask per ETSI TS 102 361-1 Table B.3
			crcHigh := byte(crcVal>>8) ^ 0xCC
			crcLow := byte(crcVal) ^ 0xCC
			for b := 7; b >= 0; b-- {
				infoBits[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
				infoBits[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
			}

			dh, fecResult := pdu.DecodeDataHeader(infoBits)
			if fecResult.Uncorrectable {
				t.Fatalf("DecodeDataHeader returned uncorrectable FEC for format %s", tt.name)
			}
			// Non-Unconfirmed formats should have nil UnconfirmedDataHeader
			if dh.UnconfirmedDataHeader != nil {
				t.Errorf("format %s should have nil UnconfirmedDataHeader", tt.name)
			}
		})
	}
}

// buildDataHeaderBits creates a 96-bit DataHeader with valid CRC for testing.
// The provided setup function may modify bits 0-79; this helper computes the CRC.
func buildDataHeaderBits(setup func(bits *[96]bit.Bit)) [96]bit.Bit {
	var infoBits [96]bit.Bit
	setup(&infoBits)

	// Pack first 10 bytes, compute CRC, apply mask, and place in bits 80-95
	var dataBytes [10]byte
	for i := range 10 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(infoBits[i*8+j])
		}
	}
	crcVal := crc.CalculateCRCCCITT(dataBytes[:])
	crcHigh := byte(crcVal>>8) ^ 0xCC
	crcLow := byte(crcVal) ^ 0xCC
	for b := 7; b >= 0; b-- {
		infoBits[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
		infoBits[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
	}
	return infoBits
}

func TestDataHeader_Confirmed_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format: pdu.FormatConfirmed,
		ConfirmedDataHeader: &pdu.ConfirmedDataHeader{
			Group:                  true,
			ResponseRequested:      false,
			PadOctetCount:          17, // 5-bit value (0-31)
			SAP:                    4,
			LLIDDestination:        9990,
			LLIDSource:             3120101,
			FullMessageFlag:        true,
			BlocksToFollow:         10,
			ReSynchronizeFlag:      true,
			SendSequenceNumber:     3,
			FragmentSequenceNumber: 7,
		},
	}

	infoBits := pdu.EncodeDataHeader(original)
	decoded, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format != pdu.FormatConfirmed {
		t.Errorf("Format = %d, want FormatConfirmed", decoded.Format)
	}
	if decoded.ConfirmedDataHeader == nil {
		t.Fatal("ConfirmedDataHeader is nil")
	}
	cdh := decoded.ConfirmedDataHeader
	if !cdh.Group {
		t.Error("Group should be true")
	}
	if cdh.PadOctetCount != 17 {
		t.Errorf("PadOctetCount = %d, want 17", cdh.PadOctetCount)
	}
	if cdh.SAP != 4 {
		t.Errorf("SAP = %d, want 4", cdh.SAP)
	}
	if cdh.LLIDDestination != 9990 {
		t.Errorf("LLIDDestination = %d, want 9990", cdh.LLIDDestination)
	}
	if cdh.LLIDSource != 3120101 {
		t.Errorf("LLIDSource = %d, want 3120101", cdh.LLIDSource)
	}
	if !cdh.FullMessageFlag {
		t.Error("FullMessageFlag should be true")
	}
	if cdh.BlocksToFollow != 10 {
		t.Errorf("BlocksToFollow = %d, want 10", cdh.BlocksToFollow)
	}
	if !cdh.ReSynchronizeFlag {
		t.Error("ReSynchronizeFlag should be true")
	}
	if cdh.SendSequenceNumber != 3 {
		t.Errorf("SendSequenceNumber = %d, want 3", cdh.SendSequenceNumber)
	}
	if cdh.FragmentSequenceNumber != 7 {
		t.Errorf("FragmentSequenceNumber = %d, want 7", cdh.FragmentSequenceNumber)
	}
}

func TestDataHeader_ResponsePacket_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format: pdu.FormatResponsePacket,
		ResponsePacketHeader: &pdu.ResponsePacketHeader{
			SAP:             4,
			LLIDDestination: 100,
			LLIDSource:      200,
			BlocksToFollow:  5,
			ResponseClass:   1,
			ResponseType:    3,
			ResponseStatus:  2,
		},
	}

	infoBits := pdu.EncodeDataHeader(original)
	decoded, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format != pdu.FormatResponsePacket {
		t.Errorf("Format = %d, want FormatResponsePacket", decoded.Format)
	}
	if decoded.ResponsePacketHeader == nil {
		t.Fatal("ResponsePacketHeader is nil")
	}
	rph := decoded.ResponsePacketHeader
	if rph.SAP != 4 {
		t.Errorf("SAP = %d, want 4", rph.SAP)
	}
	if rph.LLIDDestination != 100 {
		t.Errorf("LLIDDestination = %d, want 100", rph.LLIDDestination)
	}
	if rph.LLIDSource != 200 {
		t.Errorf("LLIDSource = %d, want 200", rph.LLIDSource)
	}
	if rph.BlocksToFollow != 5 {
		t.Errorf("BlocksToFollow = %d, want 5", rph.BlocksToFollow)
	}
	if rph.ResponseClass != 1 {
		t.Errorf("ResponseClass = %d, want 1", rph.ResponseClass)
	}
	if rph.ResponseType != 3 {
		t.Errorf("ResponseType = %d, want 3", rph.ResponseType)
	}
	if rph.ResponseStatus != 2 {
		t.Errorf("ResponseStatus = %d, want 2", rph.ResponseStatus)
	}
}

func TestDataHeader_DefinedData_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format:         pdu.FormatShortDataDefined,
		AppendedBlocks: 3,
		DefinedDataHeader: &pdu.DefinedDataHeader{
			Group:             true,
			ResponseRequested: false,
			AppendedBlocks:    3,
			SAP:               10, // Short Data
			LLIDDestination:   5000,
			LLIDSource:        6000,
			DefinedData:       15,
			SARQ:              true,
			FullMessageFlag:   true,
			BitPadding:        0,
		},
	}

	infoBits := pdu.EncodeDataHeader(original)
	decoded, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format != pdu.FormatShortDataDefined {
		t.Errorf("Format = %d, want FormatShortDataDefined", decoded.Format)
	}
	if decoded.DefinedDataHeader == nil {
		t.Fatal("DefinedDataHeader is nil")
	}
	ddh := decoded.DefinedDataHeader
	if !ddh.Group {
		t.Error("Group should be true")
	}
	if ddh.AppendedBlocks != 3 {
		t.Errorf("AppendedBlocks = %d, want 3", ddh.AppendedBlocks)
	}
	if ddh.SAP != 10 {
		t.Errorf("SAP = %d, want 10", ddh.SAP)
	}
	if ddh.LLIDDestination != 5000 {
		t.Errorf("LLIDDestination = %d, want 5000", ddh.LLIDDestination)
	}
	if ddh.LLIDSource != 6000 {
		t.Errorf("LLIDSource = %d, want 6000", ddh.LLIDSource)
	}
	if ddh.DefinedData != 15 {
		t.Errorf("DefinedData = %d, want 15", ddh.DefinedData)
	}
	if !ddh.SARQ {
		t.Error("SARQ should be true")
	}
	if !ddh.FullMessageFlag {
		t.Error("FullMessageFlag should be true")
	}
}

func TestDataHeader_RawData_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format:         pdu.FormatShortDataRawOrStatusPrecoded,
		AppendedBlocks: 2, // Non-zero → dispatches to RawDataHeader
		RawDataHeader: &pdu.RawDataHeader{
			Group:             true,
			ResponseRequested: true,
			AppendedBlocks:    2,
			SAP:               10,
			LLIDDestination:   1000,
			LLIDSource:        2000,
			SourcePort:        3,
			DestinationPort:   5,
			SARQ:              false,
			FullMessageFlag:   true,
			BitPadding:        0,
		},
	}

	infoBits := pdu.EncodeDataHeader(original)
	decoded, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format = %d, want FormatShortDataRawOrStatusPrecoded", decoded.Format)
	}
	if decoded.RawDataHeader == nil {
		t.Fatal("RawDataHeader is nil")
	}
	rdh := decoded.RawDataHeader
	if !rdh.Group {
		t.Error("Group should be true")
	}
	if !rdh.ResponseRequested {
		t.Error("ResponseRequested should be true")
	}
	if rdh.AppendedBlocks != 2 {
		t.Errorf("AppendedBlocks = %d, want 2", rdh.AppendedBlocks)
	}
	if rdh.SourcePort != 3 {
		t.Errorf("SourcePort = %d, want 3", rdh.SourcePort)
	}
	if rdh.DestinationPort != 5 {
		t.Errorf("DestinationPort = %d, want 5", rdh.DestinationPort)
	}
	if !rdh.FullMessageFlag {
		t.Error("FullMessageFlag should be true")
	}
	if decoded.StatusPrecodedHeader != nil {
		t.Error("StatusPrecodedHeader should be nil for non-zero AppendedBlocks")
	}
}

func TestDataHeader_StatusPrecoded_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format:         pdu.FormatShortDataRawOrStatusPrecoded,
		AppendedBlocks: 0, // Zero → dispatches to StatusPrecodedHeader
		StatusPrecodedHeader: &pdu.StatusPrecodedHeader{
			Group:             false,
			ResponseRequested: true,
			SAP:               10,
			LLIDDestination:   3000,
			LLIDSource:        4000,
			SourcePort:        1,
			DestinationPort:   2,
			StatusPrecoded:    512, // 10-bit value
		},
	}

	infoBits := pdu.EncodeDataHeader(original)
	decoded, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format = %d, want FormatShortDataRawOrStatusPrecoded", decoded.Format)
	}
	if decoded.StatusPrecodedHeader == nil {
		t.Fatal("StatusPrecodedHeader is nil")
	}
	sph := decoded.StatusPrecodedHeader
	if sph.ResponseRequested != true {
		t.Error("ResponseRequested should be true")
	}
	if sph.LLIDDestination != 3000 {
		t.Errorf("LLIDDestination = %d, want 3000", sph.LLIDDestination)
	}
	if sph.LLIDSource != 4000 {
		t.Errorf("LLIDSource = %d, want 4000", sph.LLIDSource)
	}
	if sph.SourcePort != 1 {
		t.Errorf("SourcePort = %d, want 1", sph.SourcePort)
	}
	if sph.DestinationPort != 2 {
		t.Errorf("DestinationPort = %d, want 2", sph.DestinationPort)
	}
	if sph.StatusPrecoded != 512 {
		t.Errorf("StatusPrecoded = %d, want 512", sph.StatusPrecoded)
	}
	if decoded.RawDataHeader != nil {
		t.Error("RawDataHeader should be nil for zero AppendedBlocks")
	}
}
