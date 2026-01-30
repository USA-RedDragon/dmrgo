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

	for i = 1; i < 197; i++ {
		deinterleavedBits[i-1] = bits[dm[i]]
	}

	correctedBits, errors, uncorrectable := hamming_correct(deinterleavedBits)
	if errors > 0 && !uncorrectable {
		deinterleavedBits = correctedBits
	}

	if uncorrectable {
		fmt.Printf("BPTC uncorrectable error\n")
	}

	// Extract data bits
	for i, k = 3, 0; i < 11; i, k = i+1, k+1 {
		temp[k] = deinterleavedBits[0*15+i]
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

	// We run multiple passes? BPTC is iterative.
	// For "Turbo" decoding we usually iterate.
	// But the spec says: "The decoding process is performed by decoding the rows and columns iteratively."
	// Let's do a few passes? Or just one pass of rows then cols?
	// The problem mentions "brute force syndrome" previously which implies simple Hamming.
	// Let's stick to Row then Column for now, as implemented before, but use the lookup table.

	// Run through each of the 9 rows containing data
	for r := 0; r < 9; r++ {
		k := r*15 + 1
		for a := 0; a < 15; a++ {
			row[a] = corrected[k+a]
		}

		s := calculate_syndrome_15_11(row)
		if s != 0 {
			pos := hamming15_11_syndrome_table[s]
			if pos != -1 {
				corrected[k+pos] ^= 1
				totalErrors++
			} else {
				// Multiple errors in row, uncorrectable by simple Hamming
				// In Turbo codes, this might be fixed by column pass later.
				// But we flag it potentially.
				// For now, let's mark uncorrectable only if we can't fix it after all passes?
				// But strict Hamming 15,11 is single error correcting.
				uncorrectable = true // Tentative
			}
		}
	}

	// Run through each of the 15 columns
	for c := 0; c < 15; c++ {
		k := c + 1
		for a := 0; a < 13; a, k = a+1, k+15 {
			col[a] = corrected[k]
		}

		s := calculate_syndrome_13_9(col)
		if s != 0 {
			pos := hamming13_9_syndrome_table[s]
			if pos != -1 {
				// Re-calculate index in 'corrected' array
				// col[pos] corresponds to which index?
				// Loop logic: a=pos, k started at c+1 and incremented by 15 'pos' times.
				idx := (c + 1) + (pos * 15)
				corrected[idx] ^= 1
				totalErrors++
			} else {
				uncorrectable = true
			}
		}
	}

	return corrected, totalErrors, uncorrectable
}
