package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestIdleMessageInfoBits_Length(t *testing.T) {
	if len(layer2.IdleMessageInfoBits) != 96 {
		t.Errorf("IdleMessageInfoBits length = %d, want 96", len(layer2.IdleMessageInfoBits))
	}
}

func TestIdleMessageInfoBits_PackedHex(t *testing.T) {
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x16, 0x64, 0x21,
		0x4E, 0x16, 0xEC, 0x5C, 0xCD, 0x49,
	}
	packed := bit.PackBits(layer2.IdleMessageInfoBits[:])
	if len(packed) != 12 {
		t.Fatalf("Packed length = %d, want 12", len(packed))
	}
	for i, b := range packed {
		if b != expected[i] {
			t.Errorf("Packed byte[%d] = 0x%02X, want 0x%02X", i, b, expected[i])
		}
	}
}

func TestIdleMessageInfoBits_BitsAreOnlyZeroOrOne(t *testing.T) {
	for i, b := range layer2.IdleMessageInfoBits {
		if b != 0 && b != 1 {
			t.Errorf("IdleMessageInfoBits[%d] = %d, want 0 or 1", i, b)
		}
	}
}

func TestNullEmbeddedLCBits_AllZeros(t *testing.T) {
	for i, b := range layer2.NullEmbeddedLCBits {
		if b != 0 {
			t.Errorf("NullEmbeddedLCBits[%d] = %d, want 0", i, b)
		}
	}
}

func TestNullEmbeddedLCBits_Length(t *testing.T) {
	if len(layer2.NullEmbeddedLCBits) != 32 {
		t.Errorf("NullEmbeddedLCBits length = %d, want 32", len(layer2.NullEmbeddedLCBits))
	}
}

func TestIdleMessageFECMatrix_Dimensions(t *testing.T) {
	if len(layer2.IdleMessageFECMatrix) != 13 {
		t.Fatalf("FEC matrix rows = %d, want 13", len(layer2.IdleMessageFECMatrix))
	}
	for i, row := range layer2.IdleMessageFECMatrix {
		if len(row) != 15 {
			t.Errorf("FEC matrix row %d cols = %d, want 15", i, len(row))
		}
	}
}

func TestIdleMessageFECMatrix_TotalBits(t *testing.T) {
	total := len(layer2.IdleMessageFECMatrix) * len(layer2.IdleMessageFECMatrix[0])
	if total != 195 {
		t.Errorf("FEC matrix total cells = %d, want 195", total)
	}
}

func TestIsIdleMessage_Match(t *testing.T) {
	if !layer2.IsIdleMessage(layer2.IdleMessageInfoBits) {
		t.Error("IsIdleMessage should return true for IdleMessageInfoBits")
	}
}

func TestIsIdleMessage_NoMatch(t *testing.T) {
	var bits [96]bit.Bit
	if layer2.IsIdleMessage(bits) {
		t.Error("IsIdleMessage should return false for all-zero bits")
	}
}

func TestIsNullEmbeddedLC_Match(t *testing.T) {
	if !layer2.IsNullEmbeddedLC(layer2.NullEmbeddedLCBits) {
		t.Error("IsNullEmbeddedLC should return true for NullEmbeddedLCBits")
	}
}

func TestIsNullEmbeddedLC_NoMatch(t *testing.T) {
	var bits [32]bit.Bit
	bits[0] = 1
	if layer2.IsNullEmbeddedLC(bits) {
		t.Error("IsNullEmbeddedLC should return false for non-zero bits")
	}
}

func TestIdleMessageFECMatrix_BitsAreOnlyZeroOrOne(t *testing.T) {
	for r, row := range layer2.IdleMessageFECMatrix {
		for c, b := range row {
			if b != 0 && b != 1 {
				t.Errorf("FEC matrix[%d][%d] = %d, want 0 or 1", r, c, b)
			}
		}
	}
}
