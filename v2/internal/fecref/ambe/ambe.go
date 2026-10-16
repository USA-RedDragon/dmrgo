// Package ambe is the bit-per-byte AMBE+2 frame FEC that internal/fec/ambe
// replaces, for differential tests.
package ambe

import (
	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fecref/golay"
)

// Encode takes the decoded 49 bits and encodes them into a 72 bit AMBE frame.
func Encode(ambe49 [49]bit.Bit) [72]bit.Bit {
	var ambe72 [72]bit.Bit

	var aOrig uint32
	var bOrig uint32
	var cOrig uint32
	var MASK uint32 = 0x000800

	for i := 0; i < 12; i, MASK = i+1, MASK>>1 {
		n1 := i
		n2 := i + 12
		if ambe49[n1] == 1 {
			aOrig |= MASK
		}
		if ambe49[n2] == 1 {
			bOrig |= MASK
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		n := i + 24
		if ambe49[n] == 1 {
			cOrig |= MASK
		}
	}

	a := golay.Encode24128(uint16(aOrig))

	p := ambeScrambleTable[aOrig] >> 1

	b := golay.Encode23127(uint16(bOrig))
	b ^= p

	MASK = 0x800000
	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if (a & MASK) != 0 {
			ambe72[aPos] = 1
		} else {
			ambe72[aPos] = 0
		}
	}

	MASK = 0x400000
	for i := 0; i < 23; i, MASK = i+1, MASK>>1 {
		bPos := bTable[i]
		if (b & MASK) != 0 {
			ambe72[bPos] = 1
		} else {
			ambe72[bPos] = 0
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := cTable[i]
		if (cOrig & MASK) != 0 {
			ambe72[cPos] = 1
		} else {
			ambe72[cPos] = 0
		}
	}

	return ambe72
}

// Decode decodes a 72-bit AMBE frame into its 49 information bits.
// Section A is Golay (24,12,8), section B is scrambled Golay (23,12,7), and
// section C has no FEC.
func Decode(bits [72]bit.Bit) ([49]bit.Bit, fec.Result) {
	var ambe49 [49]bit.Bit
	result := fec.Result{BitsChecked: 47}

	var a uint32
	var MASK uint32 = 0x800000

	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if bits[aPos] == 1 {
			a |= MASK
		}
	}

	dataA, fecA := golay.Decode24128(a)
	result.ErrorsCorrected += fecA.ErrorsCorrected
	if fecA.Uncorrectable {
		result.Uncorrectable = true
	}
	a = uint32(dataA)

	var b uint32
	MASK = 0x400000
	for i := 0; i < 23; i, MASK = i+1, MASK>>1 {
		bPos := bTable[i]
		if bits[bPos] == 1 {
			b |= MASK
		}
	}

	// The section B scrambling sequence depends on the corrected section A data.
	b ^= (ambeScrambleTable[a] >> 1)

	dataB, fecB := golay.Decode23127(b)
	result.ErrorsCorrected += fecB.ErrorsCorrected
	if fecB.Uncorrectable {
		result.Uncorrectable = true
	}
	b = uint32(dataB)

	var c uint32
	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := cTable[i]
		if bits[cPos] == 1 {
			c |= MASK
		}
	}

	MASK = 0x000800
	for i := 0; i < 12; i, MASK = i+1, MASK>>1 {
		apos := i
		bpos := i + 12
		if (a & MASK) != 0 {
			ambe49[apos] = 1
		} else {
			ambe49[apos] = 0
		}
		if (b & MASK) != 0 {
			ambe49[bpos] = 1
		} else {
			ambe49[bpos] = 0
		}
	}

	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := i + 24
		if (c & MASK) != 0 {
			ambe49[cPos] = 1
		} else {
			ambe49[cPos] = 0
		}
	}

	return ambe49, result
}
