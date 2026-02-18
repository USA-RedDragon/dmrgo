package bptc

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/bit"
	"github.com/USA-RedDragon/dmrgo/dmr/fec"
)

type BPTC19696 struct {
}

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

// DeinterleaveDataBits returns the deinterleaved data bits and FECResult
func (b *BPTC19696) DeinterleaveDataBits(bits [196]bit.Bit) ([96]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 196}
	var deinterleavedBits [196]bit.Bit
	var temp [96]bit.Bit

	var i, j, k uint

	for i = 0; i < 196; i++ {
		deinterleavedBits[i] = bits[dm[i]]
	}

	correctedBits, errors, uncorrectable := hamming_correct(deinterleavedBits)
	result.ErrorsCorrected = errors
	result.Uncorrectable = uncorrectable
	if errors > 0 && !uncorrectable {
		deinterleavedBits = correctedBits
	}

	if uncorrectable {
		fmt.Printf("BPTC uncorrectable error\n")
	}

	// Extract data bits
	for i, k = 4, 0; i <= 11; i, k = i+1, k+1 {
		temp[k] = deinterleavedBits[i]
	}
	for j = 1; j < 9; j++ {
		for i = 0; i < 11; i, k = i+1, k+1 {
			temp[k] = deinterleavedBits[j*15+i+1]
		}
	}

	return temp, result
}

func hamming_correct(bits [196]bit.Bit) ([196]bit.Bit, int, bool) {
	var row [15]bit.Bit
	var col [13]bit.Bit
	totalErrors := 0
	uncorrectable := false
	corrected := bits

	// columns first, then rows, up to 5 passes.
	for pass := 0; pass < 5; pass++ {
		fixing := false

		// Run through each of the 15 columns (Hamming 13,9,3)
		for c := 0; c < 15; c++ {
			pos := c + 1
			for a := 0; a < 13; a++ {
				col[a] = corrected[pos]
				pos += 15
			}

			s := calculate_syndrome_13_9(col)
			if s != 0 {
				bitPos := hamming13_9_syndrome_table[s]
				if bitPos != -1 {
					idx := (c + 1) + (bitPos * 15)
					corrected[idx] ^= 1
					totalErrors++
					fixing = true
				} else {
					uncorrectable = true
				}
			}
		}

		// Run through each of the 9 rows containing data (Hamming 15,11,3)
		for r := 0; r < 9; r++ {
			k := r*15 + 1
			for a := 0; a < 15; a++ {
				row[a] = corrected[k+a]
			}

			s := calculate_syndrome_15_11(row)
			if s != 0 {
				bitPos := hamming15_11_syndrome_table[s]
				if bitPos != -1 {
					corrected[k+bitPos] ^= 1
					totalErrors++
					fixing = true
				} else {
					uncorrectable = true
				}
			}
		}

		if !fixing {
			break
		}
	}

	return corrected, totalErrors, uncorrectable
}

// Encode encodes 96 data bits using BPTC(196,96) interleaving with Hamming FEC.
func Encode(data [96]bit.Bit) [196]bit.Bit {
	var grid [196]bit.Bit

	// Place data into the grid
	dataIdx := 0

	// Row 0: indices 4..11 (positions 1-3 are R(0), R(1), R(2))
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
