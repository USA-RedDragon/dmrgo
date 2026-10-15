// Package bptc implements the block product turbo codes of ETSI TS 102 361-1
// B.1 and B.2: BPTC(196,96) for full bursts, the BPTC(128,77) matrix of the
// embedded signalling, the CACH Short LC BPTC and the single burst BPTC.
package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

func calculate_syndrome_15_11(bits [15]bit.Bit) int {
	var errs [4]bit.Bit
	errs[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	errs[1] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[12]
	errs[2] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10] ^ bits[13]
	errs[3] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10] ^ bits[14]
	// Convert [S0, S1, S2, S3] to integer
	return int(errs[0]) | int(errs[1])<<1 | int(errs[2])<<2 | int(errs[3])<<3
}

func calculate_syndrome_13_9(bits [13]bit.Bit) int {
	var errs [4]bit.Bit
	errs[0] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6] ^ bits[9]
	errs[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]
	errs[2] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	errs[3] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8] ^ bits[12]
	return int(errs[0]) | int(errs[1])<<1 | int(errs[2])<<2 | int(errs[3])<<3
}

// Decode196 deinterleaves a BPTC(196,96) block, corrects it with the row and
// column Hamming codes and returns the 96 data bits and a fec.Result
// (ETSI TS 102 361-1 B.1.1).
func Decode196(bits [196]bit.Bit) ([96]bit.Bit, fec.Result) {
	data, _, result := Decode196WithReserved(bits)
	return data, result
}

// Decode196WithReserved decodes a BPTC(196,96) block as Decode196 does and
// also returns the corrected reserved bits R(0) to R(2), with R(i) at index
// i. R(3) has no FEC and is not returned.
func Decode196WithReserved(bits [196]bit.Bit) ([96]bit.Bit, [3]bit.Bit, fec.Result) {
	lowbit.Mask(bits[:])
	result := fec.Result{BitsChecked: 196}
	var deinterleavedBits [196]bit.Bit
	var temp [96]bit.Bit

	var i, j, k uint

	for i = 0; i < 196; i++ {
		deinterleavedBits[i] = bits[dm[i]]
	}

	deinterleavedBits, result.ErrorsCorrected, result.Uncorrectable = hamming_correct(deinterleavedBits)

	// Extract data bits
	for i, k = 4, 0; i <= 11; i, k = i+1, k+1 {
		temp[k] = deinterleavedBits[i]
	}
	for j = 1; j < 9; j++ {
		for i = 0; i < 11; i, k = i+1, k+1 {
			temp[k] = deinterleavedBits[j*15+i+1]
		}
	}

	reserved := [3]bit.Bit{deinterleavedBits[3], deinterleavedBits[2], deinterleavedBits[1]}
	return temp, reserved, result
}

// hamming_correct iteratively corrects the BPTC(196,96) grid with the column
// Hamming(13,9,3) and row Hamming(15,11,3) codes. The four column parity
// rows are also row codewords, because the column code is applied to all 15
// columns, so all 13 rows are row-decoded. Each pass corrects the columns
// first, as MMDVMHost does: with random errors this corrects more grids than
// rows first and does not miscorrect more often. It returns the corrected
// grid, the number of bits that differ from the input, and whether any row or
// column syndrome is still nonzero after correction.
func hamming_correct(bits [196]bit.Bit) ([196]bit.Bit, int, bool) {
	corrected := bits

	for pass := 0; pass < 5; pass++ {
		fixing := false

		for c := 0; c < 15; c++ {
			if bitPos := hamming13_9_syndrome_table[columnSyndrome(&corrected, c)]; bitPos != -1 {
				corrected[(c+1)+(bitPos*15)] ^= 1 //nolint:gosec // c∈[0,14], bitPos∈[0,12], max idx=195 < 196
				fixing = true
			}
		}

		for r := 0; r < 13; r++ {
			if bitPos := hamming15_11_syndrome_table[rowSyndrome(&corrected, r)]; bitPos != -1 {
				corrected[r*15+1+bitPos] ^= 1 //nolint:gosec // r∈[0,12], bitPos∈[0,14], max idx=195 < 196
				fixing = true
			}
		}

		if !fixing {
			break
		}
	}

	uncorrectable := false
	for r := 0; r < 13 && !uncorrectable; r++ {
		uncorrectable = rowSyndrome(&corrected, r) != 0
	}
	for c := 0; c < 15 && !uncorrectable; c++ {
		uncorrectable = columnSyndrome(&corrected, c) != 0
	}

	flips := 0
	for i := range corrected {
		flips += int(corrected[i] ^ bits[i])
	}

	return corrected, flips, uncorrectable
}

func columnSyndrome(grid *[196]bit.Bit, c int) int {
	var col [13]bit.Bit
	for a := range col {
		col[a] = grid[c+1+a*15]
	}
	return calculate_syndrome_13_9(col)
}

func rowSyndrome(grid *[196]bit.Bit, r int) int {
	var row [15]bit.Bit
	copy(row[:], grid[r*15+1:r*15+16])
	return calculate_syndrome_15_11(row)
}

// Encode196 encodes 96 data bits using BPTC(196,96) interleaving with
// Hamming FEC. The reserved bits R(0) to R(3) are zero.
func Encode196(data [96]bit.Bit) [196]bit.Bit {
	return Encode196WithReserved(data, [3]bit.Bit{})
}

// Encode196WithReserved encodes 96 data bits as Encode196 does, with the
// reserved bits R(0) to R(2) from reserved, R(i) at index i. R(3) is zero.
func Encode196WithReserved(data [96]bit.Bit, reserved [3]bit.Bit) [196]bit.Bit {
	lowbit.Mask(data[:])
	lowbit.Mask(reserved[:])
	grid := [196]bit.Bit{1: reserved[2], 2: reserved[1], 3: reserved[0]}

	// Place data into the grid
	dataIdx := 0

	// Row 0: indices 4..11 (positions 1-3 are R(2), R(1), R(0))
	for i := 4; i <= 11; i++ {
		grid[i] = data[dataIdx]
		dataIdx++
	}

	// Rows 1..8
	for j := 1; j < 9; j++ {
		for i := 0; i <= 10; i++ {
			grid[j*15+i+1] = data[dataIdx]
			dataIdx++
		}
	}

	// Compute Row Parity (Hamming 15,11) for Rows 0..8
	for r := 0; r < 9; r++ {
		calculateRowParity(&grid, r)
	}

	// Compute Column Parity (Hamming 13,9) for Columns 0..14
	for c := 0; c < 15; c++ {
		calculateColParity(&grid, c)
	}

	// Interleave using the deinterleave matrix in reverse
	var encoded [196]bit.Bit
	for i := 0; i < 196; i++ {
		encoded[dm[i]] = grid[i]
	}

	return encoded
}

func calculateRowParity(grid *[196]bit.Bit, r int) {
	k := r*15 + 1
	var bits [15]bit.Bit
	for a := 0; a < 11; a++ {
		bits[a] = grid[k+a]
	}

	bits[11] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	bits[12] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9]
	bits[13] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10]
	bits[14] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]

	for a := 11; a < 15; a++ {
		grid[k+a] = bits[a]
	}
}

func calculateColParity(grid *[196]bit.Bit, c int) {
	k := c + 1
	var bits [13]bit.Bit

	for a := 0; a < 9; a++ {
		bits[a] = grid[k+a*15]
	}

	bits[9] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6]
	bits[10] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7]
	bits[11] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	bits[12] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8]

	for a := 9; a < 13; a++ {
		grid[k+a*15] = bits[a]
	}
}
