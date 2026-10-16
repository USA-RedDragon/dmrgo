package bptc

import (
	"encoding/binary"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
)

// The embedded LC BPTC has an 8×16 encode matrix (ETSI TS 102 361-1 clause
// B.2.1). Rows 0 to 6 hold 11 information bits and 5 Hamming(16,11,4) parity
// bits, and row 7 holds the even parity of each column. The columns, top to
// bottom and left to right, make 4 fragments of 32 bits for 4 voice bursts,
// so byte k of the fragments, the first bit most significant, is column k,
// and the matrix is the transpose of those bytes. A row is a uint16 with
// column c in bit c.

// DecodeEmbedded decodes the 77 information bits (72 LC bits and a 5-bit
// checksum) from the fragments of voice bursts B to E, in that order, each
// with its first bit most significant. The information bits are rows 0 to 6,
// columns 0 to 10 of the encode matrix in row-major order, so the checksum
// bits are the last bit of rows 2 to 6 (ETSI TS 102 361-1 Figure B.3). It
// returns them packed, the first bit most significant. It is
// DecodeEmbeddedWithCheck with a nil check.
func DecodeEmbedded(fragments [4]uint32) ([10]byte, fec.Result) {
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
func DecodeEmbeddedWithCheck(fragments [4]uint32, check func(info [10]byte) bool) ([10]byte, fec.Result) {
	s := embeddedLCSearch{best: EmbeddedMaxErrors, check: check, rows: embeddedLCMatrix(fragments)}
	for row := range s.rows {
		s.syndromes[row] = rowSyndrome16_11(s.rows[row])
		s.parity ^= s.rows[row]
	}
	result := fec.Result{BitsChecked: 128}
	if s.parity == 0 && s.syndromes == [8]int{} {
		info := embeddedLCInfo(&s.rows)
		result.Uncorrectable = check != nil && !check(info)
		return info, result
	}
	for row := 7; row >= 0; row-- {
		s.minRemaining[row] = s.minRemaining[row+1] + embeddedLCMinWeight[s.syndromes[row]]
	}
	s.search(0, 0, 0)

	if !s.found || s.ambiguous {
		result.Uncorrectable = true
		return embeddedLCInfo(&s.rows), result
	}
	result.ErrorsCorrected = s.best
	return s.info, result
}

// embeddedLCSearch is a branch and bound search for the error patterns of
// least weight, with up to 4 errors in each of rows 0 to 6. The errors of
// rows 0 to 6 fix the errors of row 7, because the column parity of the
// errors must equal the column parity of the received rows.
type embeddedLCSearch struct {
	rows         [8]uint16
	syndromes    [8]int
	parity       uint16
	minRemaining [9]int
	errors       [8]uint16
	check        func([10]byte) bool
	best         int
	found        bool
	ambiguous    bool
	info         [10]byte
}

func (s *embeddedLCSearch) search(row int, sum uint16, weight int) {
	if row == 7 {
		s.errors[7] = s.parity ^ sum
		s.leaf(weight + bits.OnesCount16(s.errors[7]))
		return
	}
	syndrome := s.syndromes[row]
	for w := embeddedLCMinWeight[syndrome]; w < 5; w += 2 {
		if weight+w+s.minRemaining[row+1] > s.best {
			return
		}
		for _, e := range embeddedLCPatterns[embeddedLCPatternStart[syndrome*5+w]:embeddedLCPatternStart[syndrome*5+w+1]] {
			s.errors[row] = e
			s.search(row+1, sum^e, weight+w)
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
	var corrected [8]uint16
	for row := range 7 {
		corrected[row] = s.rows[row] ^ s.errors[row]
	}
	info := embeddedLCInfo(&corrected)
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
func DecodeEmbeddedStrict(fragments [4]uint32) ([10]byte, fec.Result) {
	rows := embeddedLCMatrix(fragments)
	received := rows
	uncorrectable := false
	for row := 0; row < 7 && !uncorrectable; row++ {
		syndrome := rowSyndrome16_11(rows[row])
		if syndrome == 0 {
			continue
		}
		if errPos := hamming16_11_syndrome_table[syndrome]; errPos >= 0 {
			rows[row] ^= 1 << errPos
		} else {
			uncorrectable = true
		}
	}
	if !uncorrectable {
		var parity uint16
		for _, r := range rows {
			parity ^= r
		}
		uncorrectable = parity != 0
	}
	result := fec.Result{BitsChecked: 128, Uncorrectable: uncorrectable}
	for row := range rows {
		result.ErrorsCorrected += bits.OnesCount16(rows[row] ^ received[row])
	}
	return embeddedLCInfo(&rows), result
}

// embeddedLCMatrix reassembles the 8×16 encode matrix from the 4 fragments.
func embeddedLCMatrix(fragments [4]uint32) [8]uint16 {
	left := transpose8(uint64(fragments[0])<<32 | uint64(fragments[1]))
	right := transpose8(uint64(fragments[2])<<32 | uint64(fragments[3]))
	var rows [8]uint16
	for row := range rows {
		shift := 56 - 8*row
		rows[row] = uint16(bits.Reverse8(uint8(left>>shift&0xFF))) | uint16(bits.Reverse8(uint8(right>>shift&0xFF)))<<8
	}
	return rows
}

// transpose8 transposes the 8×8 bit matrix x, byte i from the most
// significant holding row i with column 0 in its most significant bit
// (Warren, "Hacker's Delight", 2nd ed., section 7-3).
func transpose8(x uint64) uint64 {
	t := (x ^ x>>7) & 0x00AA00AA00AA00AA
	x ^= t ^ t<<7
	t = (x ^ x>>14) & 0x0000CCCC0000CCCC
	x ^= t ^ t<<14
	t = (x ^ x>>28) & 0x00000000F0F0F0F0
	return x ^ t ^ t<<28
}

// embeddedLCInfo returns the 77 information bits, rows 0 to 6 and columns 0
// to 10 of the matrix in row-major order, packed.
func embeddedLCInfo(rows *[8]uint16) [10]byte {
	var head, tail uint64
	for row := range 5 {
		head = head<<11 | uint64(bits.Reverse16(rows[row])>>5)
	}
	for row := 5; row < 7; row++ {
		tail = tail<<11 | uint64(bits.Reverse16(rows[row])>>5)
	}
	var info [10]byte
	binary.BigEndian.PutUint64(info[:8], head<<9|tail>>13)
	binary.BigEndian.PutUint16(info[8:], uint16(tail<<3&0xFFFF))
	return info
}

// EncodeEmbedded encodes the 77 packed information bits (72 LC bits and a
// 5-bit checksum) into the 32-bit fragments of 4 voice bursts, as
// DecodeEmbedded takes them.
func EncodeEmbedded(info [10]byte) [4]uint32 {
	var rows [8]uint16
	for row := range 7 {
		data := uint16(packed.Field(info[:], row*11, 11) & 0x7FF)
		rows[row] = rowParity16_11(bits.Reverse16(data << 5))
		rows[7] ^= rows[row]
	}

	var left, right uint64
	for row, r := range rows {
		shift := 56 - 8*row
		left |= uint64(bits.Reverse8(uint8(r&0xFF))) << shift
		right |= uint64(bits.Reverse8(uint8(r>>8))) << shift
	}
	left, right = transpose8(left), transpose8(right)
	return [4]uint32{uint32(left >> 32), uint32(left & 0xFFFFFFFF), uint32(right >> 32), uint32(right & 0xFFFFFFFF)}
}
