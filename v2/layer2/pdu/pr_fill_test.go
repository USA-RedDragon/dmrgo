package pdu_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func TestPRFill_PackedHex(t *testing.T) {
	t.Parallel()
	// ETSI TS 102 361-1 Table D.2
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	if packed := pdu.PRFillBits(); packed != expected {
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
	var zeroBits [12]byte
	if pdu.IsPRFill(zeroBits) {
		t.Fatal("IsPRFill should return false for all-zero bits")
	}
}

func TestPRFill_IsPRFill_SingleBitDifference(t *testing.T) {
	t.Parallel()
	modified := pdu.PRFillBits()
	modified[0] ^= 0x80
	if pdu.IsPRFill(modified) {
		t.Fatal("IsPRFill should return false for bits with single-bit difference")
	}
}

func TestPRFill_PRFillBits_ReturnsCopy(t *testing.T) {
	t.Parallel()
	bits := pdu.PRFillBits()
	bits[0] ^= 0x80
	if !pdu.IsPRFill(pdu.PRFillBits()) {
		t.Fatal("changing the returned bits changed the PR FILL constant")
	}
}

func TestPRFill_DataType(t *testing.T) {
	t.Parallel()
	p := &pdu.PRFill{}
	if p.DataType() != enums.DataTypeIdle {
		t.Fatalf("DataType() = %v, expected DataTypeIdle", p.DataType())
	}
}

func TestPRFill_String(t *testing.T) {
	t.Parallel()
	p := &pdu.PRFill{}
	s := p.String()
	if s == "" {
		t.Fatal("String() should not be empty")
	}
}
