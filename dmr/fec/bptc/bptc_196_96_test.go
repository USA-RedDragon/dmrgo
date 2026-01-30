package bptc

import (
	"math/rand"
	"testing"
)

// Helper to encode BPTC 196,96
// Matches the logic inferred from DeinterleaveDataBits and hamming_correct.
func encodeBPTC19696(data [96]byte) [196]byte {
	var grid [196]byte

	// 1. Place Data into Grid
	// Reverse of DeinterleaveDataBits extraction
	// Row 0: bits 3..10 (indices 3..10 in grid?)
	// Note: DeinterleaveDataBits uses 1-based logic implicitly via loop or what?
	// Code: `deinterleavedBits[0*15+i]` for i=3..10. -> Grid Indices 3..10.
	// Code: `deinterleavedBits[j*15+i]`. j=1..8. i=0..10.

	dataIdx := 0

	// Row 0
	for i := 3; i <= 10; i++ {
		grid[0*15+i] = data[dataIdx]
		dataIdx++
	}

	// Rows 1..8
	for j := 1; j < 9; j++ {
		for i := 0; i <= 10; i++ {
			grid[j*15+i+1] = data[dataIdx]
			dataIdx++
		}
	}

	// 2. Compute Row Parity (Hamming 15,11) for ALL Rows (0..12)??
	// Wait, Rows 9..12 are not populated yet. They are Column Parity.
	// Strategy:
	// A. Fill Data Rows (0..8)
	// B. Compute Row Parity for Data Rows (0..8).
	// C. Compute Column Parity for ALL Columns (0..14). This fills Rows 9..12.
	// D. Compute Row Parity for Parity Rows (9..12) ???
	//    The bottom-right corner (Parity of Parity) bits need to satisfy both?
	//    Usually computing Col Parity on the Row Parity columns is sufficient.
	//    Let's try: Row(0..8) -> Col(0..14).

	// Compute Row Parity for Rows 0..8
	for r := 0; r < 9; r++ {
		// Row r is indices r*15+1 .. r*15+15 in "Hamming View"
		// In "Grid View" (0-based array), it seems to be r*15 .. r*15+14?
		// Unless the +1 in hamming_correct implies Grid index 0 is skipped.
		// If Grid[3] was accessed as a=2 (3rd element), then Grid[1] is a=0.
		// So Grid[1..15] is Row 0.
		// Grid[16..30] is Row 1.
		// ...
		// Grid[0] is unused.
		// So my placement above `grid[0*15+i]` put data at `grid[3]`.
		// `grid[3]` is 3rd element of Row 0 (1..15). a=2.
		// Matches.

		calculateRowParity(&grid, r)
	}

	// Compute Column Parity for Cols 0..14
	for c := 0; c < 15; c++ {
		calculateColParity(&grid, c)
	}

	// Interleave
	var encoded [196]byte
	for i := 1; i < 197; i++ {
		// deinterleavedBits[i-1] = bits[dm[i]]
		// grid[i-1] = encoded[dm[i]]
		// so encoded[dm[i]] = grid[i-1]
		encoded[dm[i]] = grid[i-1]
	}

	return encoded
}

func calculateRowParity(grid *[196]byte, r int) {
	// Hamming (15,11) systematic.
	// Parity bits at 11, 12, 13, 14 (0-based index in row).
	// i.e. bits 12, 13, 14, 15 (1-based).
	// Input bits 0..10 are Data.

	k := r*15 + 1 // Start of row in Grid
	var bits [15]byte
	for a := 0; a < 11; a++ {
		bits[a] = grid[k+a]
	}

	// Calculate Parity to satisfy `hamming_15_11_3_parity`
	// errs[0] = bits[0]^... must equal bits[11]
	// So bits[11] = errs[0] calculated from bits[0..10].
	// Note: parity checks in `hamming_15_11_3_parity` include the parity bit itself?
	// `errs[0] = ... ^ bits[8]`. Does not include 11.
	// `return errs[0] == bits[11]`.
	// So `bits[11] = errs[0]`.

	// Compute P0 (bits[11])
	bits[11] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]

	// Compute P1 (bits[12])
	bits[12] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9]

	// Compute P2 (bits[13])
	bits[13] = bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10]

	// Compute P3 (bits[14])
	bits[14] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10]

	// Write back
	for a := 11; a < 15; a++ {
		grid[k+a] = bits[a]
	}
}

func calculateColParity(grid *[196]byte, c int) {
	// Hamming (13,9) systematic.
	// Parity bits at 9, 10, 11, 12 (0-based in col).
	// Col has 13 bits.

	k := c + 1 // Start of col in Grid (Row 0)
	var bits [13]byte

	// Read Data (0..8)
	for a := 0; a < 9; a++ {
		// Stride 15
		// a=0: Row 0. k
		// a=1: Row 1. k+15
		bits[a] = grid[k+a*15]
	}

	// Compute Parity
	// `hamming_13_9_3_parity`
	// bits[9] = errs[0]

	bits[9] = bits[0] ^ bits[1] ^ bits[3] ^ bits[5] ^ bits[6]
	bits[10] = bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7]
	bits[11] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8]
	bits[12] = bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[8]

	// Write back
	for a := 9; a < 13; a++ {
		grid[k+a*15] = bits[a]
	}
}

func TestBPTC19696Decode(t *testing.T) {
	bptcDiff := BPTC19696{}

	for run := 0; run < 100; run++ {
		// 1. Generate Random Data
		var data [96]byte
		for i := 0; i < 96; i++ {
			data[i] = byte(rand.Intn(2))
		}

		// 2. Encode
		encoded := encodeBPTC19696(data)

		// 3. Decode Clean
		decoded, errs, unc := bptcDiff.DeinterleaveDataBits(encoded)

		if unc {
			t.Errorf("Failed to decode clean codeword: uncorrectable")
		}
		// BPTC decoder runs single pass in current impl?
		// We expect 0 errors corrected if clean.
		// Note: The loop count `errors` is incremented when bit flipped.
		// If clean, 0.
		if errs != 0 {
			t.Errorf("Decoded clean codeword with %d errors", errs)
		}

		if decoded != data {
			t.Errorf("Decoded payload mismatch")
		}

		// 4. Test Single Error
		// Flip a bit in the encoded stream
		// We should pick a bit that maps to Data or Parity.
		// Note: If we flip a bit, DeinterleaveDataBits returns correct data.
		encodedErr := encoded
		pos := rand.Intn(196)
		for pos == int(dm[1]) { // dm[1] maps to the unused grid slot; flipping it is unobservable
			pos = rand.Intn(196)
		}
		encodedErr[pos] ^= 1

		decoded, errs, unc = bptcDiff.DeinterleaveDataBits(encodedErr)

		if unc {
			t.Errorf("Failed to correct 1 error")
		}

		// Note: `errs` returned by DeinterleaveDataBits is "how many flips it performed".
		// It should be 1.
		if errs != 1 {
			t.Errorf("Expected 1 error corrected, got %d", errs)
		}

		if decoded != data {
			t.Errorf("Decoded 1-error payload mismatch")
		}

		// 5. Test Uncorrectable?
		// BPTC (196,96) has large distance.
		// But decoder might satisfy simple checks.
		// Let's not stress test uncorrectable here, just basic functionality.
	}
}
