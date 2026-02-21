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
// Returns the 77 information bits (72 LC + 5 checksum) and FEC result.
func DecodeEmbeddedLC(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 128}

	// Reassemble the 128-bit stream from 4 fragments
	var stream [128]bit.Bit
	for i := 0; i < 4; i++ {
		copy(stream[i*32:(i+1)*32], fragments[i][:])
	}

	// Deinterleave: transmit bits → encode matrix (8 rows × 16 columns)
	// Interleaving writes columns top-to-bottom, left-to-right
	// So stream[0..7] = column 0 (rows 0..7), stream[8..15] = column 1, etc.
	var matrix [8][16]bit.Bit
	for col := 0; col < 16; col++ {
		for row := 0; row < 8; row++ {
			matrix[row][col] = stream[col*8+row]
		}
	}

	// Error correction: Hamming(16,11,4) on rows, then column parity check
	// Iterate up to 5 passes for convergence
	for pass := 0; pass < 5; pass++ {
		fixing := false

		// Column parity check (even parity, simple detection)
		for col := 0; col < 16; col++ {
			var parity bit.Bit
			for row := 0; row < 8; row++ {
				parity ^= matrix[row][col]
			}
			if parity != 0 {
				// Column parity error detected; row Hamming should fix it
				fixing = true
			}
		}

		// Row Hamming(16,11,4) correction
		for row := 0; row < 7; row++ {
			s := calculateSyndrome16_11(matrix[row])
			if s != 0 {
				errPos := hamming16_11_syndrome_table[s]
				if errPos >= 0 && errPos < 16 {
					matrix[row][errPos] ^= 1
					result.ErrorsCorrected++
					fixing = true
				} else {
					result.Uncorrectable = true
				}
			}
		}

		if !fixing {
			break
		}
	}

	// Verify column parity after correction
	if !result.Uncorrectable {
		for col := 0; col < 16; col++ {
			var parity bit.Bit
			for row := 0; row < 8; row++ {
				parity ^= matrix[row][col]
			}
			if parity != 0 {
				result.Uncorrectable = true
				break
			}
		}
	}

	// Extract 77 info bits from rows 0..6, columns 0..10
	var info [77]bit.Bit
	idx := 0
	for row := 0; row < 7; row++ {
		for col := 0; col < 11; col++ {
			info[idx] = matrix[row][col]
			idx++
		}
	}

	return info, result
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
//	s₃ = r₀⊕r₁⊕r₂⊕r₄⊕r₅⊕r₆⊕r₇⊕r₁₄
//	s₄ = r₀⊕r₂⊕r₄⊕r₅⊕r₆⊕r₈⊕r₉⊕r₁₀⊕r₁₅
func calculateSyndrome16_11(bits [16]bit.Bit) int {
	s0 := bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	s1 := bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[12]
	s2 := bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10] ^ bits[13]
	s3 := bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[7] ^ bits[14]
	s4 := bits[0] ^ bits[2] ^ bits[4] ^ bits[5] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[10] ^ bits[15]
	return int(s0) | int(s1)<<1 | int(s2)<<2 | int(s3)<<3 | int(s4)<<4
}

// parityHamming16_11 computes the 5 Hamming(16,11,4) parity bits for 11 data bits.
//
// From the generator matrix (§B.3.4, Table B.16):
//
//	p₀ = d₀⊕d₁⊕d₂⊕d₃⊕d₅⊕d₇⊕d₈
//	p₁ = d₁⊕d₂⊕d₃⊕d₄⊕d₆⊕d₈⊕d₉
//	p₂ = d₂⊕d₃⊕d₄⊕d₅⊕d₇⊕d₉⊕d₁₀
//	p₃ = d₀⊕d₁⊕d₂⊕d₄⊕d₅⊕d₆⊕d₇
//	p₄ = d₀⊕d₂⊕d₄⊕d₅⊕d₆⊕d₈⊕d₉⊕d₁₀
func parityHamming16_11(data [11]bit.Bit) [5]bit.Bit {
	return [5]bit.Bit{
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[5] ^ data[7] ^ data[8],
		data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[6] ^ data[8] ^ data[9],
		data[2] ^ data[3] ^ data[4] ^ data[5] ^ data[7] ^ data[9] ^ data[10],
		data[0] ^ data[1] ^ data[2] ^ data[4] ^ data[5] ^ data[6] ^ data[7],
		data[0] ^ data[2] ^ data[4] ^ data[5] ^ data[6] ^ data[8] ^ data[9] ^ data[10],
	}
}
