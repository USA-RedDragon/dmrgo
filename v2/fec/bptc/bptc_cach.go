package bptc

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 §B.2.3 — Variable length BPTC for CACH Signalling
//
// The CACH BPTC encodes a 36-bit Short LC PDU into 68 bits spread across
// 4 consecutive CACH burst signalling payloads (17 bits each).
//
// Encode matrix: 4 rows × 17 columns
//   - Rows 0..2: 12 data bits + 5 Hamming(17,12,3) parity bits
//   - Row 3: column parity (even parity per column across all 4 rows)
//
// Info bits: 3×12 = 36 bits (28 Short LC data + 8 CRC-8)
//
// Interleaving: columns top-to-bottom, left-to-right from the encode matrix
// are written rows left-to-right into the transmit matrix.

// calculateSyndrome17_12 computes the 5-bit Hamming(17,12,3) syndrome
// for a 17-bit codeword [d₀..d₁₁, p₀..p₄].
//
// Syndrome equations (from the parity check matrix H = [Pᵀ | I₅]):
//
//	s₀ = r₀⊕r₁⊕r₂⊕r₃⊕r₆⊕r₇⊕r₉⊕r₁₂
//	s₁ = r₀⊕r₁⊕r₂⊕r₃⊕r₄⊕r₇⊕r₈⊕r₁₀⊕r₁₃
//	s₂ = r₁⊕r₂⊕r₃⊕r₄⊕r₅⊕r₈⊕r₉⊕r₁₁⊕r₁₄
//	s₃ = r₀⊕r₁⊕r₄⊕r₅⊕r₇⊕r₁₀⊕r₁₅
//	s₄ = r₀⊕r₁⊕r₂⊕r₅⊕r₆⊕r₈⊕r₁₁⊕r₁₆
func calculateSyndrome17_12(bits [17]bit.Bit) int {
	var s [5]bit.Bit
	s[0] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[6] ^ bits[7] ^ bits[9] ^ bits[12]
	s[1] = bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[7] ^ bits[8] ^ bits[10] ^ bits[13]
	s[2] = bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[8] ^ bits[9] ^ bits[11] ^ bits[14]
	s[3] = bits[0] ^ bits[1] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[10] ^ bits[15]
	s[4] = bits[0] ^ bits[1] ^ bits[2] ^ bits[5] ^ bits[6] ^ bits[8] ^ bits[11] ^ bits[16]
	return int(s[0]) | int(s[1])<<1 | int(s[2])<<2 | int(s[3])<<3 | int(s[4])<<4
}

// parityHamming17_12 computes the 5 parity bits for 12 data bits
// using the Hamming(17,12,3) generator matrix (Table B.13).
//
// Parity equations:
//
//	p₀ = d₀⊕d₁⊕d₂⊕d₃⊕d₆⊕d₇⊕d₉
//	p₁ = d₀⊕d₁⊕d₂⊕d₃⊕d₄⊕d₇⊕d₈⊕d₁₀
//	p₂ = d₁⊕d₂⊕d₃⊕d₄⊕d₅⊕d₈⊕d₉⊕d₁₁
//	p₃ = d₀⊕d₁⊕d₄⊕d₅⊕d₇⊕d₁₀
//	p₄ = d₀⊕d₁⊕d₂⊕d₅⊕d₆⊕d₈⊕d₁₁
func parityHamming17_12(data [12]bit.Bit) [5]bit.Bit {
	return [5]bit.Bit{
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[6] ^ data[7] ^ data[9],            // p₀
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[7] ^ data[8] ^ data[10], // p₁
		data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[5] ^ data[8] ^ data[9] ^ data[11], // p₂
		data[0] ^ data[1] ^ data[4] ^ data[5] ^ data[7] ^ data[10],                     // p₃
		data[0] ^ data[1] ^ data[2] ^ data[5] ^ data[6] ^ data[8] ^ data[11],           // p₄
	}
}

