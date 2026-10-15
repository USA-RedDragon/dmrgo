package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 §B.2.1 — Variable length BPTC for Embedded Signalling
//
// The embedded signalling uses a BPTC with Hamming(16,11,4) row codes and
// simple (even) parity column codes. For a full embedded LC:
//
// Encode matrix: 8 rows × 16 columns = 128 bits
//   - Rows 0..6: 11 data bits + 5 Hamming(16,11,4) parity bits
//   - Row 7: column parity (even parity per column)
//
// Info bits: 7×11 = 77 bits (72 LC + 5 checksum)
//
// The 128 bits are interleaved column-by-column into 4 fragments of 32 bits
// each, transmitted across 4 consecutive voice bursts.
//
// Fragment assembly: columns are read top-to-bottom, left-to-right,
// and split into 4 groups of 32 bits (columns 0-3, 4-7, 8-11, 12-15).

// DecodeEmbeddedLC decodes a full embedded LC from 4 × 32-bit fragments.
// The fragments should be in order (fragment 0 from voice burst B, through
// fragment 3 from voice burst E of a superframe).
//
// Returns the 77 information bits (72 LC + 5 checksum) and FEC result. The
// bits are rows 0..6, columns 0..10 of the encode matrix in row-major order,
// so per Figure B.3 the checksum bits are the last bit of rows 2..6.
func DecodeEmbeddedLC(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.FECResult) {
	matrix := embeddedLCMatrix(fragments)
	received := matrix
	correctEmbeddedLCMatrix(&matrix)
	result := embeddedLCResult(&matrix, &received)
	result.Uncorrectable = !embeddedLCMatrixValid(&matrix)
	return embeddedLCInfo(&matrix), result
}

// DecodeEmbeddedLCStrict decodes a full embedded LC as MMDVMHost
// (CDMREmbeddedData::decodeEmbeddedData) does. Each of rows 0..6 is
// corrected for at most one error with Hamming(16,11,4), and a row with a
// detected double error makes the LC uncorrectable. Row 7 is not corrected:
// the LC is uncorrectable when any column parity over all 8 rows fails. It
// accepts far fewer LCs with bit errors than DecodeEmbeddedLC.
func DecodeEmbeddedLCStrict(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.FECResult) {
	matrix := embeddedLCMatrix(fragments)
	received := matrix
	uncorrectable := false
	for row := 0; row < 7 && !uncorrectable; row++ {
		syndrome := calculateSyndrome16_11(matrix[row])
		if syndrome == 0 {
			continue
		}
		if errPos := hamming16_11_syndrome_table[syndrome]; errPos >= 0 {
			matrix[row][errPos] ^= 1
		} else {
			uncorrectable = true
		}
	}
	for col := 0; col < 16 && !uncorrectable; col++ {
		uncorrectable = columnParity(&matrix, col) != 0
	}
	result := embeddedLCResult(&matrix, &received)
	result.Uncorrectable = uncorrectable
	return embeddedLCInfo(&matrix), result
}

// embeddedLCMatrix reassembles the 8×16 encode matrix from the 4 fragments.
// The interleaving writes the columns top to bottom, left to right.
func embeddedLCMatrix(fragments [4][32]bit.Bit) [8][16]bit.Bit {
	var matrix [8][16]bit.Bit
	for col := 0; col < 16; col++ {
		for row := 0; row < 8; row++ {
			n := col*8 + row
			matrix[row][col] = fragments[n/32][n%32]
		}
	}
	return matrix
}

func embeddedLCResult(matrix, received *[8][16]bit.Bit) fec.FECResult {
	result := fec.FECResult{BitsChecked: 128}
	for row := range matrix {
		for col := range matrix[row] {
			result.ErrorsCorrected += int(matrix[row][col] ^ received[row][col])
		}
	}
	return result
}

// embeddedLCInfo returns the 77 information bits, rows 0..6 and columns
// 0..10 of the matrix in row-major order.
func embeddedLCInfo(matrix *[8][16]bit.Bit) [77]bit.Bit {
	var info [77]bit.Bit
	for row := 0; row < 7; row++ {
		copy(info[row*11:row*11+11], matrix[row][:11])
	}
	return info
}

// correctEmbeddedLCMatrix corrects the 8×16 product code in place. Row 7 is
// the even column parity of rows 0..6, so it is also a Hamming(16,11,4)
// codeword and all 8 rows are row-decoded. When only one row has a nonzero
// syndrome, the failing column parities give the exact error positions in
// that row. This runs before single-error row decoding, which would turn a
// row with 3 errors into a wrong codeword. Otherwise each row is corrected
// for a single error.
func correctEmbeddedLCMatrix(matrix *[8][16]bit.Bit) {
	for pass := 0; pass < 3; pass++ {
		var syndromes [8]int
		badRow, badRows := 0, 0
		for row := range matrix {
			syndromes[row] = calculateSyndrome16_11(matrix[row])
			if syndromes[row] != 0 {
				badRow = row
				badRows++
			}
		}

		fixing := false
		if badRows == 1 {
			for col := 0; col < 16; col++ {
				if columnParity(matrix, col) != 0 {
					matrix[badRow][col] ^= 1
					fixing = true
				}
			}
		}
		if !fixing {
			for row := range matrix {
				if syndromes[row] == 0 {
					continue
				}
				if errPos := hamming16_11_syndrome_table[syndromes[row]]; errPos >= 0 {
					matrix[row][errPos] ^= 1
					fixing = true
				}
			}
		}

		if !fixing {
			return
		}
	}
}

