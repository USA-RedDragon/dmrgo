package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildCSBKBitsFID is buildCSBKBits with a caller-chosen FID.
func buildCSBKBitsFID(opcode, fid byte, payload [64]bit.Bit) [96]bit.Bit {
	var b [12]byte
	b[0] = 0x80 | (opcode & 0x3F)
	b[1] = fid
	copy(b[2:10], bit.PackBits(payload[:]))
	c := crc.CalculateCRCCCITT(b[:10]) ^ 0xA5A5
	b[10] = byte(c >> 8)
	b[11] = byte(c & 0xFF)
	var bits [96]bit.Bit
	copy(bits[:], bit.UnpackBits(b[:]))
	return bits
}

func testPayload64() [64]bit.Bit {
	var p [64]bit.Bit
	for i := range p {
		p[i] = bit.Bit((i * 7 / 3) % 2)
	}
	return p
}

// ETSI TS 102 361-1 Table 9.21: the FID redefines the CSBKO, so a
// manufacturer FID must not be decoded with the standard layouts.
func TestCSBK_ManufacturerFIDKeepsRawPayload(t *testing.T) {
	payload := testPayload64()
	infoBits := buildCSBKBitsFID(byte(pdu.CSBKPrivateVoiceGrant), byte(enums.FIDMotorola), payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.FID != byte(enums.FIDMotorola) {
		t.Errorf("FID = %#x, want %#x", csbk.FID, byte(enums.FIDMotorola))
	}
	if csbk.PrivateVoiceGrantPDU != nil {
		t.Error("PrivateVoiceGrantPDU should be nil for a manufacturer FID")
	}
	if csbk.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
	if got := must(csbk.Encode()); got != infoBits {
		t.Error("DecodeCSBK(x).Encode() != x")
	}
}

func TestCSBK_UnknownOpcodeKeepsRawPayload(t *testing.T) {
	payload := testPayload64()
	infoBits := buildCSBKBits(0b111111, payload)

	csbk, fecResult := pdu.DecodeCSBK(infoBits)
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
	if got := must(csbk.Encode()); got != infoBits {
		t.Error("DecodeCSBK(x).Encode() != x")
	}
}

func TestCSBK_StandardFIDDoesNotFillRawPayload(t *testing.T) {
	payload := testPayload64()
	infoBits := buildCSBKBits(byte(pdu.CSBKPrivateVoiceGrant), payload)

	csbk, _ := pdu.DecodeCSBK(infoBits)
	if csbk.PrivateVoiceGrantPDU == nil {
		t.Fatal("PrivateVoiceGrantPDU should be set for the SFID")
	}
	if csbk.RawPayload != [64]bit.Bit{} {
		t.Error("RawPayload should be empty when a layout matched")
	}
}

func TestFLC_ManufacturerFIDKeepsRawPayload(t *testing.T) {
	var payload [56]bit.Bit
	for i := range payload {
		payload[i] = bit.Bit(i % 3 % 2)
	}
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FeatureSetID:  enums.FIDMotorola,
		RawPayload:    payload,
	}
	infoBits := must(original.Encode())

	decoded, fecResult := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FeatureSetID != enums.FIDMotorola {
		t.Errorf("FeatureSetID = %#x, want %#x", decoded.FeatureSetID, enums.FIDMotorola)
	}
	if decoded.GroupVoice != nil {
		t.Error("GroupVoice should be nil for a manufacturer FID")
	}
	if decoded.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
	if got := must(decoded.Encode()); got != infoBits {
		t.Error("DecodeFullLinkControl(x).Encode() != x")
	}
}

func TestFLC_UnknownFLCOKeepsRawValues(t *testing.T) {
	var payload [56]bit.Bit
	payload[0] = 1
	payload[55] = 1
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCO(0b111111),
		FeatureSetID:  enums.FeatureSetID(0x7F),
		RawPayload:    payload,
	}
	infoBits := must(original.Encode())

	decoded, _ := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	if decoded.FLCO != 0b111111 {
		t.Errorf("FLCO = %#x, want 0x3f", decoded.FLCO)
	}
	if decoded.FeatureSetID != 0x7F {
		t.Errorf("FeatureSetID = %#x, want 0x7f", decoded.FeatureSetID)
	}
	if decoded.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}

	original.FeatureSetID = enums.FIDStandardized
	decoded, _ = pdu.DecodeFullLinkControl(must(original.Encode()), original.DataType())
	if decoded.FLCO != 0b111111 || decoded.RawPayload != payload {
		t.Error("unknown FLCO with SFID should keep the raw FLCO and payload")
	}
}

func TestFLC_AddressesIgnoreFID(t *testing.T) {
	for _, fid := range []enums.FeatureSetID{enums.FIDStandardized, enums.FIDMotorola} {
		for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
			lc := reedsolomon.Encode([9]byte{byte(flco), byte(fid), 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C})
			for i := 9; i < 12; i++ {
				lc[i] ^= 0x96
			}
			var infoBits [96]bit.Bit
			copy(infoBits[:], bit.UnpackBits(lc[:]))
			flc, fecResult := pdu.DecodeFullLinkControl(infoBits, enums.DataTypeVoiceLCHeader)
			if fecResult.Uncorrectable || fecResult.ErrorsCorrected != 0 {
				t.Fatalf("FEC = %+v", fecResult)
			}

			target, source, ok := flc.Addresses()
			if !ok || target != 9990 || source != 3191868 {
				t.Errorf("FID %#x FLCO %d: Addresses() = %d, %d, %v, want 9990, 3191868, true", fid, flco, target, source, ok)
			}
			if fid == enums.FIDMotorola && (flc.GroupVoice != nil || flc.UnitToUnit != nil) {
				t.Errorf("FID %#x FLCO %d: variant decoded for a manufacturer FID", fid, flco)
			}
		}
	}

	flc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, FeatureSetID: enums.FIDMotorola}
	if _, _, ok := flc.Addresses(); ok {
		t.Error("Addresses() ok for GPS Info")
	}
}
