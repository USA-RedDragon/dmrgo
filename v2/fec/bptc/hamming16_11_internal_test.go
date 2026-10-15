package bptc

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

// tableB16 is ETSI TS 102 361-1 Table B.16, the Hamming (16,11,4) generator matrix.
var tableB16 = [11]string{
	"1000000000010011",
	"0100000000011010",
	"0010000000011111",
	"0001000000011100",
	"0000100000001110",
	"0000010000010101",
	"0000001000001011",
	"0000000100010110",
	"0000000010011001",
	"0000000001001101",
	"0000000000100111",
}

func hamming16_11Codeword(d int) [16]bit.Bit {
	var data [11]bit.Bit
	for i := range data {
		data[i] = bit.Bit((d >> i) & 1)
	}
	var cw [16]bit.Bit
	copy(cw[:11], data[:])
	p := parityHamming16_11(data)
	copy(cw[11:], p[:])
	return cw
}

func TestHamming16_11_GeneratorMatchesTableB16(t *testing.T) {
	for row, want := range tableB16 {
		cw := hamming16_11Codeword(1 << row)
		for col := range cw {
			if byte(cw[col])+'0' != want[col] {
				t.Errorf("row %d: codeword %v, want %s", row, cw, want)
				break
			}
		}
		if s := calculateSyndrome16_11(cw); s != 0 {
			t.Errorf("row %d: syndrome %d, want 0", row, s)
		}
	}
}

func TestHamming16_11_MinimumDistance(t *testing.T) {
	minWeight := 16
	for d := 1; d < 1<<11; d++ {
		cw := hamming16_11Codeword(d)
		if s := calculateSyndrome16_11(cw); s != 0 {
			t.Fatalf("data %011b: syndrome %d, want 0", d, s)
		}
		w := 0
		for _, b := range cw {
			w += int(b)
		}
		minWeight = min(minWeight, w)
	}
	if minWeight != 4 {
		t.Errorf("minimum distance %d, want 4", minWeight)
	}
}

func TestHamming16_11_SyndromeTableLocatesSingleErrors(t *testing.T) {
	for d := 0; d < 1<<11; d += 37 {
		for pos := 0; pos < 16; pos++ {
			cw := hamming16_11Codeword(d)
			cw[pos] ^= 1
			s := calculateSyndrome16_11(cw)
			if got := hamming16_11_syndrome_table[s]; got != pos {
				t.Fatalf("data %011b error at %d: syndrome %d maps to %d", d, pos, s, got)
			}
		}
	}
}
