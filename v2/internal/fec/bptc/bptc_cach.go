package bptc

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 clause B.2.3

// ETSI TS 102 361-1 Table B.13
const (
	h1712Check0 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<6 | 1<<7 | 1<<9 | 1<<12
	h1712Check1 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<7 | 1<<8 | 1<<10 | 1<<13
	h1712Check2 = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<8 | 1<<9 | 1<<11 | 1<<14
	h1712Check3 = 1<<0 | 1<<1 | 1<<4 | 1<<5 | 1<<7 | 1<<10 | 1<<15
	h1712Check4 = 1<<0 | 1<<1 | 1<<2 | 1<<5 | 1<<6 | 1<<8 | 1<<11 | 1<<16
)

const cachRowMask = 1<<17 - 1

const (
	h1712PosLo  = 0x05008b0040af0ed0
	h1712PosHi  = 0x20061000390c0071
	h1712PosTop = 0x00010100
)

func rowSyndrome17_12(row uint32) int {
	return bits.OnesCount32(row&h1712Check0)&1 | bits.OnesCount32(row&h1712Check1)&1<<1 |
		bits.OnesCount32(row&h1712Check2)&1<<2 | bits.OnesCount32(row&h1712Check3)&1<<3 |
		bits.OnesCount32(row&h1712Check4)&1<<4
}

// DecodeCACH decodes a 36-bit Short LC PDU (28 data bits and a CRC-8) from
// the 17-bit signalling payloads of CACH bursts 0 to 3, in that order, each
// in the low 17 bits with its first bit most significant. It returns the PDU
// in the low 36 bits, the first most significant, with a fec.Result.
func DecodeCACH(fragments [4]uint32) (uint64, fec.Result) {
	result := fec.Result{BitsChecked: 68}

	for i := range fragments {
		fragments[i] &= cachRowMask
	}
	head := uint64(fragments[0])<<47 | uint64(fragments[1])<<30 | uint64(fragments[2])<<13 | uint64(fragments[3]>>4)
	var rows [4]uint32
	for r := range rows {
		rows[r] = uint32(bits.Reverse16(compactFourth(head>>(3-r)))) | (fragments[3]>>(3-r)&1)<<16
	}

	received := rows
	correctCACHMatrix(&rows)

	for row := range rows {
		result.ErrorsCorrected += bits.OnesCount32(rows[row] ^ received[row])
	}
	result.Uncorrectable = !cachMatrixValid(&rows)

	var info uint64
	for row := range 3 {
		info = info<<12 | uint64(bits.Reverse16(uint16(rows[row]&0xFFF))>>4)
	}
	return info, result
}

func correctCACHMatrix(rows *[4]uint32) {
	for range 3 {
		var syndromes [4]int
		badRow, badRows := 0, 0
		for row := range rows {
			syndromes[row] = rowSyndrome17_12(rows[row])
			if syndromes[row] != 0 {
				badRow = row
				badRows++
			}
		}

		fixing := false
		if badRows == 1 {
			if parity := cachColumnParity(rows); parity != 0 {
				rows[badRow] ^= parity
				fixing = true
			}
		}
		if !fixing {
			for row := range rows {
				if syndromes[row] == 0 {
					continue
				}
				if errPos := wordPos(h1712PosLo, h1712PosHi, h1712PosTop, syndromes[row]); errPos >= 0 {
					rows[row] ^= 1 << errPos
					fixing = true
				}
			}
		}

		if !fixing {
			return
		}
	}
}

func cachColumnParity(rows *[4]uint32) uint32 {
	return (rows[0] ^ rows[1] ^ rows[2] ^ rows[3]) & cachRowMask
}

func cachMatrixValid(rows *[4]uint32) bool {
	for row := range rows {
		if rowSyndrome17_12(rows[row]) != 0 {
			return false
		}
	}
	return cachColumnParity(rows) == 0
}

// EncodeCACH encodes the 36-bit Short LC PDU in the low bits of info, the
// first most significant, into the 17-bit signalling payloads of 4 CACH
// bursts, as DecodeCACH takes them.
func EncodeCACH(info uint64) [4]uint32 {
	var rows [4]uint32
	for row := range 3 {
		data := uint32(bits.Reverse16(uint16(info>>(24-12*row)&0xFFF) << 4))
		rows[row] = data | uint32(rowSyndrome17_12(data)&0x1F)<<12
	}
	rows[3] = rows[0] ^ rows[1] ^ rows[2]

	var head uint64
	var tail uint32
	for r, row := range rows {
		head |= spreadFourth(bits.Reverse16(uint16(row&0xFFFF))) << (3 - r)
		tail |= (row >> 16 & 1) << (3 - r)
	}
	return [4]uint32{
		uint32(head >> 47 & cachRowMask), uint32(head >> 30 & cachRowMask), uint32(head >> 13 & cachRowMask),
		uint32(head&(1<<13-1))<<4 | tail,
	}
}

func compactFourth(x uint64) uint16 {
	x &= 0x1111111111111111
	x = (x | x>>3) & 0x0303030303030303
	x = (x | x>>6) & 0x000F000F000F000F
	x = (x | x>>12) & 0x000000FF000000FF
	return uint16((x | x>>24) & 0xFFFF)
}

func spreadFourth(x uint16) uint64 {
	v := uint64(x)
	v = (v | v<<24) & 0x000000FF000000FF
	v = (v | v<<12) & 0x000F000F000F000F
	v = (v | v<<6) & 0x0303030303030303
	return (v | v<<3) & 0x1111111111111111
}
