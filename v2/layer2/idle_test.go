package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// idleMessageFECMatrix is the 13x15 BPTC(196,96) idle message of Figure D.1
// of ETSI TS 102 361-1, before interleaving, row-major.
//
//nolint:gochecknoglobals
var idleMessageFECMatrix = [13][15]bit.Bit{
	{0, 0, 0, 1, 1, 1, 1, 1, 1, 1, 1, 0, 1, 0, 0},
	{1, 0, 0, 0, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 1},
	{1, 1, 1, 1, 1, 0, 0, 0, 1, 0, 1, 1, 1, 0, 1},
	{1, 1, 0, 0, 1, 1, 0, 0, 1, 0, 0, 0, 1, 0, 1},
	{0, 0, 0, 1, 0, 0, 1, 0, 1, 0, 0, 0, 1, 1, 1},
	{1, 1, 1, 0, 1, 1, 0, 1, 0, 0, 0, 1, 1, 0, 1},
	{1, 1, 1, 1, 0, 0, 1, 1, 1, 1, 1, 0, 0, 1, 0},
	{0, 0, 1, 1, 0, 1, 1, 0, 0, 0, 1, 1, 1, 0, 1},
	{0, 1, 0, 1, 0, 0, 1, 0, 0, 0, 1, 0, 1, 0, 1},
	{0, 1, 0, 0, 1, 1, 1, 0, 0, 1, 0, 0, 0, 1, 1},
	{1, 0, 1, 1, 0, 0, 1, 1, 1, 1, 0, 1, 1, 0, 0},
	{0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0},
	{0, 1, 0, 0, 1, 0, 1, 0, 1, 1, 1, 0, 1, 1, 0},
}

func TestIdleMessageInfo_PackedHex(t *testing.T) {
	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	info := layer2.IdleMessageInfo()
	packed := bit.PackBits(info[:])
	if len(packed) != 12 {
		t.Fatalf("Packed length = %d, want 12", len(packed))
	}
	for i, b := range packed {
		if b != expected[i] {
			t.Errorf("Packed byte[%d] = 0x%02X, want 0x%02X", i, b, expected[i])
		}
	}
}

func TestIdleMessageInfo_BitsAreOnlyZeroOrOne(t *testing.T) {
	for i, b := range layer2.IdleMessageInfo() {
		if b != 0 && b != 1 {
			t.Errorf("IdleMessageInfo()[%d] = %d, want 0 or 1", i, b)
		}
	}
}

func TestIdleMessageInfo_EqualsPRFill(t *testing.T) {
	if layer2.IdleMessageInfo() != pdu.PRFillBits() {
		t.Error("IdleMessageInfo() should equal pdu.PRFillBits()")
	}
}

func TestIdleMessageInfo_ReturnsCopy(t *testing.T) {
	info := layer2.IdleMessageInfo()
	info[0] ^= 1
	idle := layer2.IdleMessageInfo()
	if !layer2.IsIdleMessage(idle) || !pdu.IsPRFill(idle) {
		t.Fatal("changing the returned bits changed the idle message constant")
	}
	if layer2.IsIdleMessage(info) || pdu.IsPRFill(info) {
		t.Fatal("IsIdleMessage and IsPRFill should reject the changed bits")
	}
}

// The info bits sit in row 0 columns 3-10 and rows 1-8 columns 0-10 of
// Figure D.1 (layout of Figure B.1).
func TestIdleMessageFECMatrix_MatchesInfoBits(t *testing.T) {
	var info [96]bit.Bit
	n := 0
	for c := 3; c <= 10; c++ {
		info[n] = idleMessageFECMatrix[0][c]
		n++
	}
	for r := 1; r <= 8; r++ {
		for c := 0; c <= 10; c++ {
			info[n] = idleMessageFECMatrix[r][c]
			n++
		}
	}
	want := layer2.IdleMessageInfo()
	if info != want {
		t.Errorf("FEC matrix info bits = %X, want %X", bit.PackBits(info[:]), bit.PackBits(want[:]))
	}
}

// bptc.Encode moves matrix bit i to transmit bit (i*181)%196 (ETSI TS
// 102 361-1 clause B.1.1, equation B.1). Bit 0 is the pad bit R(3), so
// Figure D.1 starts at bit 1.
func TestIdleMessageFECMatrix_MatchesBPTCEncode(t *testing.T) {
	encoded := bptc.Encode(layer2.IdleMessageInfo())
	for r := range idleMessageFECMatrix {
		for c := range idleMessageFECMatrix[r] {
			i := 1 + r*15 + c
			if got := encoded[(i*181)%196]; got != idleMessageFECMatrix[r][c] {
				t.Errorf("Figure D.1 [%d][%d] = %d, bptc.Encode gives %d", r, c, idleMessageFECMatrix[r][c], got)
			}
		}
	}
}

func TestIsIdleMessage_Match(t *testing.T) {
	if !layer2.IsIdleMessage(layer2.IdleMessageInfo()) {
		t.Error("IsIdleMessage should return true for IdleMessageInfo()")
	}
}

func TestIsIdleMessage_NoMatch(t *testing.T) {
	var bits [96]bit.Bit
	if layer2.IsIdleMessage(bits) {
		t.Error("IsIdleMessage should return false for all-zero bits")
	}
}

// Table D.1: the 11 information bits are 0, so all 32 transmit bits of the
// clause B.2.1 single burst BPTC are 0.
func TestIsNullEmbeddedLC_Match(t *testing.T) {
	null := bptc.EncodeSingleBurstBPTC([11]bit.Bit{}, false)
	if null != [32]bit.Bit{} {
		t.Errorf("single burst BPTC of 11 zero bits = %v, want 32 zero bits", null)
	}
	if !layer2.IsNullEmbeddedLC(null) {
		t.Error("IsNullEmbeddedLC should return true for 32 zero bits")
	}
}

func TestIsNullEmbeddedLC_NoMatch(t *testing.T) {
	var bits [32]bit.Bit
	bits[0] = 1
	if layer2.IsNullEmbeddedLC(bits) {
		t.Error("IsNullEmbeddedLC should return false for non-zero bits")
	}
}
