package bptc

import (
	"encoding/binary"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	packed "github.com/USA-RedDragon/dmrgo/v2/internal/bits"
)

// ETSI TS 102 361-1 clause B.2.1

// DecodeEmbedded decodes the 77 information bits (72 LC bits and a 5-bit
// checksum) from the fragments of voice bursts B to E, in that order, each
// with its first bit most significant. The information bits are rows 0 to 6,
// columns 0 to 10 of the encode matrix in row-major order, so the checksum
// bits are the last bit of rows 2 to 6. It returns them packed, the first bit
// most significant. It is DecodeEmbeddedWithCheck with a nil check.
// (ETSI TS 102 361-1 Figure B.3)
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
		s.syndromes[row] = uint8(rowSyndrome16_11(s.rows[row]) & 0x1F)
		s.parity ^= s.rows[row]
	}
	result := fec.Result{BitsChecked: 128}
	if s.parity == 0 && s.syndromes == [8]uint8{} {
		info := embeddedLCInfo(&s.rows)
		result.Uncorrectable = check != nil && !check(info)
		return info, result
	}
	s.prepare()
	s.run(7)
	for derived := 6; derived >= 0 && s.best >= 5; derived-- {
		s.run(derived)
	}

	if !s.found || s.ambiguous {
		result.Uncorrectable = true
		return embeddedLCInfo(&s.rows), result
	}
	result.ErrorsCorrected = s.best
	return s.info, result
}

type embeddedLCSearch struct {
	rows         [8]uint16
	syndromes    [8]uint8
	parity       uint16
	order        [7]uint8
	derived      int
	minRemaining [8]int
	errors       [8]uint16
	columns      [16]uint8
	positions    [32]int8
	check        func([10]byte) bool
	best         int
	found        bool
	ambiguous    bool
	info         [10]byte
}

func (s *embeddedLCSearch) prepare() {
	for c := range s.columns {
		s.columns[c] = column16_11(c)
	}
	for syndrome := range s.positions {
		s.positions[syndrome] = wordPos8(h1611PosLo, h1611PosHi, h1611PosTop, syndrome)
	}
}

func (s *embeddedLCSearch) run(derived int) {
	s.derived = derived
	k := 0
	for row := range s.rows {
		if row != derived {
			s.order[k] = uint8(row)
			k++
		}
	}
	s.minRemaining[7] = 5
	if derived == 7 {
		s.minRemaining[7] = minWeight16_11(int(s.syndromes[7]))
	}
	for i := 6; i >= 0; i-- {
		s.minRemaining[i] = s.minRemaining[i+1] + minWeight16_11(int(s.syndromes[s.order[i]]))
	}
	s.search(0, 0, 0)
}

const h1611Columns uint64 = 0x72d36eaae3fd79

func column16_11(c int) uint8 {
	if c >= 11 {
		return 1 << (c - 11)
	}
	return uint8(h1611Columns >> (5 * c) & 0x1F)
}

func pos16_11(syndrome int) int {
	return wordPos(h1611PosLo, h1611PosHi, h1611PosTop, syndrome)
}

func minWeight16_11(syndrome int) int {
	switch {
	case syndrome == 0:
		return 0
	case pos16_11(syndrome) >= 0:
		return 1
	}
	return 2
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
	syndrome := int(s.syndromes[row])
	for w := minWeight16_11(syndrome); w < 5; w += 2 {
		if weight+w+s.minRemaining[depth+1] > s.best {
			return
		}
		next := weight + w
		switch w {
		case 0:
			s.try(row, depth, sum, next, 0)
		case 1:
			e := uint16(1) << s.positions[syndrome]
			s.try(row, depth, sum^e, next, e)
		case 2:
			for c1 := range int8(16) {
				if c2 := s.positions[uint8(syndrome&0x1F)^s.columns[c1]]; c2 > c1 {
					e := uint16(1)<<c1 | 1<<c2
					s.try(row, depth, sum^e, next, e)
				}
			}
		case 3:
			for c1 := range int8(16) {
				s1 := uint8(syndrome&0x1F) ^ s.columns[c1]
				for c2 := c1 + 1; c2 < 16; c2++ {
					if c3 := s.positions[s1^s.columns[c2]]; c3 > c2 {
						e := uint16(1)<<c1 | 1<<c2 | 1<<c3
						s.try(row, depth, sum^e, next, e)
					}
				}
			}
		default:
			for c1 := range int8(16) {
				s1 := uint8(syndrome&0x1F) ^ s.columns[c1]
				for c2 := c1 + 1; c2 < 16; c2++ {
					s2 := s1 ^ s.columns[c2]
					for c3 := c2 + 1; c3 < 16; c3++ {
						if c4 := s.positions[s2^s.columns[c3]]; c4 > c3 {
							e := uint16(1)<<c1 | 1<<c2 | 1<<c3 | 1<<c4
							s.try(row, depth, sum^e, next, e)
						}
					}
				}
			}
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

func transpose8(x uint64) uint64 {
	t := (x ^ x>>7) & 0x00AA00AA00AA00AA
	x ^= t ^ t<<7
	t = (x ^ x>>14) & 0x0000CCCC0000CCCC
	x ^= t ^ t<<14
	t = (x ^ x>>28) & 0x00000000F0F0F0F0
	return x ^ t ^ t<<28
}

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

func (s *embeddedLCSearch) try(row uint8, depth int, sum uint16, weight int, e uint16) {
	s.errors[row] = e
	s.search(depth+1, sum, weight)
}
