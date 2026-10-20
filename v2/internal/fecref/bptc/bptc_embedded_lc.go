package bptc

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

// ETSI TS 102 361-1 clause B.2.1

// DecodeEmbedded decodes the 77 information bits (72 LC bits and a 5-bit
// checksum) from the fragments of voice bursts B to E, in that order. They
// are rows 0 to 6, columns 0 to 10 of the encode matrix in row-major order,
// so the checksum bits are the last bit of rows 2 to 6. It is
// DecodeEmbeddedWithCheck with a nil check. (ETSI TS 102 361-1 Figure B.3)
func DecodeEmbedded(fragments [4][32]unpacked.Bit) ([77]unpacked.Bit, fec.Result) {
	return DecodeEmbeddedWithCheck(fragments, nil)
}

// EmbeddedMaxErrors is the largest number of bit errors that
// DecodeEmbeddedWithCheck corrects.
const EmbeddedMaxErrors = 8

// DecodeEmbeddedWithCheck decodes a full embedded LC to the nearest codewords
// of the product code, up to EmbeddedMaxErrors bit errors, and keeps those
// whose information bits check accepts (a nil check accepts all). The LC is
// uncorrectable unless exactly one remains. Up to 3 errors give one nearest
// codeword; with more, check (such as the LC checksum) picks between them.
func DecodeEmbeddedWithCheck(fragments [4][32]unpacked.Bit, check func(info [77]unpacked.Bit) bool) ([77]unpacked.Bit, fec.Result) {
	for i := range fragments {
		unpacked.Mask(fragments[i][:])
	}
	matrix := embeddedLCMatrix(fragments)
	s := embeddedLCSearch{best: EmbeddedMaxErrors, check: check}
	for row := range matrix {
		for col := range matrix[row] {
			s.rows[row] |= uint16(matrix[row][col]) << col
		}
		s.syndromes[row] = calculateSyndrome16_11(matrix[row])
		s.parity ^= s.rows[row]
	}
	s.run(7)
	for derived := 6; derived >= 0 && s.best >= 5; derived-- {
		s.run(derived)
	}

	result := fec.Result{BitsChecked: 128}
	if !s.found || s.ambiguous {
		result.Uncorrectable = true
		return embeddedLCInfo(&matrix), result
	}
	result.ErrorsCorrected = s.best
	return s.info, result
}

type embeddedLCSearch struct {
	rows         [8]uint16
	syndromes    [8]int
	parity       uint16
	order        [7]int
	derived      int
	minRemaining [8]int
	errors       [8]uint16
	check        func([77]unpacked.Bit) bool
	best         int
	found        bool
	ambiguous    bool
	info         [77]unpacked.Bit
}

func (s *embeddedLCSearch) run(derived int) {
	s.derived = derived
	k := 0
	for row := range s.rows {
		if row != derived {
			s.order[k] = row
			k++
		}
	}
	s.minRemaining[7] = 5
	if derived == 7 {
		s.minRemaining[7] = embeddedLCMinWeight[s.syndromes[7]]
	}
	for i := 6; i >= 0; i-- {
		s.minRemaining[i] = s.minRemaining[i+1] + embeddedLCMinWeight[s.syndromes[s.order[i]]]
	}
	s.search(0, 0, 0)
}

func (s *embeddedLCSearch) search(depth int, sum uint16, weight int) {
	if depth == 7 {
		e := s.parity ^ sum
		w := bits.OnesCount16(e)
		if s.derived != 7 && w < 5 {
			return
		}
		s.errors[s.derived] = e
		s.leaf(weight + w)
		return
	}
	row := s.order[depth]
	syndrome := s.syndromes[row]
	for w := embeddedLCMinWeight[syndrome]; w < 5; w += 2 {
		if weight+w+s.minRemaining[depth+1] > s.best {
			return
		}
		for _, e := range embeddedLCPatterns[embeddedLCPatternStart[syndrome*5+w]:embeddedLCPatternStart[syndrome*5+w+1]] {
			s.errors[row] = e
			s.search(depth+1, sum^e, weight+w)
		}
	}
}

func (s *embeddedLCSearch) leaf(weight int) {
	if weight > s.best {
		return
	}
	if weight < s.best {
		s.best = weight
		s.found = false
		s.ambiguous = false
	}
	var info [77]unpacked.Bit
	for row := 0; row < 7; row++ {
		corrected := s.rows[row] ^ s.errors[row]
		for col := 0; col < 11; col++ {
			info[row*11+col] = unpacked.Bit(corrected >> col & 1)
		}
	}
	if s.check != nil && !s.check(info) {
		return
	}
	if s.found {
		s.ambiguous = true
		return
	}
	s.found = true
	s.info = info
}

