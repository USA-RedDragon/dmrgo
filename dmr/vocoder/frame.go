package vocoder

import (
	"fmt"

	"github.com/USA-RedDragon/dmrgo/dmr/vocoder/fec"
)

type VocoderFrame struct {
	DecodedBits [49]byte
}

func (vf *VocoderFrame) ToString() string {
	return fmt.Sprintf("{ DecodedBits: %x }", vf.DecodedBits)
}

var aTable = []int{0, 4, 8, 12, 16, 20, 24, 28, 32, 36, 40, 44,
	48, 52, 56, 60, 64, 68, 1, 5, 9, 13, 17, 21}

var bTable = []int{25, 29, 33, 37, 41, 45, 49, 53, 57, 61, 65, 69,
	2, 6, 10, 14, 18, 22, 26, 30, 34, 38, 42}

var cTable = []int{46, 50, 54, 58, 62, 66, 70, 3, 7, 11, 15, 19,
	23, 27, 31, 35, 39, 43, 47, 51, 55, 59, 63, 67, 71}

func NewVocoderFrameFromBits(bits [72]byte) *VocoderFrame {
	var ambe49 [49]byte

	var a uint32 = 0
	var MASK uint32 = 0x800000
	ambe49[9] = 0

	for i := 0; i < 24; i, MASK = i+1, MASK>>1 {
		aPos := aTable[i]
		if bits[aPos] == 1 {
			a |= MASK
		}
	}

	var b uint32 = 0
	MASK = 0x400000
	for i := 0; i < 23; i, MASK = i+1, MASK>>1 {
		bPos := bTable[i]
		if bits[bPos] == 1 {
			b |= MASK
		}
	}

	var c uint32 = 0
	MASK = 0x1000000
	for i := 0; i < 25; i, MASK = i+1, MASK>>1 {
		cPos := cTable[i]
		if bits[cPos] == 1 {
			c |= MASK
		}
	}

	a >>= 12

	// The PRNG
	b ^= (fec.PRNG_TABLE[a] >> 1)
	b >>= 11

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

	vf := VocoderFrame{
		DecodedBits: ambe49,
	}

	return &vf
}
