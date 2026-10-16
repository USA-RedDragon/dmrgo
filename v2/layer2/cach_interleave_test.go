package layer2_test

import (
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/bits"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestCACHInterleave_RoundTrip(t *testing.T) {
	t.Parallel()
	for pos := 0; pos < layer2.CACHBits; pos++ {
		var logical [3]byte
		bits.SetBit(logical[:], pos, 1)

		tx := layer2.CACHInterleave(logical)
		recovered := layer2.CACHDeinterleave(tx)

		for i := 0; i < layer2.CACHBits; i++ {
			if bits.Bit(recovered[:], i) != bits.Bit(logical[:], i) {
				t.Errorf("pos=%d: bit %d: got %d, want %d", pos, i, bits.Bit(recovered[:], i), bits.Bit(logical[:], i))
			}
		}
	}
}

func TestCACHDeinterleave_RoundTrip(t *testing.T) {
	t.Parallel()
	for pos := 0; pos < layer2.CACHBits; pos++ {
		var tx [3]byte
		bits.SetBit(tx[:], pos, 1)

		logical := layer2.CACHDeinterleave(tx)
		recovered := layer2.CACHInterleave(logical)

		for i := 0; i < layer2.CACHBits; i++ {
			if bits.Bit(recovered[:], i) != bits.Bit(tx[:], i) {
				t.Errorf("pos=%d: bit %d: got %d, want %d", pos, i, bits.Bit(recovered[:], i), bits.Bit(tx[:], i))
			}
		}
	}
}

func TestCACHInterleave_TACTSpread(t *testing.T) {
	t.Parallel()
	var logical [3]byte
	logical[0] = 0xFE

	tx := layer2.CACHInterleave(logical)

	contiguous := 0
	for i := 0; i < layer2.CACHBits; i++ {
		if bits.Bit(tx[:], i) == 1 {
			contiguous++
		} else {
			break
		}
	}

	if contiguous > 2 {
		t.Errorf("TACT bits not spread: %d contiguous at start of tx", contiguous)
	}

	total := 0
	for i := 0; i < layer2.CACHBits; i++ {
		if bits.Bit(tx[:], i) == 1 {
			total++
		}
	}
	if total != 7 {
		t.Errorf("expected 7 set bits, got %d", total)
	}
}

func TestCACHInterleave_ATPosition(t *testing.T) {
	t.Parallel()
	var logical [3]byte
	bits.SetBit(logical[:], 0, 1)

	tx := layer2.CACHInterleave(logical)
	if bits.Bit(tx[:], 0) != 1 {
		t.Errorf("AT should be at tx[0] (TX(23), first sent), but tx[0]=%d", bits.Bit(tx[:], 0))
	}
}

// TestCACHInterleave_FigureB9 checks every position against the figure,
// where the transmit row reads TX(23), sent first, to TX(0) (ETSI TS 102
// 361-1 Figure B.9).
func TestCACHInterleave_FigureB9(t *testing.T) {
	t.Parallel()
	txRow := []string{
		"AT", "P16", "P15", "P14", "TC", "P13", "P12", "P11", "LS1", "P10", "P9", "P8",
		"LS0", "P7", "H2", "P6", "P5", "P4", "H1", "P3", "P2", "P1", "H0", "P0",
	}
	logicalRow := []string{"AT", "TC", "LS1", "LS0", "H2", "H1", "H0"}
	for p := 16; p >= 0; p-- {
		logicalRow = append(logicalRow, fmt.Sprintf("P%d", p))
	}

	for l := 0; l < layer2.CACHBits; l++ {
		var logical [3]byte
		bits.SetBit(logical[:], l, 1)
		tx := layer2.CACHInterleave(logical)
		for i := 0; i < layer2.CACHBits; i++ {
			if bits.Bit(tx[:], i) == 1 && txRow[i] != logicalRow[l] {
				t.Errorf("logical %s landed at tx[%d] (%s)", logicalRow[l], i, txRow[i])
			}
		}
	}
}

func TestCACHDeinterleave_PreservesData(t *testing.T) {
	t.Parallel()
	var logical [3]byte
	bits.SetBit(logical[:], 0, 1)
	bits.SetBit(logical[:], 2, 1)
	bits.SetBit(logical[:], 4, 1)
	bits.SetBit(logical[:], 6, 1)
	bits.SetBit(logical[:], 23, 1)

	tx := layer2.CACHInterleave(logical)
	recovered := layer2.CACHDeinterleave(tx)

	if bits.Bit(recovered[:], 0) != 1 {
		t.Error("AT not recovered")
	}
	if bits.Bit(recovered[:], 2) != 1 {
		t.Error("LS(1) not recovered")
	}
	if bits.Bit(recovered[:], 4) != 1 {
		t.Error("H(2) not recovered")
	}
	if bits.Bit(recovered[:], 6) != 1 {
		t.Error("H(0) not recovered")
	}
	if bits.Bit(recovered[:], 23) != 1 {
		t.Error("P(0) not recovered")
	}

	for i := 0; i < layer2.CACHBits; i++ {
		if i == 0 || i == 2 || i == 4 || i == 6 || i == 23 {
			continue
		}
		if bits.Bit(recovered[:], i) != 0 {
			t.Errorf("bit %d should be 0, got %d", i, bits.Bit(recovered[:], i))
		}
	}
}

func TestCACHDeinterleave_WithDecodeCACH(t *testing.T) {
	t.Parallel()
	cach := layer2.CACH{}
	cach.TACT.AccessType = true
	cach.TACT.TDMAChannel = false
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		bits.SetBit(cach.Payload[:], i, byte(i%2))
	}

	logical := must(cach.Encode())
	tx := layer2.CACHInterleave(logical)
	deinterleaved := layer2.CACHDeinterleave(tx)
	decoded := layer2.DecodeCACH(deinterleaved)

	if decoded.TACT.AccessType != true {
		t.Error("AccessType not recovered")
	}
	if decoded.TACT.TDMAChannel != false {
		t.Error("TDMAChannel should be false")
	}
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		if got := bits.Bit(decoded.Payload[:], i); got != byte(i%2) {
			t.Errorf("payload bit %d: got %d, want %d", i, got, i%2)
		}
	}
}

func TestCACHInterleave_InverseConsistency(t *testing.T) {
	t.Parallel()
	for i := 0; i < layer2.CACHBits; i++ {
		var logical [3]byte
		bits.SetBit(logical[:], i, 1)

		tx := layer2.CACHInterleave(logical)

		count := 0
		txPos := -1
		for j := 0; j < layer2.CACHBits; j++ {
			if bits.Bit(tx[:], j) == 1 {
				count++
				txPos = j
			}
		}
		if count != 1 {
			t.Errorf("logical[%d]: expected 1 set bit in tx, got %d", i, count)
			continue
		}

		var tx2 [3]byte
		bits.SetBit(tx2[:], txPos, 1)
		logical2 := layer2.CACHDeinterleave(tx2)

		if bits.Bit(logical2[:], i) != 1 {
			t.Errorf("logical[%d] -> tx[%d] -> logical[%d] should be 1", i, txPos, i)
		}
	}
}
