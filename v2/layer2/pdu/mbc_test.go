package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildMBCHeaderBits constructs a [96]bit.Bit array for an MBC Header PDU.
// lb is LastBlock, pf is ProtectFlag, opcode is 6-bit CSBK opcode, fid is 8-bit FID.
// CRC-CCITT is computed and XOR-masked per §B.3.12 (mask 0xAAAA).
func buildMBCHeaderBits(lb bool, pf bool, opcode byte, fid byte, data [64]bit.Bit) [96]bit.Bit {
	var bits [96]bit.Bit

	// Bit 0: LastBlock
	if lb {
		bits[0] = 1
	}
	// Bit 1: ProtectFlag
	if pf {
		bits[1] = 1
	}
	// Bits 2-7: CSBKOpcode
	bit.PutUint(bits[2:8], uint64(opcode))
	// Bits 8-15: FID
	bit.PutUint(bits[8:16], uint64(fid))
	// Bits 16-79: Data
	copy(bits[16:80], data[:])

	// Pack first 10 bytes to compute CRC
	var dataBytes [10]byte
	for i := range 10 {
		for j := range 8 {
			dataBytes[i] <<= 1
			dataBytes[i] |= byte(bits[i*8+j])
		}
	}

	crcVal := crc.CCITT(dataBytes[:])
	crcHigh := byte(crcVal>>8) ^ 0xAA
	crcLow := byte(crcVal) ^ 0xAA
	for b := 7; b >= 0; b-- {
		bits[80+(7-b)] = bit.Bit((crcHigh >> b) & 1)
		bits[88+(7-b)] = bit.Bit((crcLow >> b) & 1)
	}

	return bits
}

// buildMBCContinuationBits constructs a [96]bit.Bit array for an MBC Continuation PDU.
// No CRC is applied at the single-burst level.
func buildMBCContinuationBits(lb bool, data [95]bit.Bit) [96]bit.Bit {
	var bits [96]bit.Bit
	if lb {
		bits[0] = 1
	}
	copy(bits[1:96], data[:])
	return bits
}

func TestMBCHeader_Decode(t *testing.T) {
	var data [64]bit.Bit
	data[0] = 1
	data[10] = 1
	data[63] = 1

	infoBits := buildMBCHeaderBits(false, true, 0b00111101, 0x00, data)

	hdr := pdu.DecodeMBCHeader(infoBits)
	fecResult := hdr.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeMBCHeader returned uncorrectable FEC")
	}

	if hdr.LastBlock {
		t.Error("LastBlock should be false")
	}
	if !hdr.ProtectFlag {
		t.Error("ProtectFlag should be true")
	}
	if hdr.Opcode != pdu.CSBKPreamble {
		t.Errorf("CSBKOpcode = %08b, want %08b (PreamblePDU)", byte(hdr.Opcode), byte(pdu.CSBKPreamble))
	}
	if hdr.FID != 0x00 {
		t.Errorf("FID = %d, want 0", hdr.FID)
	}

	// Verify data bits
	dataBits := bit.Unpack(hdr.Data[:])
	if dataBits[0] != 1 {
		t.Error("Data[0] should be 1")
	}
	if dataBits[10] != 1 {
		t.Error("Data[10] should be 1")
	}
	if dataBits[63] != 1 {
		t.Error("Data[63] should be 1")
	}
	if dataBits[1] != 0 {
		t.Error("Data[1] should be 0")
	}
}

func TestMBCHeader_DecodeLastBlock(t *testing.T) {
	var data [64]bit.Bit
	infoBits := buildMBCHeaderBits(true, false, 0b00000001, 0x10, data)

	hdr := pdu.DecodeMBCHeader(infoBits)
	fecResult := hdr.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeMBCHeader returned uncorrectable FEC")
	}

	if !hdr.LastBlock {
		t.Error("LastBlock should be true")
	}
	if hdr.ProtectFlag {
		t.Error("ProtectFlag should be false")
	}
	if byte(hdr.Opcode) != 0b00000001 {
		t.Errorf("CSBKOpcode = %08b, want 00000001", byte(hdr.Opcode))
	}
	if hdr.FID != 0x10 {
		t.Errorf("FID = 0x%02X, want 0x10", hdr.FID)
	}
}

