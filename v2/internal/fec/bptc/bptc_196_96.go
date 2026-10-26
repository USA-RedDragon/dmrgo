// Package bptc encodes and decodes the block product turbo codes of DMR:
// BPTC(196,96) for full bursts, the BPTC(128,77) of the embedded signalling,
// the CACH Short LC BPTC and the single burst BPTC. The codecs take and
// return packed bits: a bit vector in bytes or a k-bit value in an unsigned
// integer, the first bit most significant.
// (ETSI TS 102 361-1 clauses B.1 and B.2)
package bptc

import (
	"encoding/binary"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 clause B.1.1

const (
	h1511Check0 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11
	h1511Check1 = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<6 | 1<<8 | 1<<9 | 1<<12
	h1511Check2 = 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<9 | 1<<10 | 1<<13
	h1511Check3 = 1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10 | 1<<14
)

const gridRowMask = 1<<15 - 1

const (
	h1511Pos = 0x358b271f4a6e9dc0
	h139Pos  = 0x1369050d284c7ba0
)

func nibblePos(positions uint64, syndrome int) int {
	return int(positions>>(4*(syndrome&15))&0xF) - 1
}

func rowSyndrome15_11(row uint16) int {
	return bits.OnesCount16(row&h1511Check0)&1 | bits.OnesCount16(row&h1511Check1)&1<<1 |
		bits.OnesCount16(row&h1511Check2)&1<<2 | bits.OnesCount16(row&h1511Check3)&1<<3
}

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
// and a fec.Result. R(3) has no FEC and is not returned. Bits 196 to 199 of
// info are ignored. (ETSI TS 102 361-1 clause B.1.1)
func Decode196(info [25]byte) ([12]byte, uint8, fec.Result) {
	rows := Rows(info)
	return DecodeRows(&rows)
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
	for i := 1; i < 196; i++ {
		if int(dm[i]) == p {
			return (i - 1) / 15, (i - 1) % 15, true
		}
	}
	return 0, 0, false
}

// GridBit returns the information bit of a block, 1 to 195, that lands in
// row and col of the grid.
func GridBit(row, col int) int {
	return int(dm[1+15*row+col])
}

// DecodeRows corrects a copy of the rows of a BPTC(196,96) grid that Rows
// returns and returns what Decode196 returns for the block.
//
// It is not inlined, so that its locals are on the stack only while it
// runs, for microcontrollers.
//
//go:noinline
func DecodeRows(grid *[13]uint16) ([12]byte, uint8, fec.Result) {
	rows, received := *grid, *grid
	result := fec.Result{BitsChecked: 196, Uncorrectable: hammingCorrect(&rows)}
	for r := range rows {
		result.ErrorsCorrected += bits.OnesCount16(rows[r] ^ received[r])
	}

	var data [12]byte
	reserved := rowsData(&rows, &data)
	return data, reserved, result
}

// Recode corrects the rows of a BPTC(196,96) grid in place, writes the
// data bits into data, re-encodes the rows from them and returns the
// reserved bits.
//
//go:noinline
func Recode(rows *[13]uint16, data *[12]byte) uint8 {
	hammingCorrect(rows)
	reserved := rowsData(rows, data)
	encodeRows(rows, data, reserved)
	return reserved
}

func rowsData(rows *[13]uint16, data *[12]byte) uint8 {
	var head, tail uint64
	head = uint64(bits.Reverse16(rows[0]) >> 5 & 0xFF)
	for r := 1; r <= 5; r++ {
		head = head<<11 | uint64(bits.Reverse16(rows[r])>>5&0x7FF)
	}
	for r := 6; r <= 8; r++ {
		tail = tail<<11 | uint64(bits.Reverse16(rows[r])>>5&0x7FF)
	}
	binary.BigEndian.PutUint64(data[:8], head<<1|tail>>32)
	binary.BigEndian.PutUint32(data[8:], uint32(tail&0xFFFFFFFF))
	return uint8((rows[0]>>2&1 | rows[0]>>1&1<<1 | rows[0]&1<<2) & 0x7)
}

func hammingCorrect(rows *[13]uint16) bool {
	for range 5 {
		fixing := false

		s := columnSyndromes(rows)
		for bad := (s[0] | s[1] | s[2] | s[3]) & gridRowMask; bad != 0; bad &= bad - 1 {
			c := bits.TrailingZeros16(bad)
			syndrome := int(s[0]>>c&1 | s[1]>>c&1<<1 | s[2]>>c&1<<2 | s[3]>>c&1<<3)
			if bitPos := nibblePos(h139Pos, syndrome); bitPos != -1 {
				rows[bitPos] ^= 1 << c
				fixing = true
			}
		}

		for r := range rows {
			if bitPos := nibblePos(h1511Pos, rowSyndrome15_11(rows[r])); bitPos != -1 {
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
	rows := EncodeRows(&data, reserved)
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

// EncodeRows returns the 13 grid rows of the BPTC(196,96) codeword of data
// and reserved, which Rows returns for the block of Encode196.
func EncodeRows(data *[12]byte, reserved uint8) [13]uint16 {
	var rows [13]uint16
	encodeRows(&rows, data, reserved)
	return rows
}

func encodeRows(rows *[13]uint16, data *[12]byte, reserved uint8) {
	head := binary.BigEndian.Uint64(data[:8])
	tail := uint64(binary.BigEndian.Uint32(data[8:]))
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
	rows[9], rows[10], rows[11], rows[12] = 0, 0, 0, 0
	s := columnSyndromes(rows)
	rows[9], rows[10], rows[11], rows[12] = s[0], s[1], s[2], s[3]
}