func columnParity(matrix *[8][16]bit.Bit, col int) bit.Bit {
	var parity bit.Bit
	for row := range matrix {
		parity ^= matrix[row][col]
	}
	return parity
}

func embeddedLCMatrixValid(matrix *[8][16]bit.Bit) bool {
	for row := range matrix {
		if calculateSyndrome16_11(matrix[row]) != 0 {
			return false
		}
	}
	for col := 0; col < 16; col++ {
		if columnParity(matrix, col) != 0 {
			return false
		}
	}
	return true
}

// EncodeEmbeddedLC encodes 77 information bits (72 LC + 5 checksum) into
// 4 × 32-bit fragments for transmission across 4 voice bursts.
func EncodeEmbeddedLC(info [77]bit.Bit) [4][32]bit.Bit {
	var matrix [8][16]bit.Bit

	// Place 77 info bits into rows 0..6, columns 0..10
	idx := 0
	for row := 0; row < 7; row++ {
		for col := 0; col < 11; col++ {
			matrix[row][col] = info[idx]
			idx++
		}
	}

	// Compute row Hamming(16,11,4) parity for rows 0..6
	for row := 0; row < 7; row++ {
		var data [11]bit.Bit
		copy(data[:], matrix[row][:11])
		parity := parityHamming16_11(data)
		matrix[row][11] = parity[0]
		matrix[row][12] = parity[1]
		matrix[row][13] = parity[2]
		matrix[row][14] = parity[3]
		matrix[row][15] = parity[4]
	}

	// Compute column parity (even) for row 7
	for col := 0; col < 16; col++ {
		var parity bit.Bit
		for row := 0; row < 7; row++ {
			parity ^= matrix[row][col]
		}
		matrix[7][col] = parity
	}

	// Interleave: read columns top-to-bottom, left-to-right → 128-bit stream
	var stream [128]bit.Bit
	for col := 0; col < 16; col++ {
		for row := 0; row < 8; row++ {
			stream[col*8+row] = matrix[row][col]
		}
	}

	// Split into 4 × 32-bit fragments
	var fragments [4][32]bit.Bit
	for i := 0; i < 4; i++ {
		copy(fragments[i][:], stream[i*32:(i+1)*32])
	}

	return fragments
}

// calculateSyndrome16_11 computes the 5-bit syndrome for a 16-bit
// Hamming(16,11,4) codeword.
//
// Syndrome equations from the H matrix (§B.3.4):
//
//	s₀ = r₀⊕r₁⊕r₂⊕r₃⊕r₅⊕r₇⊕r₈⊕r₁₁
//	s₁ = r₁⊕r₂⊕r₃⊕r₄⊕r₆⊕r₈⊕r₉⊕r₁₂
//	s₂ = r₂⊕r₃⊕r₄⊕r₅⊕r₇⊕r₉⊕r₁₀⊕r₁₃
//	s₃ = r₀⊕r₁⊕r₂⊕r₄⊕r₆⊕r₇⊕r₁₀⊕r₁₄
//	s₄ = r₀⊕r₂⊕r₅⊕r₆⊕r₈⊕r₉⊕r₁₀⊕r₁₅
func calculateSyndrome16_11(bits [16]bit.Bit) int {
	s0 := bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	s1 := bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[12]
	s2 := bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10] ^ bits[13]
	s3 := bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10] ^ bits[14]
	s4 := bits[0] ^ bits[2] ^ bits[5] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[10] ^ bits[15]
	return int(s0) | int(s1)<<1 | int(s2)<<2 | int(s3)<<3 | int(s4)<<4
}

// parityHamming16_11 computes the 5 Hamming(16,11,4) parity bits for 11 data bits.
//
// From the generator matrix (§B.3.4, Table B.16):
//
//	p₀ = d₀⊕d₁⊕d₂⊕d₃⊕d₅⊕d₇⊕d₈
//	p₁ = d₁⊕d₂⊕d₃⊕d₄⊕d₆⊕d₈⊕d₉
//	p₂ = d₂⊕d₃⊕d₄⊕d₅⊕d₇⊕d₉⊕d₁₀
//	p₃ = d₀⊕d₁⊕d₂⊕d₄⊕d₆⊕d₇⊕d₁₀
//	p₄ = d₀⊕d₂⊕d₅⊕d₆⊕d₈⊕d₉⊕d₁₀
func parityHamming16_11(data [11]bit.Bit) [5]bit.Bit {
	return [5]bit.Bit{
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[5] ^ data[7] ^ data[8],
		data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[6] ^ data[8] ^ data[9],
		data[2] ^ data[3] ^ data[4] ^ data[5] ^ data[7] ^ data[9] ^ data[10],
		data[0] ^ data[1] ^ data[2] ^ data[4] ^ data[6] ^ data[7] ^ data[10],
		data[0] ^ data[2] ^ data[5] ^ data[6] ^ data[8] ^ data[9] ^ data[10],
	}
}