func TestMBCHeader_EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		lb     bool
		pf     bool
		opcode byte
		fid    byte
	}{
		{"PreambleNoFlags", false, false, 0b00111101, 0x00},
		{"PreambleLastBlock", true, false, 0b00111101, 0x00},
		{"PreambleProtected", false, true, 0b00111101, 0x00},
		{"CustomOpcode", false, false, 0b00010101, 0xFF},
		{"AllFlags", true, true, 0b00111111, 0x42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data [64]bit.Bit
			for i := 0; i < 64; i += 3 {
				data[i] = 1
			}

			orig := pdu.MBCHeader{
				LastBlock:   tt.lb,
				ProtectFlag: tt.pf,
				Opcode:      pdu.CSBKOpcode(tt.opcode),
				FID:         enums.FeatureSetID(tt.fid),
				Data:        [8]byte(bit.Pack(data[:])),
			}

			encoded := must(orig.Encode())
			decoded := pdu.DecodeMBCHeader(encoded)
			fecResult := decoded.FEC
			if fecResult.Uncorrectable {
				t.Fatal("DecodeMBCHeader round-trip returned uncorrectable FEC")
			}

			if decoded.LastBlock != orig.LastBlock {
				t.Errorf("LastBlock = %v, want %v", decoded.LastBlock, orig.LastBlock)
			}
			if decoded.ProtectFlag != orig.ProtectFlag {
				t.Errorf("ProtectFlag = %v, want %v", decoded.ProtectFlag, orig.ProtectFlag)
			}
			if decoded.Opcode != orig.Opcode {
				t.Errorf("CSBKOpcode = %08b, want %08b", byte(decoded.Opcode), byte(orig.Opcode))
			}
			if decoded.FID != orig.FID {
				t.Errorf("FID = 0x%02X, want 0x%02X", decoded.FID, orig.FID)
			}
			if decoded.Data != orig.Data {
				t.Errorf("Data = %x, want %x", decoded.Data, orig.Data)
			}
		})
	}
}

