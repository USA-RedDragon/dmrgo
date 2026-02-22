package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/crc"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// buildValidShortLCFragments constructs 4x17-bit CACH payloads that encode
// a valid Short LC PDU with the given SLCO and 24-bit data payload.
func buildValidShortLCFragments(slco byte, data [24]bit.Bit) [4][17]bit.Bit {
	var bits [36]bit.Bit
	// SLCO: bits 0-3 (MSB-first)
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
	return bptc.EncodeCACHBPTC(bits)
}

func TestShortLCAssembler_Lifecycle(t *testing.T) {
	t.Parallel()
	var a layer2.ShortLCAssembler

	if a.Count() != 0 {
		t.Errorf("initial count should be 0, got %d", a.Count())
	}

	var payload [layer2.CACHPayloadBits]bit.Bit
	for i := 0; i < 3; i++ {
		ready := a.AddFragment(payload)
		if ready {
			t.Errorf("should not be ready after %d fragments", i+1)
		}
		if a.Count() != i+1 {
			t.Errorf("count should be %d, got %d", i+1, a.Count())
		}
	}

	ready := a.AddFragment(payload)
	if !ready {
		t.Error("should be ready after 4 fragments")
	}
	if a.Count() != 4 {
		t.Errorf("count should be 4, got %d", a.Count())
	}

	a.Reset()
	if a.Count() != 0 {
		t.Errorf("count should be 0 after reset, got %d", a.Count())
	}
}

func TestShortLCAssembler_Complete_NullMessage(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	var a layer2.ShortLCAssembler
	for i := range fragments {
		a.AddFragment(fragments[i])
	}

	slc, fecResult := a.Complete()
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
	if slc.NullMessage == nil {
		t.Error("NullMessage should not be nil")
	}
}

func TestShortLCAssembler_Complete_ActivityUpdate(t *testing.T) {
	t.Parallel()
	// TS1ActivityID=5, TS2ActivityID=3, HashTS1=0xAB, HashTS2=0xCD
	var data [24]bit.Bit
	data[0], data[1], data[2], data[3] = 0, 1, 0, 1
	data[4], data[5], data[6], data[7] = 0, 0, 1, 1
	data[8], data[9], data[10], data[11] = 1, 0, 1, 0
	data[12], data[13], data[14], data[15] = 1, 0, 1, 1
	data[16], data[17], data[18], data[19] = 1, 1, 0, 0
	data[20], data[21], data[22], data[23] = 1, 1, 0, 1

	fragments := buildValidShortLCFragments(byte(enums.SLCOActivityUpdate), data)

	var a layer2.ShortLCAssembler
	for i := range fragments {
		a.AddFragment(fragments[i])
	}

	slc, fecResult := a.Complete()
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

func TestShortLCAssembler_DecodeShortLCFromFragments(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	slc, fecResult := layer2.DecodeShortLCFromFragments(fragments)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if slc.SLCO != enums.SLCONullMessage {
		t.Errorf("expected SLCONullMessage, got %d", slc.SLCO)
	}
}

func TestShortLCAssembler_RoundTrip_EncodeShortLC(t *testing.T) {
	t.Parallel()
	// Build a ShortLC, encode it, run through BPTC, then decode via assembler
	original := pdu.ShortLC{
		SLCO: enums.SLCOActivityUpdate,
		ActivityUpdate: &pdu.ShortLCActivityUpdate{
			TS1ActivityID: 15,
			TS2ActivityID: 8,
			HashTS1:       0x12,
			HashTS2:       0x34,
		},
	}
	encoded := pdu.EncodeShortLC(&original)
	fragments := bptc.EncodeCACHBPTC(encoded)

	slc, fecResult := layer2.DecodeShortLCFromFragments(fragments)
	if fecResult.Uncorrectable {
		t.Fatal("round-trip failed: uncorrectable")
	}
	if slc.ActivityUpdate == nil {
		t.Fatal("ActivityUpdate is nil after round-trip")
	}
	if slc.ActivityUpdate.TS1ActivityID != 15 {
		t.Errorf("TS1ActivityID: got %d, want 15", slc.ActivityUpdate.TS1ActivityID)
	}
	if slc.ActivityUpdate.TS2ActivityID != 8 {
		t.Errorf("TS2ActivityID: got %d, want 8", slc.ActivityUpdate.TS2ActivityID)
	}
	if slc.ActivityUpdate.HashTS1 != 0x12 {
		t.Errorf("HashTS1: got 0x%02X, want 0x12", slc.ActivityUpdate.HashTS1)
	}
	if slc.ActivityUpdate.HashTS2 != 0x34 {
		t.Errorf("HashTS2: got 0x%02X, want 0x34", slc.ActivityUpdate.HashTS2)
	}
}

func TestShortLCAssembler_CombinedFEC(t *testing.T) {
	t.Parallel()
	var data [24]bit.Bit
	fragments := buildValidShortLCFragments(byte(enums.SLCONullMessage), data)

	slc, fecResult := layer2.DecodeShortLCFromFragments(fragments)
	// BitsChecked should include both BPTC (68) and CRC (36)
	expectedBitsChecked := 68 + 36
	if fecResult.BitsChecked != expectedBitsChecked {
		t.Errorf("BitsChecked: got %d, want %d", fecResult.BitsChecked, expectedBitsChecked)
	}
	// The FEC on the ShortLC struct should match the returned FEC
	if slc.FEC.BitsChecked != fecResult.BitsChecked {
		t.Errorf("ShortLC.FEC.BitsChecked mismatch: got %d, want %d", slc.FEC.BitsChecked, fecResult.BitsChecked)
	}
}

func TestShortLCAssembler_AddFragment_OverflowReturnsReady(t *testing.T) {
	t.Parallel()
	var a layer2.ShortLCAssembler
	var payload [layer2.CACHPayloadBits]bit.Bit

	for i := 0; i < 4; i++ {
		a.AddFragment(payload)
	}

	// A 5th add should still return true (already full)
	if !a.AddFragment(payload) {
		t.Error("5th AddFragment should return true (already full)")
	}
	// Count should stay at 4
	if a.Count() != 4 {
		t.Errorf("count should stay at 4, got %d", a.Count())
	}
}
