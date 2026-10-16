// Package reedsolomon encodes, checks and decodes the shortened Reed-Solomon
// (12,9) code that protects a 9-byte Full Link Control (ETSI TS 102 361-1
// clause B.3.6).
package reedsolomon

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

const (
	dataSize     = 9
	checksumSize = 3
)

// syndromes holds the syndromes S1 to S3 of a codeword: the received
// polynomial at alpha, alpha^2 and alpha^3, the first byte the coefficient of
// the highest power.
type syndromes [checksumSize]uint8

// decodeResult is the result of a Reed-Solomon decode.
type decodeResult struct {
	Data          [dataSize]byte // Corrected data
	ErrorsFound   int            // Number of symbol (byte) errors found and corrected
	BitsCorrected int            // Number of bits the correction flipped
	Uncorrectable bool           // True if errors could not be corrected
}

// decode corrects a 12-byte codeword (9 data bytes and 3 parity bytes) and
// returns the data bytes and the error counts. The code has distance 4, so it
// corrects one symbol error: the syndromes of an error of value e at degree d
// are e*alpha^d, e*alpha^2d and e*alpha^3d. Any other nonzero syndromes are
// uncorrectable, and the received data comes back unchanged.
func decode(data [dataSize + checksumSize]byte) decodeResult {
	var result decodeResult
	copy(result.Data[:], data[:dataSize])

	var s syndromes
	calcSyndrome(&data, &s)
	if !syndromeHasErrors(&s) {
		return result
	}
	if s[0] == 0 || s[1] == 0 || s[2] == 0 {
		result.Uncorrectable = true
		return result
	}
	l1, l2, l3 := int(galois_log_table[s[0]]), int(galois_log_table[s[1]]), int(galois_log_table[s[2]])
	d := (l2 - l1 + 255) % 255
	if (l3-l2+255)%255 != d || d >= dataSize+checksumSize {
		result.Uncorrectable = true
		return result
	}
	value := galois_exp_table[(l1-d+255)%255]
	if pos := dataSize + checksumSize - 1 - d; pos < dataSize {
		result.Data[pos] ^= value
	}
	result.ErrorsFound = 1
	result.BitsCorrected = bits.OnesCount8(value)
	return result
}

// Encode returns the 12-byte codeword of 9 data bytes: the data followed by
// 3 parity bytes (ETSI TS 102 361-1 clause B.3.6).
func Encode(data [9]byte) [12]byte {
	var codeword [12]byte
	copy(codeword[:], data[:])

	// Systematic encoding by division by g(x) = x^3 + 0x0E x^2 + 0x38 x + 0x40
	// (ETSI TS 102 361-1 equation B.10).
	var r0, r1, r2 uint8
	for i := 0; i < dataSize; i++ {
		fb := codeword[i] ^ r0
		r0 = r1 ^ galoisMul(fb, 0x0E)
		r1 = r2 ^ galoisMul(fb, 0x38)
		r2 = galoisMul(fb, 0x40)
	}
	codeword[dataSize] = r0
	codeword[dataSize+1] = r1
	codeword[dataSize+2] = r2

	return codeword
}

// Decode corrects up to one symbol error in a 12-byte codeword and returns
// the 9 data bytes. ErrorsCorrected counts corrected bits. When the codeword
// is uncorrectable, Decode returns the received data bytes and sets
// Uncorrectable.
func Decode(cw [12]byte) ([9]byte, fec.Result) {
	r := decode(cw)
	return r.Data, fec.Result{
		BitsChecked:     96,
		ErrorsCorrected: r.BitsCorrected,
		Uncorrectable:   r.Uncorrectable,
	}
}

// Check reports whether cw is a codeword.
func Check(cw [12]byte) bool {
	var s syndromes
	calcSyndrome(&cw, &s)
	return !syndromeHasErrors(&s)
}

// calcSyndrome computes the 3 syndromes of a 12-byte codeword. Byte i is the
// coefficient of x^(11-i), so it adds b*alpha^(j*(11-i)) to syndrome j.
func calcSyndrome(data *[dataSize + checksumSize]byte, s *syndromes) {
	var s1, s2, s3 uint8
	for i, b := range data {
		if b == 0 {
			continue
		}
		l, d := int(galois_log_table[b]), len(data)-1-i
		s1 ^= expSum(l, d)
		s2 ^= expSum(l, 2*d)
		s3 ^= expSum(l, 3*d)
	}
	*s = syndromes{s1, s2, s3}
}

// syndromeHasErrors reports whether any syndrome value is nonzero.
func syndromeHasErrors(s *syndromes) bool {
	return s[0]|s[1]|s[2] != 0
}

// galoisMul returns the product of a and b in GF(2^8).
func galoisMul(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	return expSum(int(galois_log_table[a]), int(galois_log_table[b]))
}

// expSum returns alpha^(a+b) for a and b from 0 to 254.
func expSum(a, b int) uint8 {
	if a += b; a >= 255 {
		a -= 255
	}
	return galois_exp_table[a]
}
