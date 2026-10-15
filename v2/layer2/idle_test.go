package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestIdleMessageInfoBits_Length(t *testing.T) {
	if len(layer2.IdleMessageInfoBits) != 96 {
		t.Errorf("IdleMessageInfoBits length = %d, want 96", len(layer2.IdleMessageInfoBits))
	}
}

func TestIdleMessageInfoBits_PackedHex(t *testing.T) {
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
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
	for i := range layer2.IdleMessageFECMatrix {
		if len(layer2.IdleMessageFECMatrix[i]) != 15 {
			t.Errorf("FEC matrix row %d cols = %d, want 15", i, len(layer2.IdleMessageFECMatrix[i]))
		}
	}
}

func TestIdleMessageFECMatrix_TotalBits(t *testing.T) {
	total := len(layer2.IdleMessageFECMatrix) * len(layer2.IdleMessageFECMatrix[0])
	if total != 195 {
		t.Errorf("FEC matrix total cells = %d, want 195", total)
	}
}

// The info bits sit in row 0 columns 3-10 and rows 1-8 columns 0-10 of
// Figure D.1 (layout of Figure B.1).
func TestIdleMessageFECMatrix_MatchesInfoBits(t *testing.T) {
	var info [96]bit.Bit
	n := 0
	for c := 3; c <= 10; c++ {
		info[n] = layer2.IdleMessageFECMatrix[0][c]
		n++
	}
	for r := 1; r <= 8; r++ {
		for c := 0; c <= 10; c++ {
			info[n] = layer2.IdleMessageFECMatrix[r][c]
			n++
		}
	}
	if info != layer2.IdleMessageInfoBits {
		t.Errorf("FEC matrix info bits = %X, want %X", bit.PackBits(info[:]), bit.PackBits(layer2.IdleMessageInfoBits[:]))
	}
}

func TestIdleMessageInfoBits_EqualsPRFill(t *testing.T) {
	if layer2.IdleMessageInfoBits != pdu.PRFillBits {
		t.Error("IdleMessageInfoBits should equal pdu.PRFillBits")
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
