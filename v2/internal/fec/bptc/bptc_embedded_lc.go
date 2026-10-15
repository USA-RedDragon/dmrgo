package bptc

import (
	"math/bits"
	"sync"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/lowbit"
)

// ETSI TS 102 361-1 clause B.2.1: Variable length BPTC for Embedded Signalling
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

// DecodeEmbedded decodes a full embedded LC from 4 × 32-bit fragments.
// The fragments should be in order (fragment 0 from voice burst B, through
// fragment 3 from voice burst E of a superframe).
//
// Returns the 77 information bits (72 LC + 5 checksum) and FEC result. The
// bits are rows 0..6, columns 0..10 of the encode matrix in row-major order,
// so per Figure B.3 the checksum bits are the last bit of rows 2..6. It is
// DecodeEmbeddedWithCheck with no check, so a received word at equal
// distance from two codewords is uncorrectable.
func DecodeEmbedded(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.Result) {
	return DecodeEmbeddedWithCheck(fragments, nil)
}

// EmbeddedMaxErrors is the largest number of bit errors that
// DecodeEmbeddedWithCheck corrects.
const EmbeddedMaxErrors = 8

// DecodeEmbeddedWithCheck decodes a full embedded LC to the nearest
// codeword of the product code. It finds the codewords at the smallest
// Hamming distance from the received bits, up to EmbeddedMaxErrors, and
// keeps those whose information bits check accepts; a nil check accepts all
// of them. The LC is uncorrectable unless exactly one codeword remains. The
// search covers error patterns of up to 4 bits in each of rows 0..6. The
// product code has minimum distance 8, so up to 3 errors always give one
// nearest codeword. With 4 or more errors two codewords can be equally near,
// and check, for example the 5-bit LC checksum, picks between them.
func DecodeEmbeddedWithCheck(fragments [4][32]bit.Bit, check func(info [77]bit.Bit) bool) ([77]bit.Bit, fec.Result) {
	for i := range fragments {
		lowbit.Mask(fragments[i][:])
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
	patterns := embeddedLCPatterns()
	for row := 7; row >= 0; row-- {
		s.minRemaining[row] = s.minRemaining[row+1] + patterns.minWeight[s.syndromes[row]]
	}
	s.search(patterns, 0, 0, 0)

	result := fec.Result{BitsChecked: 128}
	if !s.found || s.ambiguous {
		result.Uncorrectable = true
		return embeddedLCInfo(&matrix), result
	}
	result.ErrorsCorrected = s.best
	return s.info, result
}

// embeddedLCSearch is a branch and bound search for the error patterns of
// least weight. An error pattern of rows 0..6 with the row syndromes fixes
// row 7, because the column parity of the errors must equal the column
// parity of the received rows. Row 7 then also has its syndrome, as the
// syndrome is linear.
type embeddedLCSearch struct {
	rows         [8]uint16
	syndromes    [8]int
	parity       uint16
	minRemaining [9]int
	errors       [8]uint16
	check        func([77]bit.Bit) bool
	best         int
	found        bool
	ambiguous    bool
	info         [77]bit.Bit
}

func (s *embeddedLCSearch) search(patterns *embeddedLCPatternTable, row int, sum uint16, weight int) {
	if row == 7 {
		s.errors[7] = s.parity ^ sum
		s.leaf(weight + bits.OnesCount16(s.errors[7]))
		return
	}
	syndrome := s.syndromes[row]
	for w := patterns.minWeight[syndrome]; w < len(patterns.bySyndrome[syndrome]); w += 2 {
		if weight+w+s.minRemaining[row+1] > s.best {
			return
		}
		for _, e := range patterns.bySyndrome[syndrome][w] {
			s.errors[row] = e
			s.search(patterns, row+1, sum^e, weight+w)
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
	var info [77]bit.Bit
	for row := 0; row < 7; row++ {
		corrected := s.rows[row] ^ s.errors[row]
		for col := 0; col < 11; col++ {
			info[row*11+col] = bit.Bit(corrected >> col & 1)
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

// embeddedLCPatternTable lists the 16-bit row error patterns of weight up to
// 4 by Hamming(16,11,4) syndrome and weight. Bit c is column c. Each column
// syndrome has odd weight, so the weight of a pattern has the parity of the
// weight of its syndrome.
type embeddedLCPatternTable struct {
	bySyndrome [32][5][]uint16
	minWeight  [32]int
}

var embeddedLCPatterns = sync.OnceValue(func() *embeddedLCPatternTable { //nolint:gochecknoglobals
	t := &embeddedLCPatternTable{}
	var columns [16]int
	for col := range columns {
		var unit [16]bit.Bit
		unit[col] = 1
		columns[col] = calculateSyndrome16_11(unit)
	}
	for s := range t.minWeight {
		t.minWeight[s] = -1
	}
	for m := 0; m < 1<<16; m++ {
		w := bits.OnesCount16(uint16(m))
		if w > 4 {
			continue
		}
		syndrome := 0
		for col := range columns {
			if m>>col&1 == 1 {
				syndrome ^= columns[col]
			}
		}
		t.bySyndrome[syndrome][w] = append(t.bySyndrome[syndrome][w], uint16(m))
		if t.minWeight[syndrome] < 0 || w < t.minWeight[syndrome] {
			t.minWeight[syndrome] = w
		}
	}
	return t
})

// DecodeEmbeddedStrict decodes a full embedded LC as MMDVMHost
// (CDMREmbeddedData::decodeEmbeddedData) does. Each of rows 0..6 is
// corrected for at most one error with Hamming(16,11,4), and a row with a
// detected double error makes the LC uncorrectable. Row 7 is not corrected:
// the LC is uncorrectable when any column parity over all 8 rows fails. It
// accepts far fewer LCs with bit errors than DecodeEmbedded.
func DecodeEmbeddedStrict(fragments [4][32]bit.Bit) ([77]bit.Bit, fec.Result) {
	for i := range fragments {
		lowbit.Mask(fragments[i][:])
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

func embeddedLCResult(matrix, received *[8][16]bit.Bit) fec.Result {
	result := fec.Result{BitsChecked: 128}
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

func columnParity(matrix *[8][16]bit.Bit, col int) bit.Bit {
	var parity bit.Bit
	for row := range matrix {
		parity ^= matrix[row][col]
	}
	return parity
}

// EncodeEmbedded encodes 77 information bits (72 LC + 5 checksum) into
// 4 × 32-bit fragments for transmission across 4 voice bursts.
func EncodeEmbedded(info [77]bit.Bit) [4][32]bit.Bit {
	lowbit.Mask(info[:])
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

	// Interleave: read columns top-to-bottom, left-to-right, to a 128-bit stream
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
