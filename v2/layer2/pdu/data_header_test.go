package pdu_test

import (
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestDataHeader_UnconfirmedDecode(t *testing.T) {
	t.Parallel()

	var infoBits [12]byte
	bits.SetBit(infoBits[:], 6, 1)
	bits.SetBit(infoBits[:], 0, 1)
	bits.SetBit(infoBits[:], 14, 1)
	bits.SetBit(infoBits[:], 15, 1)
	bits.SetBit(infoBits[:], 64, 1)
	bits.SetBit(infoBits[:], 65+3, 1)
	bits.SetBit(infoBits[:], 65+5, 1)

	crcVal := crc.CCITT(infoBits[:10]) ^ 0xCCCC
	infoBits[10] = byte(crcVal >> 8)
	infoBits[11] = byte(crcVal)

	dh := pdu.DecodeDataHeader(infoBits)
	fecResult := dh.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if dh.DataType() != enums.DataTypeDataHeader {
		t.Errorf("DataType() = %d, want DataTypeDataHeader", dh.DataType())
	}
	u, ok := dh.AsUnconfirmedDataHeader()
	if !ok {
		t.Fatal("AsUnconfirmedDataHeader should be ok")
	}
	if !u.Group {
		t.Error("Group should be true")
	}
	if !u.FullMessageFlag {
		t.Error("FullMessageFlag should be true")
	}
	if u.BlocksToFollow != 10 {
		t.Errorf("BlocksToFollow = %d, want 10", u.BlocksToFollow)
	}

	s := dh.String()
	if s == "" {
		t.Error("String() should not be empty")
	}
}

func TestDataHeader_CRCFailure(t *testing.T) {
	t.Parallel()

	var infoBits [12]byte
	fecResult := pdu.DecodeDataHeader(infoBits).FEC
	if !fecResult.Uncorrectable {
		t.Error("DecodeDataHeader should return uncorrectable for invalid CRC")
	}
}

func TestDataHeader_FormatDispatch(t *testing.T) {
	t.Parallel()

	formats := []struct {
		name   string
		format uint64
	}{
		{"UDT", 0b0000},
		{"Proprietary", 0b1111},
	}

	for _, tt := range formats {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var infoBits [12]byte
			bits.SetField(infoBits[:], 4, 4, tt.format)

			crcVal := crc.CCITT(infoBits[:10]) ^ 0xCCCC
			infoBits[10] = byte(crcVal >> 8)
			infoBits[11] = byte(crcVal)

			dh := pdu.DecodeDataHeader(infoBits)
			fecResult := dh.FEC
			if fecResult.Uncorrectable {
				t.Fatalf("DecodeDataHeader returned uncorrectable FEC for format %s", tt.name)
			}
			if _, ok := dh.AsUnconfirmedDataHeader(); ok {
				t.Errorf("format %s should not be a U_HEAD", tt.name)
			}
			if _, ok := dh.AsUDTHeader(); tt.name == "UDT" && !ok {
				t.Error("UDT format should be a UDT_HEAD")
			}
			if _, ok := dh.AsProprietaryHeader(); tt.name == "Proprietary" && !ok {
				t.Error("Proprietary format should be a P_HEAD")
			}
		})
	}
}

func buildDataHeader(setup func(b *[12]byte)) [12]byte {
	var infoBits [12]byte
	setup(&infoBits)

	crcVal := crc.CCITT(infoBits[:10]) ^ 0xCCCC
	infoBits[10] = byte(crcVal >> 8)
	infoBits[11] = byte(crcVal)
	return infoBits
}

