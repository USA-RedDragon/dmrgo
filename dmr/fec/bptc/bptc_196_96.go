package bptc

import "fmt"

type BPTC19696 struct {
}

// deinterleave matrix
//
//nolint:gochecknoglobals // lookup table shared across decoder instances
var dm = [256]uint8{
	0, 181, 166, 151, 136, 121, 106, 91, 76, 61, 46, 31, 16, 1, 182, 167,
	152, 137, 122, 107, 92, 77, 62, 47, 32, 17, 2, 183, 168, 153, 138, 123,
	108, 93, 78, 63, 48, 33, 18, 3, 184, 169, 154, 139, 124, 109, 94, 79,
	64, 49, 34, 19, 4, 185, 170, 155, 140, 125, 110, 95, 80, 65, 50, 35,
	20, 5, 186, 171, 156, 141, 126, 111, 96, 81, 66, 51, 36, 21, 6, 187,
	172, 157, 142, 127, 112, 97, 82, 67, 52, 37, 22, 7, 188, 173, 158, 143,
	128, 113, 98, 83, 68, 53, 38, 23, 8, 189, 174, 159, 144, 129, 114, 99,
	84, 69, 54, 39, 24, 9, 190, 175, 160, 145, 130, 115, 100, 85, 70, 55,
	40, 25, 10, 191, 176, 161, 146, 131, 116, 101, 86, 71, 56, 41, 26, 11,
	192, 177, 162, 147, 132, 117, 102, 87, 72, 57, 42, 27, 12, 193, 178, 163,
	148, 133, 118, 103, 88, 73, 58, 43, 28, 13, 194, 179, 164, 149, 134, 119,
	104, 89, 74, 59, 44, 29, 14, 195, 180, 165, 150, 135, 120, 105, 90, 75,
	60, 45, 30, 15, 0, 181, 166, 151, 136, 121, 106, 91, 76, 61, 46, 31,
	16, 1, 182, 167, 152, 137, 122, 107, 92, 77, 62, 47, 32, 17, 2, 183,
	168, 153, 138, 123, 108, 93, 78, 63, 48, 33, 18, 3, 184, 169, 154, 139,
	124, 109, 94, 79, 64, 49, 34, 19, 4, 185, 170, 155, 140, 125, 110, 95,
}

// Syndrome table for Hamming (15,11) code
// Maps 4-bit syndrome (0-15) to bit position (0-14). -1 indicates no error or uncorrectable
// The syndrome is calculated as [S0, S1, S2, S3] treated as an integer.
//
//nolint:gochecknoglobals // static lookup table
var hamming15_11_syndrome_table = [16]int{
	-1, // 0000
	11, // 0001
	12, // 0010
	8,  // 0011
	13, // 0100
	5,  // 0101
	9,  // 0110
	3,  // 0111
	14, // 1000
	0,  // 1001
	6,  // 1010
	1,  // 1011
	10, // 1100
	7,  // 1101
	4,  // 1110
	2,  // 1111
}

// Hamming (13,9) Syndrome Table
//
//nolint:gochecknoglobals // static lookup table
var hamming13_9_syndrome_table = [16]int{
	-1, // 0000
	9,  // 0001
	10, // 0010
	6,  // 0011
	11, // 0100
	3,  // 0101
	7,  // 0110
	1,  // 0111
	12, // 1000
	-1, // 1001
	4,  // 1010
	-1, // 1011
	8,  // 1100
	5,  // 1101
	2,  // 1110
	0,  // 1111
}

func calculate_syndrome_15_11(bits [15]byte) int {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	errs[1] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[12]
	errs[2] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10] ^ bits[13]
	errs[3] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10] ^ bits[14]
	// Convert [S0, S1, S2, S3] to integer
	return int(errs[0]) | int(errs[1])<<1 | int(errs[2])<<2 | int(errs[3])<<3
}

func calculate_syndrome_13_9(bits [13]byte) int {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6] ^ bits[9]
	errs[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]
	errs[2] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	errs[3] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8] ^ bits[12]
	return int(errs[0]) | int(errs[1])<<1 | int(errs[2])<<2 | int(errs[3])<<3
}

// DeinterleaveDataBits returns the deinterleaved data bits, error count, and uncorrectable flag
func (b *BPTC19696) DeinterleaveDataBits(bits [196]byte) ([96]byte, int, bool) {
	var deinterleavedBits [196]byte
	var temp [96]byte

	var i, j, k uint

	for i = 0; i < 196; i++ {
		deinterleavedBits[i] = bits[dm[i]]
	}

	correctedBits, errors, uncorrectable := hamming_correct(deinterleavedBits)
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

	return temp, errors, uncorrectable
}

func hamming_correct(bits [196]byte) ([196]byte, int, bool) {
	var row [15]byte
	var col [13]byte
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
func Encode(data [96]byte) [196]byte {
	var grid [196]byte

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
	var encoded [196]byte
	for i := 0; i < 196; i++ {
		encoded[dm[i]] = grid[i]
	}

	return encoded
}

func calculateRowParity(grid *[196]byte, r int) {
	k := r*15 + 1
	var bits [15]byte
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

func calculateColParity(grid *[196]byte, c int) {
	k := c + 1
	var bits [13]byte

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
