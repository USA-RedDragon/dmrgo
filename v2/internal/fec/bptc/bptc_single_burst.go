package bptc

import (
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
)

// ETSI TS 102 361-1 clause B.2.2

// DecodeSingleBurst decodes the 11 information bits from the 32-bit embedded
// data field tx of one burst, transmit bit 0 most significant, and returns
// them in the low 11 bits, the first most significant. Set oddParity for a
// Reverse Channel burst. It corrects up to 3 bit errors, and it always
// detects 4 and sets Uncorrectable.
func DecodeSingleBurst(tx uint32, oddParity bool) (uint16, fec.Result) {
	result := fec.Result{BitsChecked: 32}

	u := bits.Reverse32(tx)
	row0, row1 := compactEven(u), bits.RotateLeft16(compactEven(u>>1), 8)
	if oddParity {
		row1 = ^row1
	}

	// clause B.2.2.1; clause B.2.2.2
	best, bestDist := row0, 33
	for _, row := range [2]uint16{row0, row1} {
		candidate, ok := correctHamming16_11(row)
		if !ok {
			continue
		}
		if d := bits.OnesCount16(candidate^row0) + bits.OnesCount16(candidate^row1); d < bestDist {
			best, bestDist = candidate, d
		}
	}

	if bestDist > 3 {
		result.Uncorrectable = true
	} else {
		row0 = best
		result.ErrorsCorrected = bestDist
	}
	return bits.Reverse16(row0) >> 5, result
}

// EncodeSingleBurst encodes the 11 information bits in the low bits of info,
// the first most significant, into the 32-bit embedded data field of one
// burst, transmit bit 0 most significant. Set oddParity for a Reverse
// Channel burst.
func EncodeSingleBurst(info uint16, oddParity bool) uint32 {
	row0 := rowParity16_11(bits.Reverse16(info << 5))
	row1 := row0
	if oddParity {
		row1 = ^row1
	}
	return bits.Reverse32(spreadEven(row0) | spreadEven(bits.RotateLeft16(row1, 8))<<1)
}

func compactEven(x uint32) uint16 {
	x &= 0x55555555
	x = (x | x>>1) & 0x33333333
	x = (x | x>>2) & 0x0F0F0F0F
	x = (x | x>>4) & 0x00FF00FF
	return uint16((x | x>>8) & 0xFFFF)
}

func spreadEven(x uint16) uint32 {
	v := uint32(x)
	v = (v | v<<8) & 0x00FF00FF
	v = (v | v<<4) & 0x0F0F0F0F
	v = (v | v<<2) & 0x33333333
	return (v | v<<1) & 0x55555555
}

func correctHamming16_11(row uint16) (uint16, bool) {
	s := rowSyndrome16_11(row)
	if s == 0 {
		return row, true
	}
	errPos := hamming16_11_syndrome_table[s]
	if errPos < 0 {
		return row, false
	}
	return row ^ 1<<errPos, true
}

// ETSI TS 102 361-1 clause B.3.4, Table B.16
const (
	h1611Check0 = 1<<0 | 1<<1 | 1<<2 | 1<<3 | 1<<5 | 1<<7 | 1<<8 | 1<<11
	h1611Check1 = 1<<1 | 1<<2 | 1<<3 | 1<<4 | 1<<6 | 1<<8 | 1<<9 | 1<<12
	h1611Check2 = 1<<2 | 1<<3 | 1<<4 | 1<<5 | 1<<7 | 1<<9 | 1<<10 | 1<<13
	h1611Check3 = 1<<0 | 1<<1 | 1<<2 | 1<<4 | 1<<6 | 1<<7 | 1<<10 | 1<<14
	h1611Check4 = 1<<0 | 1<<2 | 1<<5 | 1<<6 | 1<<8 | 1<<9 | 1<<10 | 1<<15
)

func rowSyndrome16_11(row uint16) int {
	return bits.OnesCount16(row&h1611Check0)&1 | bits.OnesCount16(row&h1611Check1)&1<<1 |
		bits.OnesCount16(row&h1611Check2)&1<<2 | bits.OnesCount16(row&h1611Check3)&1<<3 |
		bits.OnesCount16(row&h1611Check4)&1<<4
}

func rowParity16_11(row uint16) uint16 {
	row &= 1<<11 - 1
	s := rowSyndrome16_11(row)
	return row | uint16(s&0x1F)<<11
}
