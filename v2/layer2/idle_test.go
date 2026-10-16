package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// idleMessageFECMatrix is the 13x15 BPTC(196,96) idle message before
// interleaving, row-major (ETSI TS 102 361-1 Figure D.1).

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
	t.Parallel()

	expected := [12]byte{
		0xFF, 0x83, 0xDF, 0x17, 0x32, 0x09,
		0x4E, 0xD1, 0xE7, 0xCD, 0x8A, 0x91,
	}
	info := layer2.IdleMessageInfo()
	packed := bit.Pack(info[:])
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
	t.Parallel()

	for i, b := range layer2.IdleMessageInfo() {
		if b != 0 && b != 1 {
			t.Errorf("IdleMessageInfo()[%d] = %d, want 0 or 1", i, b)
		}
	}
}

func TestIdleMessageInfo_EqualsPRFill(t *testing.T) {
	t.Parallel()

	if layer2.IdleMessageInfo() != pdu.PRFillBits() {
		t.Error("IdleMessageInfo() should equal pdu.PRFillBits()")
	}
}

func TestIdleMessageInfo_ReturnsCopy(t *testing.T) {
	t.Parallel()

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

// TestIdleMessageFECMatrix_MatchesInfoBits checks the info bits in row 0
// columns 3 to 10 and rows 1 to 8 columns 0 to 10 of the matrix (ETSI TS
// 102 361-1 Figure B.1, Figure D.1).
func TestIdleMessageFECMatrix_MatchesInfoBits(t *testing.T) {
	t.Parallel()

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
		t.Errorf("FEC matrix info bits = %X, want %X", bit.Pack(info[:]), bit.Pack(want[:]))
	}
}

// TestIdleMessageFECMatrix_MatchesBPTCEncode checks that bptc.Encode196 moves
// matrix bit i to transmit bit (i*181)%196. Bit 0 is the pad bit R(3), so
// the matrix starts at bit 1 (ETSI TS 102 361-1 clause B.1.1, Figure D.1).
func TestIdleMessageFECMatrix_MatchesBPTCEncode(t *testing.T) {
	t.Parallel()

	encoded := bptc.Encode196(layer2.IdleMessageInfo())
	for r := range idleMessageFECMatrix {
		for c := range idleMessageFECMatrix[r] {
			i := 1 + r*15 + c
			if got := encoded[(i*181)%196]; got != idleMessageFECMatrix[r][c] {
				t.Errorf("Figure D.1 [%d][%d] = %d, bptc.Encode196 gives %d", r, c, idleMessageFECMatrix[r][c], got)
			}
		}
	}
}

func TestIsIdleMessage_Match(t *testing.T) {
	t.Parallel()

	if !layer2.IsIdleMessage(layer2.IdleMessageInfo()) {
		t.Error("IsIdleMessage should return true for IdleMessageInfo()")
	}
}

func TestIsIdleMessage_NoMatch(t *testing.T) {
	t.Parallel()

	var bits [96]bit.Bit
	if layer2.IsIdleMessage(bits) {
		t.Error("IsIdleMessage should return false for all-zero bits")
	}
}

// TestIsNullEmbeddedLC_Match checks that the single burst BPTC of 11 zero
// information bits is 32 zero transmit bits (ETSI TS 102 361-1 clause B.2.1,
// Table D.1).
func TestIsNullEmbeddedLC_Match(t *testing.T) {
	t.Parallel()

	var null [32]bit.Bit
	bit.PutUint(null[:], uint64(bptc.EncodeSingleBurst(0, false)))
	if null != [32]bit.Bit{} {
		t.Errorf("single burst BPTC of 11 zero bits = %v, want 32 zero bits", null)
	}
	if !layer2.IsNullEmbeddedLC(null) {
		t.Error("IsNullEmbeddedLC should return true for 32 zero bits")
	}
}

func TestIsNullEmbeddedLC_NoMatch(t *testing.T) {
	t.Parallel()

	var bits [32]bit.Bit
	bits[0] = 1
	if layer2.IsNullEmbeddedLC(bits) {
		t.Error("IsNullEmbeddedLC should return false for non-zero bits")
	}
}
