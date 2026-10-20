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

type syndromes [checksumSize]uint8

type decodeResult struct {
	Data          [dataSize]byte
	ErrorsFound   int
	BitsCorrected int
	Uncorrectable bool
}

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

	// ETSI TS 102 361-1 equation B.10
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

func syndromeHasErrors(s *syndromes) bool {
	return s[0]|s[1]|s[2] != 0
}

func galoisMul(a, b uint8) uint8 {
	if a == 0 || b == 0 {
		return 0
	}
	return expSum(int(galois_log_table[a]), int(galois_log_table[b]))
}

func expSum(a, b int) uint8 {
	if a += b; a >= 255 {
		a -= 255
	}
	return galois_exp_table[a]
}