func TestMBCHeader_EncodeStability(t *testing.T) {
	orig := pdu.MBCHeader{
		LastBlock:   true,
		ProtectFlag: false,
		Opcode:      pdu.CSBKPreamble,
		FID:         0x68,
	}
	orig.Data = [8]byte{0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA, 0xAA}

	enc1 := must(orig.Encode())
	dec1 := pdu.DecodeMBCHeader(enc1)
	enc2 := must(dec1.Encode())

	for i := 0; i < 96; i++ {
		if enc1[i] != enc2[i] {
			t.Errorf("Encode stability failed at bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestMBCHeader_CRCFailure(t *testing.T) {
	var infoBits [96]bit.Bit
	fecResult := pdu.DecodeMBCHeader(infoBits).FEC
	if !fecResult.Uncorrectable {
		t.Error("DecodeMBCHeader should return uncorrectable for invalid CRC")
	}
}

func TestMBCHeader_CRCBitFlip(t *testing.T) {
	var data [64]bit.Bit
	data[0] = 1
	infoBits := buildMBCHeaderBits(true, true, 0b00111101, 0x00, data)

	// Flip a data bit
	infoBits[20] ^= 1

	fecResult := pdu.DecodeMBCHeader(infoBits).FEC
	if !fecResult.Uncorrectable {
		t.Error("DecodeMBCHeader should return uncorrectable after bit flip")
	}
}

func TestMBCHeader_DataType(t *testing.T) {
	hdr := &pdu.MBCHeader{}
	if hdr.DataType() != enums.DataTypeMBCHeader {
		t.Errorf("DataType() = %d, want DataTypeMBCHeader", hdr.DataType())
	}
}

func TestMBCHeader_String(t *testing.T) {
	var data [64]bit.Bit
	infoBits := buildMBCHeaderBits(true, false, 0b00111101, 0x00, data)
	hdr := pdu.DecodeMBCHeader(infoBits)

	str := hdr.String()
	if str == "" {
		t.Error("MBCHeader.String() should not be empty")
	}
}

func TestMBCContinuation_DecodeIntermediate(t *testing.T) {
	var data [95]bit.Bit
	data[0] = 1
	data[50] = 1
	data[94] = 1

	infoBits := buildMBCContinuationBits(false, data) // LB=0 → intermediate

	cont := pdu.DecodeMBCContinuation(infoBits)

	if cont.LastBlock {
		t.Error("LastBlock should be false for intermediate block")
	}
	if rawBit(cont.Data[:], 0) != 1 {
		t.Error("Data[0] should be 1")
	}
	if rawBit(cont.Data[:], 50) != 1 {
		t.Error("Data[50] should be 1")
	}
	if rawBit(cont.Data[:], 94) != 1 {
		t.Error("Data[94] should be 1")
	}
	if rawBit(cont.Data[:], 1) != 0 {
		t.Error("Data[1] should be 0")
	}
}

func TestMBCContinuation_DecodeLastBlock(t *testing.T) {
	var data [95]bit.Bit
	data[0] = 1
	data[78] = 1

	infoBits := buildMBCContinuationBits(true, data) // LB=1 → last block

	cont := pdu.DecodeMBCContinuation(infoBits)

	if !cont.LastBlock {
		t.Error("LastBlock should be true for last block")
	}
	if rawBit(cont.Data[:], 0) != 1 {
		t.Error("Data[0] should be 1")
	}
	if rawBit(cont.Data[:], 78) != 1 {
		t.Error("Data[78] should be 1")
	}
}

func TestMBCContinuation_EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		lb   bool
	}{
		{"Intermediate", false},
		{"LastBlock", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var data [95]bit.Bit
			for i := 0; i < 95; i += 2 {
				data[i] = 1
			}

			orig := pdu.MBCContinuation{
				LastBlock: tt.lb,
				Data:      [12]byte(bit.Pack(data[:])),
			}

			encoded := must(orig.Encode())
			decoded := pdu.DecodeMBCContinuation(encoded)

			if decoded.LastBlock != orig.LastBlock {
				t.Errorf("LastBlock = %v, want %v", decoded.LastBlock, orig.LastBlock)
			}
			if decoded.Data != orig.Data {
				t.Errorf("Data = %x, want %x", decoded.Data, orig.Data)
			}
		})
	}
}

func TestMBCContinuation_EncodeStability(t *testing.T) {
	orig := pdu.MBCContinuation{
		LastBlock: true,
	}
	for i := 0; i < 95; i += 3 {
		orig.Data[i/8] |= 0x80 >> (i % 8)
	}

	enc1 := must(orig.Encode())
	dec1 := pdu.DecodeMBCContinuation(enc1)
	enc2 := must(dec1.Encode())

	for i := 0; i < 96; i++ {
		if enc1[i] != enc2[i] {
			t.Errorf("Encode stability failed at bit %d: first=%d, second=%d", i, enc1[i], enc2[i])
		}
	}
}

func TestMBCContinuation_DataType(t *testing.T) {
	cont := &pdu.MBCContinuation{}
	if cont.DataType() != enums.DataTypeMBCContinuation {
		t.Errorf("DataType() = %d, want DataTypeMBCContinuation", cont.DataType())
	}
}

func TestMBCContinuation_String(t *testing.T) {
	var data [95]bit.Bit
	data[0] = 1

	infoBits := buildMBCContinuationBits(false, data)
	cont := pdu.DecodeMBCContinuation(infoBits)

	str := cont.String()
	if str == "" {
		t.Error("MBCContinuation.String() should not be empty")
	}
}

func TestMBCContinuation_NoCRCCheck(t *testing.T) {
	var infoBits [96]bit.Bit
	for i := 0; i < 96; i += 3 {
		infoBits[i] = 1
	}

	cont := pdu.DecodeMBCContinuation(infoBits)

	if !cont.LastBlock {
		t.Error("LastBlock should be true (bit 0 = 1)")
	}
}
