package bptc

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// tableB16 returns ETSI TS 102 361-1 Table B.16, the Hamming (16,11,4) generator matrix.
func tableB16() [11]string {
	return [11]string{
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
}

func hamming16_11Codeword(d int) [16]unpacked.Bit {
	row := rowParity16_11(uint16(d & 0x7FF))
	var cw [16]unpacked.Bit
	for i := range cw {
		cw[i] = unpacked.Bit(row >> i & 1)
	}
	return cw
}

// syndrome16_11 returns the syndrome of a row given one unpacked.Bit per column.
func syndrome16_11(cw [16]unpacked.Bit) int {
	var row uint16
	for i, b := range cw {
		row |= uint16(b&1) << i
	}
	return rowSyndrome16_11(row)
}

func TestHamming16_11_GeneratorMatchesTableB16(t *testing.T) {
	t.Parallel()

	for row, want := range tableB16() {
		cw := hamming16_11Codeword(1 << row)
		for col := range cw {
			if byte(cw[col])+'0' != want[col] {
				t.Errorf("row %d: codeword %v, want %s", row, cw, want)
				break
			}
		}
		if s := syndrome16_11(cw); s != 0 {
			t.Errorf("row %d: syndrome %d, want 0", row, s)
		}
	}
}

func TestHamming16_11_MinimumDistance(t *testing.T) {
	t.Parallel()

	minWeight := 16
	for d := 1; d < 1<<11; d++ {
		cw := hamming16_11Codeword(d)
		if s := syndrome16_11(cw); s != 0 {
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
	t.Parallel()

	for d := 0; d < 1<<11; d += 37 {
		for pos := 0; pos < 16; pos++ {
			cw := hamming16_11Codeword(d)
			cw[pos] ^= 1
			s := syndrome16_11(cw)
			if got := hamming16_11_syndrome_table[s]; got != pos {
				t.Fatalf("data %011b error at %d: syndrome %d maps to %d", d, pos, s, got)
			}
		}
	}
}
