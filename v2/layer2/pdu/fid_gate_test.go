package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/crc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/reedsolomon"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func buildCSBKFID(opcode, fid byte, payload [8]byte) [12]byte {
	var b [12]byte
	b[0] = 0x80 | (opcode & 0x3F)
	b[1] = fid
	copy(b[2:10], payload[:])
	c := crc.CCITT(b[:10]) ^ 0xA5A5
	b[10] = byte(c >> 8)
	b[11] = byte(c & 0xFF)
	return b
}

func testPayload64() [8]byte {
	var p [8]byte
	for i := range 64 {
		bits.SetBit(p[:], i, byte((i*7/3)%2))
	}
	return p
}

// ETSI TS 102 361-1 Table 9.21
func TestCSBK_ManufacturerFIDKeepsRawPayload(t *testing.T) {
	t.Parallel()

	payload := testPayload64()
	infoBits := buildCSBKFID(byte(pdu.CSBKPrivateVoiceGrant), byte(enums.FIDMotorola), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeCSBK returned uncorrectable FEC")
	}
	if csbk.FID != enums.FIDMotorola {
		t.Errorf("FID = %#x, want %#x", csbk.FID, byte(enums.FIDMotorola))
	}
	if _, ok := csbk.AsPrivateVoiceGrant(); ok {
		t.Error("AsPrivateVoiceGrant should be false for a manufacturer FID")
	}
	if csbk.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
	if got := must(csbk.Encode()); got != infoBits {
		t.Error("DecodeCSBK(x).Encode() != x")
	}
}

func TestCSBK_UnknownOpcodeKeepsRawPayload(t *testing.T) {
	t.Parallel()

	payload := testPayload64()
	infoBits := buildCSBK(0b111111, payload)

	csbk := pdu.DecodeCSBK(infoBits)
	fecResult := csbk.FEC
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

func TestCSBK_StandardFIDSelectsPayload(t *testing.T) {
	t.Parallel()

	payload := testPayload64()
	infoBits := buildCSBK(byte(pdu.CSBKPrivateVoiceGrant), payload)

	csbk := pdu.DecodeCSBK(infoBits)
	if _, ok := csbk.AsPrivateVoiceGrant(); !ok {
		t.Fatal("AsPrivateVoiceGrant should be true for the SFID")
	}
	if csbk.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
}

func TestFLC_ManufacturerFIDKeepsRawPayload(t *testing.T) {
	t.Parallel()

	var payload [7]byte
	for i := range 56 {
		bits.SetBit(payload[:], i, byte(i%3%2))
	}
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCOGroupVoiceChannelUser,
		FID:           enums.FIDMotorola,
		RawPayload:    payload,
	}
	infoBits := must(original.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	fecResult := decoded.FEC
	if fecResult.Uncorrectable {
		t.Fatal("DecodeFullLinkControl returned uncorrectable FEC")
	}
	if decoded.FID != enums.FIDMotorola {
		t.Errorf("FeatureSetID = %#x, want %#x", decoded.FID, enums.FIDMotorola)
	}
	if _, ok := decoded.AsGroupVoice(); ok {
		t.Error("AsGroupVoice should be false for a manufacturer FID")
	}
	if decoded.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}
	if got := must(decoded.Encode()); got != infoBits {
		t.Error("DecodeFullLinkControl(x).Encode() != x")
	}
}

func TestFLC_UnknownFLCOKeepsRawValues(t *testing.T) {
	t.Parallel()

	var payload [7]byte
	bits.SetBit(payload[:], 0, 1)
	bits.SetBit(payload[:], 55, 1)
	original := &pdu.FullLinkControl{
		BurstDataType: enums.DataTypeVoiceLCHeader,
		FLCO:          enums.FLCO(0b111111),
		FID:           enums.FID(0x7F),
		RawPayload:    payload,
	}
	infoBits := must(original.Encode())

	decoded := pdu.DecodeFullLinkControl(infoBits, original.DataType())
	if decoded.FLCO != 0b111111 {
		t.Errorf("FLCO = %#x, want 0x3f", decoded.FLCO)
	}
	if decoded.FID != 0x7F {
		t.Errorf("FeatureSetID = %#x, want 0x7f", decoded.FID)
	}
	if decoded.RawPayload != payload {
		t.Error("RawPayload does not hold the payload")
	}

	original.FID = enums.FIDStandardized
	decoded = pdu.DecodeFullLinkControl(must(original.Encode()), original.DataType())
	if decoded.FLCO != 0b111111 || decoded.RawPayload != payload {
		t.Error("unknown FLCO with SFID should keep the raw FLCO and payload")
	}
}

func TestFLC_AddressesIgnoreFID(t *testing.T) {
	t.Parallel()

	for _, fid := range []enums.FID{enums.FIDStandardized, enums.FIDMotorola} {
		for _, flco := range []enums.FLCO{enums.FLCOGroupVoiceChannelUser, enums.FLCOUnitToUnitVoiceChannelUser} {
			lc := reedsolomon.Encode([9]byte{byte(flco), byte(fid), 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C})
			for i := 9; i < 12; i++ {
				lc[i] ^= 0x96
			}
			flc := pdu.DecodeFullLinkControl(lc, enums.DataTypeVoiceLCHeader)
			fecResult := flc.FEC
			if fecResult.Uncorrectable || fecResult.ErrorsCorrected != 0 {
				t.Fatalf("FEC = %+v", fecResult)
			}

			target, source, ok := flc.Addresses()
			if !ok || target != 9990 || source != 3191868 {
				t.Errorf("FID %#x FLCO %d: Addresses() = %d, %d, %v, want 9990, 3191868, true", fid, flco, target, source, ok)
			}
			_, gv := flc.AsGroupVoice()
			_, uu := flc.AsUnitToUnit()
			if fid == enums.FIDMotorola && (gv || uu) {
				t.Errorf("FID %#x FLCO %d: variant decoded for a manufacturer FID", fid, flco)
			}
		}
	}

	flc := pdu.FullLinkControl{FLCO: enums.FLCOGPSInfo, FID: enums.FIDMotorola}
	if _, _, ok := flc.Addresses(); ok {
		t.Error("Addresses() ok for GPS Info")
	}
}
