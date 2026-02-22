package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildShortLCBits constructs a valid 36-bit Short LC PDU with correct CRC-8.
// slco is the 4-bit SLCO value, data is the 24-bit payload.
func buildShortLCBits(slco byte, data [24]bit.Bit) [36]bit.Bit {
	var bits [36]bit.Bit
	// SLCO: bits 0-3
	bits[0] = bit.Bit((slco >> 3) & 1)
	bits[1] = bit.Bit((slco >> 2) & 1)
	bits[2] = bit.Bit((slco >> 1) & 1)
	bits[3] = bit.Bit(slco & 1)
	// Data: bits 4-27
	copy(bits[4:28], data[:])
	// CRC-8: bits 28-35
	crcVal := crc.CalculateCRC8(bits[:28])
	for i := range 8 {
		bits[28+i] = bit.Bit((crcVal >> (7 - i)) & 1)
	}
	return bits
}

func TestShortLC_NullMessage_Decode(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	bits := buildShortLCBits(byte(enums.SLCONullMessage), data)

	slc, fecResult := pdu.DecodeShortLC(bits)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
	if slc.NullMessage == nil {
		t.Fatal("NullMessage should not be nil")
	}
	if slc.ActivityUpdate != nil {
		t.Error("ActivityUpdate should be nil for NullMessage")
	}
}

func TestShortLC_ActivityUpdate_Decode(t *testing.T) {
	t.Parallel()
	// TS1ActivityID=5, TS2ActivityID=3, HashTS1=0xAB, HashTS2=0xCD
	var data [24]bit.Bit
	// TS1ActivityID: 0101
	data[0], data[1], data[2], data[3] = 0, 1, 0, 1
	// TS2ActivityID: 0011
	data[4], data[5], data[6], data[7] = 0, 0, 1, 1
	// HashTS1: 0xAB = 10101011
	data[8], data[9], data[10], data[11] = 1, 0, 1, 0
	data[12], data[13], data[14], data[15] = 1, 0, 1, 1
	// HashTS2: 0xCD = 11001101
	data[16], data[17], data[18], data[19] = 1, 1, 0, 0
	data[20], data[21], data[22], data[23] = 1, 1, 0, 1

	bits := buildShortLCBits(byte(enums.SLCOActivityUpdate), data)

	slc, fecResult := pdu.DecodeShortLC(bits)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCOActivityUpdate {
		t.Errorf("expected SLCOActivityUpdate, got %d", slc.SLCO)
	}
	if slc.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate should not be nil")
	}
	if slc.ActivityUpdate.TS1ActivityID != 5 {
		t.Errorf("expected TS1ActivityID=5, got %d", slc.ActivityUpdate.TS1ActivityID)
	}
	if slc.ActivityUpdate.TS2ActivityID != 3 {
		t.Errorf("expected TS2ActivityID=3, got %d", slc.ActivityUpdate.TS2ActivityID)
	}
	if slc.ActivityUpdate.HashTS1 != 0xAB {
		t.Errorf("expected HashTS1=0xAB, got 0x%02X", slc.ActivityUpdate.HashTS1)
	}
	if slc.ActivityUpdate.HashTS2 != 0xCD {
		t.Errorf("expected HashTS2=0xCD, got 0x%02X", slc.ActivityUpdate.HashTS2)
	}
}

func TestShortLC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	// Build an Activity Update, encode it, decode it, verify match
	original := pdu.ShortLC{
		SLCO: enums.SLCOActivityUpdate,
		ActivityUpdate: &pdu.ShortLCActivityUpdate{
			TS1ActivityID: 7,
			TS2ActivityID: 2,
			HashTS1:       0x42,
			HashTS2:       0xFF,
		},
	}

	encoded := pdu.EncodeShortLC(&original)
	decoded, fecResult := pdu.DecodeShortLC(encoded)
	if fecResult.Uncorrectable {
		t.Fatal("round-trip CRC failed")
	}
	if decoded.SLCO != enums.SLCOActivityUpdate {
		t.Errorf("SLCO mismatch: got %d", decoded.SLCO)
	}
	if decoded.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate is nil after round-trip")
	}
	if decoded.ActivityUpdate.TS1ActivityID != 7 {
		t.Errorf("TS1ActivityID: got %d, want 7", decoded.ActivityUpdate.TS1ActivityID)
	}
	if decoded.ActivityUpdate.TS2ActivityID != 2 {
		t.Errorf("TS2ActivityID: got %d, want 2", decoded.ActivityUpdate.TS2ActivityID)
	}
	if decoded.ActivityUpdate.HashTS1 != 0x42 {
		t.Errorf("HashTS1: got 0x%02X, want 0x42", decoded.ActivityUpdate.HashTS1)
	}
	if decoded.ActivityUpdate.HashTS2 != 0xFF {
		t.Errorf("HashTS2: got 0x%02X, want 0xFF", decoded.ActivityUpdate.HashTS2)
	}
}

func TestShortLC_CRCFailure(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	bits := buildShortLCBits(byte(enums.SLCONullMessage), data)

	// Corrupt a CRC bit
	bits[30] ^= 1

	_, fecResult := pdu.DecodeShortLC(bits)
	if !fecResult.Uncorrectable {
		t.Error("expected uncorrectable after CRC corruption")
	}
}

func TestShortLC_BitsChecked(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	bits := buildShortLCBits(byte(enums.SLCONullMessage), data)

	_, fecResult := pdu.DecodeShortLC(bits)
	if fecResult.BitsChecked != 36 {
		t.Errorf("expected BitsChecked=36, got %d", fecResult.BitsChecked)
	}
}

func TestShortLC_ToString(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	bits := buildShortLCBits(byte(enums.SLCOActivityUpdate), data)

	slc, _ := pdu.DecodeShortLC(bits)
	s := slc.ToString()
	if s == "" {
		t.Error("ToString should not be empty")
	}
}
