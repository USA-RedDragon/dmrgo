package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// idleMessageFECMatrix is the 13x15 BPTC(196,96) idle message before
// interleaving, row-major (ETSI TS 102 361-1 Figure D.1).

var idleMessageFECMatrix = [13][15]unpacked.Bit{
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

// TestIdleMessageFECMatrix_MatchesInfoBits checks the info bits in row 0
// columns 3 to 10 and rows 1 to 8 columns 0 to 10 of the matrix (ETSI TS
// 102 361-1 Figure B.1, Figure D.1).
func TestIdleMessageFECMatrix_MatchesInfoBits(t *testing.T) {
	t.Parallel()

	var info [12]byte
	n := 0
	for c := 3; c <= 10; c++ {
		bits.SetBit(info[:], n, byte(idleMessageFECMatrix[0][c]))
		n++
	}
	for r := 1; r <= 8; r++ {
		for c := 0; c <= 10; c++ {
			bits.SetBit(info[:], n, byte(idleMessageFECMatrix[r][c]))
			n++
		}
	}
	if want := pdu.PRFillBits(); info != want {
		t.Errorf("FEC matrix info bits = %X, want %X", info, want)
	}
}

// TestIdleMessageFECMatrix_MatchesBPTCEncode checks that bptc.Encode196 moves
// matrix bit i to transmit bit (i*181)%196. Bit 0 is the pad bit R(3), so
// the matrix starts at bit 1 (ETSI TS 102 361-1 clause B.1.1, Figure D.1).
func TestIdleMessageFECMatrix_MatchesBPTCEncode(t *testing.T) {
	t.Parallel()

	encoded := bptc.Encode196(pdu.PRFillBits(), 0)
	for r := range idleMessageFECMatrix {
		for c := range idleMessageFECMatrix[r] {
			i := 1 + r*15 + c
			if got := unpacked.Bit(bits.Bit(encoded[:], (i*181)%196)); got != idleMessageFECMatrix[r][c] {
				t.Errorf("Figure D.1 [%d][%d] = %d, bptc.Encode196 gives %d", r, c, idleMessageFECMatrix[r][c], got)
			}
		}
	}
}

// TestNullEmbeddedLC_IsZero checks that the single burst BPTC of 11 zero
// information bits is 32 zero transmit bits, the Null embedded message
// (ETSI TS 102 361-1 clause B.2.1, Table D.1).
func TestNullEmbeddedLC_IsZero(t *testing.T) {
	t.Parallel()

	var null [4]byte
	bits.SetField(null[:], 0, 32, uint64(bptc.EncodeSingleBurst(0, false)))
	if null != [4]byte{} {
		t.Errorf("single burst BPTC of 11 zero bits = %x, want 32 zero bits", null)
	}
}