func TestDataHeader_Confirmed_RoundTrip(t *testing.T) {
	t.Parallel()
	var original pdu.DataHeader
	if err := original.SetConfirmedDataHeader(pdu.ConfirmedDataHeader{
		Group:                  true,
		ResponseRequested:      false,
		PadOctetCount:          17,
		SAP:                    4,
		LLIDDestination:        9990,
		LLIDSource:             3120101,
		FullMessageFlag:        true,
		BlocksToFollow:         10,
		ReSynchronizeFlag:      true,
		SendSequenceNumber:     3,
		FragmentSequenceNumber: 7,
	}); err != nil {
		t.Fatal(err)
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeDataHeader(infoBits)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format() != pdu.FormatConfirmed {
		t.Errorf("Format = %d, want FormatConfirmed", decoded.Format())
	}
	cdh, ok := decoded.AsConfirmedDataHeader()
	if !ok {
		t.Fatal("AsConfirmedDataHeader is not ok")
	}
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
	var original pdu.DataHeader
	if err := original.SetResponsePacketHeader(pdu.ResponsePacketHeader{
		SAP:             4,
		LLIDDestination: 100,
		LLIDSource:      200,
		BlocksToFollow:  5,
		ResponseClass:   1,
		ResponseType:    3,
		ResponseStatus:  2,
	}); err != nil {
		t.Fatal(err)
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeDataHeader(infoBits)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format() != pdu.FormatResponsePacket {
		t.Errorf("Format = %d, want FormatResponsePacket", decoded.Format())
	}
	rph, ok := decoded.AsResponsePacketHeader()
	if !ok {
		t.Fatal("AsResponsePacketHeader is not ok")
	}
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
	var original pdu.DataHeader
	if err := original.SetDefinedDataHeader(pdu.DefinedDataHeader{
		Group:             true,
		ResponseRequested: false,
		AppendedBlocks:    3,
		SAP:               10,
		LLIDDestination:   5000,
		LLIDSource:        6000,
		DefinedData:       15,
		SARQ:              true,
		FullMessageFlag:   true,
		BitPadding:        0,
	}); err != nil {
		t.Fatal(err)
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeDataHeader(infoBits)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format() != pdu.FormatShortDataDefined {
		t.Errorf("Format = %d, want FormatShortDataDefined", decoded.Format())
	}
	ddh, ok := decoded.AsDefinedDataHeader()
	if !ok {
		t.Fatal("AsDefinedDataHeader is not ok")
	}
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
	var original pdu.DataHeader
	if err := original.SetRawDataHeader(pdu.RawDataHeader{
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
	}); err != nil {
		t.Fatal(err)
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeDataHeader(infoBits)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format() != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format = %d, want FormatShortDataRawOrStatusPrecoded", decoded.Format())
	}
	rdh, ok := decoded.AsRawDataHeader()
	if !ok {
		t.Fatal("AsRawDataHeader is not ok")
	}
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
	if _, ok := decoded.AsStatusPrecodedHeader(); ok {
		t.Error("AsStatusPrecodedHeader should not be ok for non-zero AppendedBlocks")
	}
}

func TestDataHeader_StatusPrecoded_RoundTrip(t *testing.T) {
	t.Parallel()
	var original pdu.DataHeader
	if err := original.SetStatusPrecodedHeader(pdu.StatusPrecodedHeader{
		Group:             false,
		ResponseRequested: true,
		SAP:               10,
		LLIDDestination:   3000,
		LLIDSource:        4000,
		SourcePort:        1,
		DestinationPort:   2,
		StatusPrecoded:    512,
	}); err != nil {
		t.Fatal(err)
	}

	infoBits := must(original.Encode())
	decoded := pdu.DecodeDataHeader(infoBits)
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeDataHeader returned uncorrectable FEC")
	}
	if decoded.Format() != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format = %d, want FormatShortDataRawOrStatusPrecoded", decoded.Format())
	}
	sph, ok := decoded.AsStatusPrecodedHeader()
	if !ok {
		t.Fatal("AsStatusPrecodedHeader is not ok")
	}
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
	if _, ok := decoded.AsRawDataHeader(); ok {
		t.Error("AsRawDataHeader should not be ok for zero AppendedBlocks")
	}
}

// TS 102 361-1 Table 9.17D; TS 102 361-4 Table 7.24
func TestDataHeader_UDT_Decode(t *testing.T) {
	t.Parallel()
	infoBits := buildDataHeader(func(b *[12]byte) {
		bits.SetBit(b[:], 0, 1)
		bits.SetBit(b[:], 2, 1)
		bits.SetBit(b[:], 3, 1)
		bits.SetField(b[:], 4, 4, 0b0000)
		bits.SetField(b[:], 8, 4, 0b0000)
		bits.SetField(b[:], 12, 4, 0b0101)
		bits.SetField(b[:], 16, 24, 0x123456)
		bits.SetField(b[:], 40, 24, 0xABCDEF)
		bits.SetField(b[:], 64, 5, 0b10011)
		bits.SetField(b[:], 70, 2, 0b10)
		bits.SetBit(b[:], 72, 1)
		bits.SetField(b[:], 74, 6, 0b011010)
	})
	dh := pdu.DecodeDataHeader(infoBits)
	fecResult := dh.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if dh.Format() != pdu.FormatUnifiedDataTransport {
		t.Fatalf("Format = %d, want UDT", dh.Format())
	}
	u, ok := dh.AsUDTHeader()
	if !ok {
		t.Fatal("AsUDTHeader should be ok")
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
	var original pdu.DataHeader
	if err := original.SetUDTHeader(pdu.UDTHeader{
		ResponseRequested: true,
		OptionFlag:        true,
		UDTFormat:         0b0111,
		LLIDDestination:   0xFFFED4,
		LLIDSource:        3120101,
		PadNibble:         7,
		AppendedBlocks:    3,
		ProtectFlag:       true,
		Opcode:            pdu.UDTOpcodeInboundHeader,
	}); err != nil {
		t.Fatal(err)
	}
	decoded := pdu.DecodeDataHeader(must(original.Encode()))
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	want, _ := original.AsUDTHeader()
	if got, ok := decoded.AsUDTHeader(); !ok || got != want {
		t.Errorf("round trip = %+v (%t), want %+v", got, ok, want)
	}
}

// TS 102 361-1 Table 9.17
func TestDataHeader_Proprietary_Decode(t *testing.T) {
	t.Parallel()
	infoBits := buildDataHeader(func(b *[12]byte) {
		bits.SetField(b[:], 0, 4, 0b1001)
		bits.SetField(b[:], 4, 4, 0b1111)
		bits.SetField(b[:], 8, 8, 0x10)
		bits.SetBit(b[:], 16, 1)
		bits.SetBit(b[:], 79, 1)
	})
	dh := pdu.DecodeDataHeader(infoBits)
	fecResult := dh.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	p, ok := dh.AsProprietaryHeader()
	if !ok {
		t.Fatal("AsProprietaryHeader should be ok")
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
	var mfData [8]byte
	bits.SetBit(mfData[:], 3, 1)
	bits.SetBit(mfData[:], 40, 1)
	want := pdu.ProprietaryHeader{SAP: 9, MFID: 0x68, ManufacturerData: mfData}
	var original pdu.DataHeader
	if err := original.SetProprietaryHeader(want); err != nil {
		t.Fatal(err)
	}
	decoded := pdu.DecodeDataHeader(must(original.Encode()))
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("unexpected CRC failure")
	}
	if got, ok := decoded.AsProprietaryHeader(); !ok || got != want {
		t.Errorf("round trip = %+v (%t), want %+v", got, ok, want)
	}
}

func TestDataHeader_ReservedFormat_RoundTrip(t *testing.T) {
	t.Parallel()

	in := buildDataHeader(func(b *[12]byte) {
		for i := range 80 {
			bits.SetBit(b[:], i, byte((i*5+1)/3%2))
		}
		bits.SetField(b[:], 4, 4, 0b0100)
	})
	dh := pdu.DecodeDataHeader(in)
	fecResult := dh.FEC
	if fecResult.Uncorrectable {
		t.Fatal("CRC failed")
	}
	if dh.Format() != 0b0100 {
		t.Fatalf("Format = %04b, want 0100", dh.Format())
	}
	if got := must(dh.Encode()); got != in {
		t.Errorf("Encode(Decode(x)) = %v, want %v", got, in)
	}
}

func TestDataHeaderSubEncodersSetFormat(t *testing.T) {
	t.Parallel()

	encode := map[pdu.Format]func() ([10]byte, error){
		pdu.FormatUnconfirmed: func() ([10]byte, error) {
			return (&pdu.UnconfirmedDataHeader{}).Encode()
		},
		pdu.FormatConfirmed: func() ([10]byte, error) {
			return (&pdu.ConfirmedDataHeader{}).Encode()
		},
		pdu.FormatResponsePacket: func() ([10]byte, error) {
			return (&pdu.ResponsePacketHeader{}).Encode()
		},
		pdu.FormatShortDataDefined: func() ([10]byte, error) {
			return (&pdu.DefinedDataHeader{}).Encode()
		},
		pdu.FormatShortDataRawOrStatusPrecoded: func() ([10]byte, error) {
			return (&pdu.RawDataHeader{AppendedBlocks: 1}).Encode()
		},
		pdu.FormatUnifiedDataTransport: func() ([10]byte, error) {
			return (&pdu.UDTHeader{}).Encode()
		},
		pdu.FormatProprietary: func() ([10]byte, error) {
			return (&pdu.ProprietaryHeader{}).Encode()
		},
	}
	for format, enc := range encode {
		hdr, err := enc()
		if err != nil {
			t.Fatalf("%s: %v", format.String(), err)
		}
		if got := pdu.Format(uint8(bits.Field(hdr[:], 4, 4))); got != format {
			t.Errorf("%s: Format bits = %d, want %d", format.String(), got, format)
		}
	}
}

func TestStatusPrecodedEncoderClearsAppendedBlocks(t *testing.T) {
	t.Parallel()

	hdr := must((&pdu.StatusPrecodedHeader{Group: true, SAP: 0xF}).Encode())
	if got := pdu.Format(uint8(bits.Field(hdr[:], 4, 4))); got != pdu.FormatShortDataRawOrStatusPrecoded {
		t.Errorf("Format bits = %d, want %d", got, pdu.FormatShortDataRawOrStatusPrecoded)
	}
	if ab := uint8(bits.Field(hdr[:], 2, 2))<<4 | uint8(bits.Field(hdr[:], 12, 4)); ab != 0 {
		t.Errorf("Appended Blocks = %d, want 0 (ETSI TS 102 361-1 Table 9.17A)", ab)
	}
}

func TestDataHeader_AppendedBlocksOnlyForShortData(t *testing.T) {
	t.Parallel()

	uhead := [12]byte{0x02, 0x3b, 0x2f, 0xeb, 0xf6, 0x30, 0xb4, 0x3c, 0x88, 0x00, 0xe5, 0x00}
	dh := pdu.DecodeDataHeader(uhead)
	u, ok := dh.AsUnconfirmedDataHeader()
	if dh.FEC.Uncorrectable || !ok {
		t.Fatalf("U_HEAD did not decode: %s", dh.String())
	}
	if u.PadOctetCount != 11 || u.BlocksToFollow != 8 {
		t.Fatalf("POC %d BF %d, want 11 and 8", u.PadOctetCount, u.BlocksToFollow)
	}
	if dh.AppendedBlocks() != 0 {
		t.Errorf("U_HEAD AppendedBlocks = %d, want 0", dh.AppendedBlocks())
	}
	if s := dh.String(); strings.Contains(s, "AppendedBlocks: 11") {
		t.Errorf("String shows the POC as AppendedBlocks: %s", s)
	}
	for _, f := range []pdu.Format{pdu.FormatShortDataDefined, pdu.FormatShortDataRawOrStatusPrecoded} {
		h := uhead
		h[0] = h[0]&^0x3F | 0x20 | byte(f)
		h[1] = h[1]&^0x0F | 0x05
		dh := pdu.DecodeDataHeader(h)
		if got := dh.AppendedBlocks(); got != 0x25 {
			t.Errorf("format %d: AppendedBlocks = %d, want %d", f, got, 0x25)
		}
	}
}
