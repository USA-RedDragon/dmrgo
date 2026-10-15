package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
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
	if dh.DataType() != enums.DataTypeDataHeader {
		t.Errorf("DataType() = %d, want DataTypeDataHeader", dh.DataType())
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

	// String should not panic
	s := dh.String()
	if s == "" {
		t.Error("String() should not be empty")
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
			if tt.name == "UDT" && dh.UDTHeader == nil {
				t.Error("UDT format should populate UDTHeader")
			}
			if tt.name == "Proprietary" && dh.ProprietaryHeader == nil {
				t.Error("Proprietary format should populate ProprietaryHeader")
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

	infoBits := must(original.Encode())
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

	infoBits := must(original.Encode())
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

	infoBits := must(original.Encode())
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

	infoBits := must(original.Encode())
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

	infoBits := must(original.Encode())
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

func setBits(bits *[96]bit.Bit, start, length int, val uint32) {
	for i := 0; i < length; i++ {
		bits[start+i] = bit.Bit((val >> (length - 1 - i)) & 1)
	}
}

// Bit positions follow TS 102 361-1 Table 9.17D and TS 102 361-4 Table 7.24.
func TestDataHeader_UDT_Decode(t *testing.T) {
	t.Parallel()
	infoBits := buildDataHeaderBits(func(b *[96]bit.Bit) {
		b[0] = 1                     // G/I
		b[2] = 1                     // Emergency
		b[3] = 1                     // UDT_Option_Flag
		setBits(b, 4, 4, 0b0000)     // DPF = UDT
		setBits(b, 8, 4, 0b0000)     // SAP = UDT
		setBits(b, 12, 4, 0b0101)    // UDT_Format
		setBits(b, 16, 24, 0x123456) // target
		setBits(b, 40, 24, 0xABCDEF) // source
		setBits(b, 64, 5, 0b10011)   // pad nibble
		setBits(b, 70, 2, 0b10)      // UAB = 3 blocks
		b[72] = 1                    // SF
		setBits(b, 74, 6, 0b011010)  // C_UDTHD
	})
	dh, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if dh.Format != pdu.FormatUnifiedDataTransport {
		t.Fatalf("Format = %d, want UDT", dh.Format)
	}
	u := dh.UDTHeader
	if u == nil {
		t.Fatal("UDTHeader should not be nil")
	}
	if !u.Group || u.ResponseRequested || !u.Emergency || !u.OptionFlag {
		t.Errorf("octet 0 flags = G:%t A:%t E:%t Opt:%t, want G/E/Opt set and A clear", u.Group, u.ResponseRequested, u.Emergency, u.OptionFlag)
	}
	if u.SAP != 0 || u.UDTFormat != 0b0101 {
		t.Errorf("SAP=%d UDTFormat=%d, want 0 5", u.SAP, u.UDTFormat)
	}
	if u.LLIDDestination != 0x123456 || u.LLIDSource != 0xABCDEF {
		t.Errorf("addresses = %X/%X, want 123456/ABCDEF", u.LLIDDestination, u.LLIDSource)
	}
	if u.PadNibble != 0b10011 || u.Reserved || u.AppendedBlocks != 0b10 {
		t.Errorf("octet 8 = pad %d rsvd %t UAB %d, want 19 false 2", u.PadNibble, u.Reserved, u.AppendedBlocks)
	}
	if !u.SupplementaryFlag || u.ProtectFlag || u.Opcode != pdu.UDTOpcodeOutboundHeader {
		t.Errorf("octet 9 = SF %t PF %t op %d, want true false C_UDTHD", u.SupplementaryFlag, u.ProtectFlag, u.Opcode)
	}
	if u.Opcode.String() != "C_UDTHD" {
		t.Errorf("Opcode.String() = %q, want C_UDTHD", u.Opcode.String())
	}
	if got := must(dh.Encode()); got != infoBits {
		t.Error("re-encode does not match original bits")
	}
}

func TestDataHeader_UDT_RoundTrip(t *testing.T) {
	t.Parallel()
	original := &pdu.DataHeader{
		Format: pdu.FormatUnifiedDataTransport,
		UDTHeader: &pdu.UDTHeader{
			ResponseRequested: true,
			OptionFlag:        true,
			UDTFormat:         0b0111,
			LLIDDestination:   0xFFFED4,
			LLIDSource:        3120101,
			PadNibble:         7,
			AppendedBlocks:    3,
			ProtectFlag:       true,
			Opcode:            pdu.UDTOpcodeInboundHeader,
		},
	}
	decoded, fecResult := pdu.DecodeDataHeader(must(original.Encode()))
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if decoded.UDTHeader == nil || *decoded.UDTHeader != *original.UDTHeader {
		t.Errorf("round trip = %+v, want %+v", decoded.UDTHeader, original.UDTHeader)
	}
}

// Bit positions follow TS 102 361-1 Table 9.17.
func TestDataHeader_Proprietary_Decode(t *testing.T) {
	t.Parallel()
	infoBits := buildDataHeaderBits(func(b *[96]bit.Bit) {
		setBits(b, 0, 4, 0b1001) // SAP = proprietary packet data
		setBits(b, 4, 4, 0b1111) // DPF = proprietary
		setBits(b, 8, 8, 0x10)   // MFID
		b[16] = 1
		b[79] = 1
	})
	dh, fecResult := pdu.DecodeDataHeader(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	p := dh.ProprietaryHeader
	if p == nil {
		t.Fatal("ProprietaryHeader should not be nil")
	}
	if p.SAP != 0b1001 || p.MFID != 0x10 {
		t.Errorf("SAP=%d MFID=0x%02X, want 9 0x10", p.SAP, p.MFID)
	}
	if p.ManufacturerData[0]&0x80 == 0 || p.ManufacturerData[7]&1 == 0 {
		t.Error("ManufacturerData should span bits 16-79")
	}
	if got := must(dh.Encode()); got != infoBits {
		t.Error("re-encode does not match original bits")
	}
}

func TestDataHeader_Proprietary_RoundTrip(t *testing.T) {
	t.Parallel()
	var mfData [64]bit.Bit
	mfData[3] = 1
	mfData[40] = 1
	original := &pdu.DataHeader{
		Format:            pdu.FormatProprietary,
		ProprietaryHeader: &pdu.ProprietaryHeader{SAP: 9, MFID: 0x68, ManufacturerData: [8]byte(bit.Pack(mfData[:]))},
	}
	decoded, fecResult := pdu.DecodeDataHeader(must(original.Encode()))
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if decoded.ProprietaryHeader == nil || *decoded.ProprietaryHeader != *original.ProprietaryHeader {
		t.Errorf("round trip = %+v, want %+v", decoded.ProprietaryHeader, original.ProprietaryHeader)
	}
}

func TestDataHeader_ReservedFormat_RoundTrip(t *testing.T) {
	in := buildDataHeaderBits(func(bits *[96]bit.Bit) {
		for i := range 80 {
			bits[i] = bit.Bit((i*5 + 1) / 3 % 2)
		}
		bit.PutUint(bits[4:8], uint64(0b0100))
	})
	dh, fecResult := pdu.DecodeDataHeader(in)
	if fecResult.Uncorrectable {
		t.Fatal("CRC failed")
	}
	if dh.Format != 0b0100 {
		t.Fatalf("Format = %04b, want 0100", dh.Format)
	}
	if got := must(dh.Encode()); got != in {
		t.Errorf("Encode(Decode(x)) = %v, want %v", got, in)
	}
}

func TestDataHeaderSubEncodersSetFormat(t *testing.T) {
	encode := map[pdu.Format]func() ([80]bit.Bit, error){
		pdu.FormatUnconfirmed: func() ([80]bit.Bit, error) {
			return (&pdu.UnconfirmedDataHeader{}).Encode()
		},
		pdu.FormatConfirmed: func() ([80]bit.Bit, error) {
			return (&pdu.ConfirmedDataHeader{}).Encode()
		},
		pdu.FormatResponsePacket: func() ([80]bit.Bit, error) {
			return (&pdu.ResponsePacketHeader{}).Encode()
		},
		pdu.FormatShortDataDefined: func() ([80]bit.Bit, error) {
			return (&pdu.DefinedDataHeader{}).Encode()
		},
		pdu.FormatShortDataRawOrStatusPrecoded: func() ([80]bit.Bit, error) {
			return (&pdu.RawDataHeader{AppendedBlocks: 1}).Encode()
		},
		pdu.FormatUnifiedDataTransport: func() ([80]bit.Bit, error) {
			return (&pdu.UDTHeader{}).Encode()
		},
		pdu.FormatProprietary: func() ([80]bit.Bit, error) {
			return (&pdu.ProprietaryHeader{}).Encode()
		},
	}
	for format, enc := range encode {
		bits, err := enc()
		if err != nil {
			t.Fatalf("%s: %v", format.String(), err)
		}
		if got := pdu.Format(uint8(bit.Uint(bits[4 : 4+4]))); got != format {
			t.Errorf("%s: Format bits = %d, want %d", format.String(), got, format)
		}
	}
}

func TestStatusPrecodedEncoderClearsAppendedBlocks(t *testing.T) {
	bits := must((&pdu.StatusPrecodedHeader{Group: true, SAP: 0xF}).Encode())
	if got := pdu.Format(uint8(bit.Uint(bits[4 : 4+4]))); got != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format bits = %d, want %d", got, pdu.FormatShortDataRawOrStatusPrecoded)
	}
	if ab := uint8(bit.Uint(bits[2:2+2]))<<4 | uint8(bit.Uint(bits[12:12+4])); ab != 0 {
		t.Errorf("Appended Blocks = %d, want 0 (ETSI TS 102 361-1 Table 9.17A)", ab)
	}
}