// DecodeEmbeddedStrict decodes a full embedded LC as MMDVMHost
// (CDMREmbeddedData::decodeEmbeddedData) does. It corrects at most one error
// in each of rows 0 to 6. The LC is uncorrectable when a row has a detected
// double error or a column parity over all 8 rows fails, so it accepts far
// fewer LCs with bit errors than DecodeEmbedded.
func DecodeEmbeddedStrict(fragments [4][32]unpacked.Bit) ([77]unpacked.Bit, fec.Result) {
	for i := range fragments {
		unpacked.Mask(fragments[i][:])
	}
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

func embeddedLCMatrix(fragments [4][32]unpacked.Bit) [8][16]unpacked.Bit {
	var matrix [8][16]unpacked.Bit
	for col := 0; col < 16; col++ {
		for row := 0; row < 8; row++ {
			n := col*8 + row
			matrix[row][col] = fragments[n/32][n%32]
		}
	}
	return matrix
}

func embeddedLCResult(matrix, received *[8][16]unpacked.Bit) fec.Result {
	result := fec.Result{BitsChecked: 128}
	for row := range matrix {
		for col := range matrix[row] {
			result.ErrorsCorrected += int(matrix[row][col] ^ received[row][col])
		}
	}
	return result
}

func embeddedLCInfo(matrix *[8][16]unpacked.Bit) [77]unpacked.Bit {
	var info [77]unpacked.Bit
	for row := 0; row < 7; row++ {
		copy(info[row*11:row*11+11], matrix[row][:11])
	}
	return info
}

func columnParity(matrix *[8][16]unpacked.Bit, col int) unpacked.Bit {
	var parity unpacked.Bit
	for row := range matrix {
		parity ^= matrix[row][col]
	}
	return parity
}

// EncodeEmbedded encodes 77 information bits (72 LC bits and a 5-bit
// checksum) into the 32-bit fragments of 4 voice bursts.
func EncodeEmbedded(info [77]unpacked.Bit) [4][32]unpacked.Bit {
	unpacked.Mask(info[:])
	var matrix [8][16]unpacked.Bit

	idx := 0
	for row := 0; row < 7; row++ {
		for col := 0; col < 11; col++ {
			matrix[row][col] = info[idx]
			idx++
		}
	}

	for row := 0; row < 7; row++ {
		var data [11]unpacked.Bit
		copy(data[:], matrix[row][:11])
		parity := parityHamming16_11(data)
		matrix[row][11] = parity[0]
		matrix[row][12] = parity[1]
		matrix[row][13] = parity[2]
		matrix[row][14] = parity[3]
		matrix[row][15] = parity[4]
	}

	for col := 0; col < 16; col++ {
		var parity unpacked.Bit
		for row := 0; row < 7; row++ {
			parity ^= matrix[row][col]
		}
		matrix[7][col] = parity
	}

	var stream [128]unpacked.Bit
	for col := 0; col < 16; col++ {
		for row := 0; row < 8; row++ {
			stream[col*8+row] = matrix[row][col]
		}
	}

	var fragments [4][32]unpacked.Bit
	for i := 0; i < 4; i++ {
		copy(fragments[i][:], stream[i*32:(i+1)*32])
	}

	return fragments
}

// ETSI TS 102 361-1 clause B.3.4
func calculateSyndrome16_11(bits [16]unpacked.Bit) int {
	s0 := bits[0] ^ bits[1] ^ bits[2] ^ bits[3] ^ bits[5] ^ bits[7] ^ bits[8] ^ bits[11]
	s1 := bits[1] ^ bits[2] ^ bits[3] ^ bits[4] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[12]
	s2 := bits[2] ^ bits[3] ^ bits[4] ^ bits[5] ^ bits[7] ^ bits[9] ^ bits[10] ^ bits[13]
	s3 := bits[0] ^ bits[1] ^ bits[2] ^ bits[4] ^ bits[6] ^ bits[7] ^ bits[10] ^ bits[14]
	s4 := bits[0] ^ bits[2] ^ bits[5] ^ bits[6] ^ bits[8] ^ bits[9] ^ bits[10] ^ bits[15]
	return int(s0) | int(s1)<<1 | int(s2)<<2 | int(s3)<<3 | int(s4)<<4
}

// ETSI TS 102 361-1 clause B.3.4, Table B.16
func parityHamming16_11(data [11]unpacked.Bit) [5]unpacked.Bit {
	return [5]unpacked.Bit{
		data[0] ^ data[1] ^ data[2] ^ data[3] ^ data[5] ^ data[7] ^ data[8],
		data[1] ^ data[2] ^ data[3] ^ data[4] ^ data[6] ^ data[8] ^ data[9],
		data[2] ^ data[3] ^ data[4] ^ data[5] ^ data[7] ^ data[9] ^ data[10],
		data[0] ^ data[1] ^ data[2] ^ data[4] ^ data[6] ^ data[7] ^ data[10],
		data[0] ^ data[2] ^ data[5] ^ data[6] ^ data[8] ^ data[9] ^ data[10],
	}
}
