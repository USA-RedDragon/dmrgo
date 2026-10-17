// Package bptc encodes and decodes the block product turbo codes of DMR:
// BPTC(196,96) for full bursts, the BPTC(128,77) of the embedded signalling,
// the CACH Short LC BPTC and the single burst BPTC (ETSI TS 102 361-1 clauses
// B.1 and B.2). The codecs take and return packed bits: a bit vector in bytes
// or a k-bit value in an unsigned integer, the first bit most significant.
package bptc

import (
	"encoding/binary"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// The BPTC(196,96) grid has 13 rows of 15 columns after a leading bit R(3)
// (ETSI TS 102 361-1 clause B.1.1). Grid index i, information bit dm[i] on
// air, is row (i-1)/15 and column (i-1)%15. A row is a uint16 with column c
// in bit c. Rows 0 to 8 are Hamming(15,11,3) codewords with data and
// reserved bits in columns 0 to 10, and rows 9 to 12 hold the Hamming(13,9,3)
// parity of each column.

// Masks of the columns that each Hamming(15,11,3) parity check covers.
const (
	h1511Check0 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11
	h1511Check1 = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<6 | 1<<8 | 1<<9 | 1<<12
	h1511Check2 = 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<9 | 1<<10 | 1<<13
	h1511Check3 = 1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10 | 1<<14
)

// gridRowMask covers the 15 columns of a row.
const gridRowMask = 1<<15 - 1

// rowSyndrome15_11 returns the 4-bit Hamming(15,11,3) syndrome of a row,
// check k in bit k.
func rowSyndrome15_11(row uint16) int {
	return bits.OnesCount16(row&h1511Check0)&1 | bits.OnesCount16(row&h1511Check1)&1<<1 |
		bits.OnesCount16(row&h1511Check2)&1<<2 | bits.OnesCount16(row&h1511Check3)&1<<3
}

// columnSyndromes returns the Hamming(13,9,3) syndromes of the 15 columns:
// bit c of element k is check k of column c.
func columnSyndromes(rows *[13]uint16) [4]uint16 {
	r := rows
	return [4]uint16{
		r[0] ^ r[1] ^ r[3] ^ r[5] ^ r[6] ^ r[9],
		r[0] ^ r[1] ^ r[2] ^ r[4] ^ r[6] ^ r[7] ^ r[10],
		r[0] ^ r[1] ^ r[2] ^ r[3] ^ r[5] ^ r[7] ^ r[8] ^ r[11],
		r[0] ^ r[2] ^ r[4] ^ r[5] ^ r[8] ^ r[12],
	}
}

// Decode196 deinterleaves a BPTC(196,96) block of 196 packed information
// bits, corrects it with the row and column Hamming codes and returns the 96
// data bits packed, the corrected reserved bits R(0) to R(2), R(i) in bit i,
// and a fec.Result (ETSI TS 102 361-1 clause B.1.1). R(3) has no FEC and is
// not returned. Bits 196 to 199 of info are ignored.
func Decode196(info [25]byte) ([12]byte, uint8, fec.Result) {
	return DecodeRows(Rows(info))
}

// Rows deinterleaves a BPTC(196,96) block of 196 packed information bits
// into the 13 rows of its grid, as Decode196 does before it corrects them.
func Rows(info [25]byte) [13]uint16 {
	var words [4]uint64
	for k := range 3 {
		words[k] = binary.BigEndian.Uint64(info[8*k:])
	}
	words[3] = uint64(info[24]) << 56
	var rows [13]uint16
	i := 1
	for r := range rows {
		var row uint16
		for c := range 15 {
			p := dm[i]
			row |= uint16(words[p>>6&3]>>(63-p&63)&1) << c
			i++
		}
		rows[r] = row
	}
	return rows
}

// GridPos returns the row and column of the grid that information bit p of
// a block, 0 to 195, lands in, and false for a bit outside the grid.
func GridPos(p int) (row, col int, ok bool) {
	for i := 1; i < len(dm); i++ {
		if int(dm[i]) == p {
			return (i - 1) / 15, (i - 1) % 15, true
		}
	}
	return 0, 0, false
}

// DecodeRows corrects the rows of a BPTC(196,96) grid that Rows returns
// and returns what Decode196 returns for the block.
func DecodeRows(rows [13]uint16) ([12]byte, uint8, fec.Result) {
	received := rows
	result := fec.Result{BitsChecked: 196, Uncorrectable: hammingCorrect(&rows)}
	for r := range rows {
		result.ErrorsCorrected += bits.OnesCount16(rows[r] ^ received[r])
	}

	var head, tail uint64
	head = uint64(bits.Reverse16(rows[0]) >> 5 & 0xFF)
	for r := 1; r <= 5; r++ {
		head = head<<11 | uint64(bits.Reverse16(rows[r])>>5&0x7FF)
	}
	for r := 6; r <= 8; r++ {
		tail = tail<<11 | uint64(bits.Reverse16(rows[r])>>5&0x7FF)
	}
	var data [12]byte
	binary.BigEndian.PutUint64(data[:8], head<<1|tail>>32)
	binary.BigEndian.PutUint32(data[8:], uint32(tail&0xFFFFFFFF))

	reserved := uint8((rows[0]>>2&1 | rows[0]>>1&1<<1 | rows[0]&1<<2) & 0x7)
	return data, reserved, result
}

// hammingCorrect corrects the BPTC(196,96) grid with the column
// Hamming(13,9,3) and row Hamming(15,11,3) codes, columns first in each pass
// as MMDVMHost does. The column parity rows are also row codewords, so all
// 13 rows are row-decoded. It reports whether a row or column syndrome is
// still nonzero. The corrections of the columns of one pass touch different
// columns, and those of the rows different rows, so each step computes its
// syndromes once.
func hammingCorrect(rows *[13]uint16) bool {
	for range 5 {
		fixing := false

		s := columnSyndromes(rows)
		for bad := (s[0] | s[1] | s[2] | s[3]) & gridRowMask; bad != 0; bad &= bad - 1 {
			c := bits.TrailingZeros16(bad)
			syndrome := int(s[0]>>c&1 | s[1]>>c&1<<1 | s[2]>>c&1<<2 | s[3]>>c&1<<3)
			if bitPos := hamming13_9_syndrome_table[syndrome]; bitPos != -1 {
				rows[bitPos] ^= 1 << c
				fixing = true
			}
		}

		for r := range rows {
			if bitPos := hamming15_11_syndrome_table[rowSyndrome15_11(rows[r])]; bitPos != -1 {
				rows[r] ^= 1 << bitPos
				fixing = true
			}
		}

		if !fixing {
			break
		}
	}

	for r := range rows {
		if rowSyndrome15_11(rows[r]) != 0 {
			return true
		}
	}
	s := columnSyndromes(rows)
	return (s[0]|s[1]|s[2]|s[3])&gridRowMask != 0
}

// Encode196 encodes 96 packed data bits with BPTC(196,96) and returns the
// 196 packed information bits in transmit order, bits 196 to 199 zero. The
// reserved bits R(0) to R(2) come from bits 0 to 2 of reserved, and R(3) is
// zero.
func Encode196(data [12]byte, reserved uint8) [25]byte {
	head := binary.BigEndian.Uint64(data[:8])
	tail := uint64(binary.BigEndian.Uint32(data[8:]))
	var rows [13]uint16
	rows[0] = uint16(reserved>>2&1|reserved>>1&1<<1|reserved&1<<2) | bits.Reverse16(uint16(head>>56)<<5)
	for r := 1; r <= 5; r++ {
		rows[r] = bits.Reverse16(uint16(head>>(56-11*r)&0x7FF) << 5)
	}
	rest := head&1<<32 | tail
	for r := 6; r <= 8; r++ {
		rows[r] = bits.Reverse16(uint16(rest>>(33-11*(r-5))&0x7FF) << 5)
	}
	for r := range 9 {
		rows[r] |= uint16(rowSyndrome15_11(rows[r])&0xF) << 11
	}
	s := columnSyndromes(&rows)
	rows[9], rows[10], rows[11], rows[12] = s[0], s[1], s[2], s[3]

	var words [4]uint64
	i := 1
	for _, row := range rows {
		for c := range 15 {
			p := dm[i]
			words[p>>6&3] |= uint64(row>>c&1) << (63 - p&63)
			i++
		}
	}
	var info [25]byte
	for k := range 3 {
		binary.BigEndian.PutUint64(info[8*k:], words[k])
	}
	info[24] = byte(words[3] >> 56)
	return info
}
