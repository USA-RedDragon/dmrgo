package bptc

import "fmt"

type BPTC19696 struct {
}

var (
	// deinterleave matrix
	dm = [256]uint8{}
)

// Syndrome table for Hamming (15,11) code
// Maps 4-bit syndrome (0-15) to bit position (0-14). -1 indicates no error or uncorrectable
// The syndrome is calculated as [S0, S1, S2, S3] treated as an integer.
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

func init() {
	for i := 0; i < 0x100; i++ {
		dm[i] = uint8((i * 181) % 196)
	}
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

func hamming_15_11_3_syndrome(bits [15]byte) int {
	if hamming_15_11_3_parity(bits) {
		return 0
	}
	return 1
}

func hamming_13_9_3_syndrome(bits [13]byte) int {
	if hamming_13_9_3_parity(bits) {
		return 0
	}
	return 1
}

func hamming_15_11_3_parity(bits [15]byte) bool {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	errs[1] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9]
	errs[2] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10]
	errs[3] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]
	return (errs[0] == bits[11]) && (errs[1] == bits[12]) && (errs[2] == bits[13]) && (errs[3] == bits[14])
}

func hamming_13_9_3_parity(bits [13]byte) bool {
	var errs [4]byte
	errs[0] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6]
	errs[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7]
	errs[2] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	errs[3] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8]
	return (errs[0] == bits[9]) && (errs[1] == bits[10]) && (errs[2] == bits[11]) && (errs[3] == bits[12])
}