// DecodeCACHBPTC decodes a 36-bit Short LC PDU from 4 × 17-bit CACH
// signalling payloads.
//
// The fragments should be in order (CACH burst 0 through CACH burst 3).
// Returns the 36 information bits (28 data + 8 CRC-8) and FEC result.
func DecodeCACHBPTC(fragments [4][17]bit.Bit) ([36]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 68}

	// Concatenate into 68-bit transmit stream
	var stream [68]bit.Bit
	for i := 0; i < 4; i++ {
		copy(stream[i*17:(i+1)*17], fragments[i][:])
	}

	// Deinterleave: transmit order → encode matrix
	// Transmit matrix is read rows left-to-right.
	// Encode matrix is read columns top-to-bottom, left-to-right.
	// For sequence index j (0..67):
	//   transmit_row = j / 17, transmit_col = j % 17
	//   encode_row = j % 4, encode_col = j / 4
	var matrix [4][17]bit.Bit
	for j := 0; j < 68; j++ {
		encRow := j % 4
		encCol := j / 4
		matrix[encRow][encCol] = stream[j]
	}

	// Error correction: Hamming(17,12,3) on rows, then column parity check
	// Iterate up to 5 passes for convergence
	for pass := 0; pass < 5; pass++ {
		fixing := false

		// Column parity check (even parity, simple detection)
		for col := 0; col < 17; col++ {
			var parity bit.Bit
			for row := 0; row < 4; row++ {
				parity ^= matrix[row][col]
			}
			if parity != 0 {
				result.ErrorsCorrected++
				fixing = true
			}
		}

		// Row Hamming(17,12,3) correction on rows 0..2
		for row := 0; row < 3; row++ {
			s := calculateSyndrome17_12(matrix[row])
			if s != 0 {
				errPos := hamming17_12_syndrome_table[s]
				if errPos >= 0 && errPos < 17 {
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

	// Recompute column parity row from corrected data rows
	if !result.Uncorrectable {
		for col := 0; col < 17; col++ {
			var expected bit.Bit
			for row := 0; row < 3; row++ {
				expected ^= matrix[row][col]
			}
			if matrix[3][col] != expected {
				result.Uncorrectable = true
				break
			}
		}
	}

	// Extract 36 info bits from rows 0..2, columns 0..11
	var info [36]bit.Bit
	for row := 0; row < 3; row++ {
		copy(info[row*12:(row+1)*12], matrix[row][:12])
	}

	return info, result
}

// EncodeCACHBPTC encodes a 36-bit Short LC PDU into 4 × 17-bit CACH
// signalling payloads.
func EncodeCACHBPTC(info [36]bit.Bit) [4][17]bit.Bit {
	var matrix [4][17]bit.Bit

	// Place 36 info bits into rows 0..2, columns 0..11
	for row := 0; row < 3; row++ {
		copy(matrix[row][:12], info[row*12:(row+1)*12])
	}

	// Compute Hamming(17,12,3) parity for rows 0..2
	for row := 0; row < 3; row++ {
		var data [12]bit.Bit
		copy(data[:], matrix[row][:12])
		parity := parityHamming17_12(data)
		matrix[row][12] = parity[0]
		matrix[row][13] = parity[1]
		matrix[row][14] = parity[2]
		matrix[row][15] = parity[3]
		matrix[row][16] = parity[4]
	}

	// Compute column parity row (row 3)
	for col := 0; col < 17; col++ {
		matrix[3][col] = matrix[0][col] ^ matrix[1][col] ^ matrix[2][col]
	}

	// Interleave: encode matrix → transmit stream
	// Read encode matrix columns top-to-bottom, left-to-right → 68-bit stream
	// Write into transmit matrix rows left-to-right
	var stream [68]bit.Bit
	for j := 0; j < 68; j++ {
		encRow := j % 4
		encCol := j / 4
		stream[j] = matrix[encRow][encCol]
	}

	// Split into 4 × 17-bit fragments
	var fragments [4][17]bit.Bit
	for i := 0; i < 4; i++ {
		copy(fragments[i][:], stream[i*17:(i+1)*17])
	}

	return fragments
}
