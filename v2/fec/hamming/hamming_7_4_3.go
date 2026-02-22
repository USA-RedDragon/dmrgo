// Package hamming provides Hamming FEC codec implementations.
package hamming

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 - B.3.5 Hamming (7,4,3)
//
// Generator polynomial: G(x) = x³ + x + 1
//
// Generator matrix (systematic form [I₄ | P]):
//
//	1 0 0 0 | 1 0 1
//	0 1 0 0 | 1 1 1
//	0 0 1 0 | 1 1 0
//	0 0 0 1 | 0 1 1
//
// Codeword: [d₀ d₁ d₂ d₃ p₀ p₁ p₂]
//
// Parity check matrix H = [Pᵀ | I₃]:
//
//	1 1 1 0 | 1 0 0
//	0 1 1 1 | 0 1 0
//	1 1 0 1 | 0 0 1

// Parity743 computes the 3 parity bits for 4 data bits using Hamming(7,4,3).
func Parity743(data [4]bit.Bit) [3]bit.Bit {
	return [3]bit.Bit{
		data[0] ^ data[1] ^ data[2], // p₀
		data[1] ^ data[2] ^ data[3], // p₁
		data[0] ^ data[1] ^ data[3], // p₂
	}
}

// Decode743 decodes a 7-bit Hamming(7,4,3) codeword, correcting up to 1 bit error.
// Returns the corrected codeword and FECResult.
// Uses the generated hamming743_syndrome_table for syndrome→error-position lookup.
func Decode743(bits [7]bit.Bit) ([7]bit.Bit, fec.FECResult) {
	result := fec.FECResult{BitsChecked: 7}

	// Calculate syndrome: s = H · rᵀ
	// s₀ = r₀ ⊕ r₁ ⊕ r₂ ⊕ r₄
	// s₁ = r₁ ⊕ r₂ ⊕ r₃ ⊕ r₅
	// s₂ = r₀ ⊕ r₁ ⊕ r₃ ⊕ r₆
	s0 := bits[0] ^ bits[1] ^ bits[2] ^ bits[4]
	s1 := bits[1] ^ bits[2] ^ bits[3] ^ bits[5]
	s2 := bits[0] ^ bits[1] ^ bits[3] ^ bits[6]

	syndrome := int(s0)<<2 | int(s1)<<1 | int(s2)

	if syndrome == 0 {
		return bits, result
	}

	errPos := hamming743_syndrome_table[syndrome]
	if errPos < 0 {
		// Should not happen for a 3-bit syndrome in Hamming(7,4,3)
		result.Uncorrectable = true
		return bits, result
	}

	// Apply correction
	corrected := bits
	corrected[errPos] ^= 1
	result.ErrorsCorrected = 1

	return corrected, result
}

// Check743 returns true if the 7-bit codeword has valid parity.
func Check743(bits [7]bit.Bit) bool {
	var data [4]bit.Bit
	copy(data[:], bits[:4])

	parity := Parity743(data)
	return parity[0] == bits[4] && parity[1] == bits[5] && parity[2] == bits[6]
}

// Encode743 encodes 4 data bits into a 7-bit Hamming(7,4,3) codeword.
// Layout: [d₀ d₁ d₂ d₃ p₀ p₁ p₂]
func Encode743(data [4]bit.Bit) [7]bit.Bit {
	parity := Parity743(data)
	return [7]bit.Bit{
		data[0], data[1], data[2], data[3],
		parity[0], parity[1], parity[2],
	}
}

// Encode encodes a byte value (4 LSBs used) into a 7-bit Hamming(7,4,3) codeword.
// This is the byte-oriented entry point used by code generation.
func Encode(data byte) [7]bit.Bit {
	var bits [4]bit.Bit
	bits[0] = bit.Bit((data >> 3) & 1)
	bits[1] = bit.Bit((data >> 2) & 1)
	bits[2] = bit.Bit((data >> 1) & 1)
	bits[3] = bit.Bit(data & 1)
	return Encode743(bits)
}
