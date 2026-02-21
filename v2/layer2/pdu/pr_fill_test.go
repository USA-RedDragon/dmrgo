package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/elements"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestPRFill_Length(t *testing.T) {
	t.Parallel()
	if len(pdu.PRFillBits) != 96 {
		t.Fatalf("PRFillBits should be 96 bits, got %d", len(pdu.PRFillBits))
	}
}

func TestPRFill_BitsAreOnlyZeroOrOne(t *testing.T) {
	t.Parallel()
	for i, b := range pdu.PRFillBits {
		if b != 0 && b != 1 {
			t.Fatalf("PRFillBits[%d] = %d, expected 0 or 1", i, b)
		}
	}
}

func TestPRFill_PackedHex(t *testing.T) {
	t.Parallel()
	// PR FILL should pack to FF 83 DF 16 64 21 4E 16 EC 5C CD 49
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x16, 0x64, 0x21,
		0x4E, 0x16, 0xEC, 0x5C, 0xCD, 0x49,
	}
	var packed [12]byte
	for i := 0; i < 96; i++ {
		packed[i/8] |= byte(pdu.PRFillBits[i]) << (7 - (i % 8))
	}
	if packed != expected {
		t.Fatalf("packed PRFillBits = %X, expected %X", packed, expected)
	}
}

func TestPRFill_IsPRFill_Match(t *testing.T) {
	t.Parallel()
	if !pdu.IsPRFill(pdu.PRFillBits) {
		t.Fatal("IsPRFill should return true for PRFillBits")
	}
}

func TestPRFill_IsPRFill_NoMatch(t *testing.T) {
	t.Parallel()
	var zeroBits [96]bit.Bit
	if pdu.IsPRFill(zeroBits) {
		t.Fatal("IsPRFill should return false for all-zero bits")
	}
}

func TestPRFill_IsPRFill_SingleBitDifference(t *testing.T) {
	t.Parallel()
	modified := pdu.PRFillBits
	modified[0] ^= 1
	if pdu.IsPRFill(modified) {
		t.Fatal("IsPRFill should return false for bits with single-bit difference")
	}
}

func TestPRFill_GetDataType(t *testing.T) {
	t.Parallel()
	p := &pdu.PRFill{DataType: elements.DataTypeIdle}
	if p.GetDataType() != elements.DataTypeIdle {
		t.Fatalf("GetDataType() = %v, expected DataTypeIdle", p.GetDataType())
	}
}

func TestPRFill_ToString(t *testing.T) {
	t.Parallel()
	p := &pdu.PRFill{}
	s := p.ToString()
	if s == "" {
		t.Fatal("ToString() should not be empty")
	}
}