package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// ETSI TS 102 361-1 Figure D.1

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

// ETSI TS 102 361-1 Figure B.1, Figure D.1
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

// ETSI TS 102 361-1 clause B.1.1, Figure D.1
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

// ETSI TS 102 361-1 clause B.2.1, Table D.1
func TestNullEmbeddedLC_IsZero(t *testing.T) {
	t.Parallel()

	var null [4]byte
	bits.SetField(null[:], 0, 32, uint64(bptc.EncodeSingleBurst(0, false)))
	if null != [4]byte{} {
		t.Errorf("single burst BPTC of 11 zero bits = %x, want 32 zero bits", null)
	}
}
