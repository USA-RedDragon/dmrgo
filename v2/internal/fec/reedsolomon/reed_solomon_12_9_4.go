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
	t1, t2 := s[0], s[1]
	d := 0
	for ; d < dataSize+checksumSize && (t1 != s[1] || t2 != s[2]); d++ {
		t1, t2 = xtime(t1), xtime(t2)
	}
	if d == dataSize+checksumSize {
		result.Uncorrectable = true
		return result
	}
	value := s[0]
	for range d {
		value = xtimeInverse(value)
	}
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
		x1 := xtime(codeword[i] ^ r0)
		x2 := xtime(x1)
		x3 := xtime(x2)
		x4 := xtime(x3)
		x5 := xtime(x4)
		r0 = r1 ^ x1 ^ x2 ^ x3
		r1 = r2 ^ x3 ^ x4 ^ x5
		r2 = xtime(x5)
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
	for _, b := range data {
		s1 = xtime(s1) ^ b
		s2 = xtime(xtime(s2)) ^ b
		s3 = xtime(xtime(xtime(s3))) ^ b
	}
	*s = syndromes{s1, s2, s3}
}

func syndromeHasErrors(s *syndromes) bool {
	return s[0]|s[1]|s[2] != 0
}

func xtime(a uint8) uint8 {
	return a<<1 ^ 0x1D&-(a>>7)
}

func xtimeInverse(a uint8) uint8 {
	return a>>1 ^ 0x8E&-(a&1)
}
