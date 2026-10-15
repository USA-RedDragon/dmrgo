package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestPRFill_Length(t *testing.T) {
	t.Parallel()
	if len(pdu.PRFillBits()) != 96 {
		t.Fatalf("PRFillBits should be 96 bits, got %d", len(pdu.PRFillBits()))
	}
}

func TestPRFill_BitsAreOnlyZeroOrOne(t *testing.T) {
	t.Parallel()
	for i, b := range pdu.PRFillBits() {
		if b != 0 && b != 1 {
			t.Fatalf("PRFillBits[%d] = %d, expected 0 or 1", i, b)
		}
	}
}

func TestPRFill_PackedHex(t *testing.T) {
	t.Parallel()
	// ETSI TS 102 361-1 Table D.2, I(95) first.
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	bits := pdu.PRFillBits()
	var packed [12]byte
	for i := 0; i < 96; i++ {
		packed[i/8] |= byte(bits[i]) << (7 - (i % 8))
	}
	if packed != expected {
		t.Fatalf("packed PRFillBits = %X, expected %X", packed, expected)
	}
}

func TestPRFill_IsPRFill_Match(t *testing.T) {
	t.Parallel()
	if !pdu.IsPRFill(pdu.PRFillBits()) {
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
	modified := pdu.PRFillBits()
	modified[0] ^= 1
	if pdu.IsPRFill(modified) {
		t.Fatal("IsPRFill should return false for bits with single-bit difference")
	}
}

func TestPRFill_PRFillBits_ReturnsCopy(t *testing.T) {
	t.Parallel()
	bits := pdu.PRFillBits()
	bits[0] ^= 1
	if !pdu.IsPRFill(pdu.PRFillBits()) {
		t.Fatal("changing the returned bits changed the PR FILL constant")
	}
}

func TestPRFill_GetDataType(t *testing.T) {
	t.Parallel()
	p := &pdu.PRFill{DataType: enums.DataTypeIdle}
	if p.GetDataType() != enums.DataTypeIdle {
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
