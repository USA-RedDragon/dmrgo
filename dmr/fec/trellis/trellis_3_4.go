package trellis34

import (
	"fmt"
	"math"
)

var interleaveMatrix = []byte{
	0, 1, 8, 9, 16, 17, 24, 25, 32, 33, 40, 41, 48, 49, 56, 57, 64, 65, 72, 73, 80, 81, 88, 89, 96, 97,
	2, 3, 10, 11, 18, 19, 26, 27, 34, 35, 42, 43, 50, 51, 58, 59, 66, 67, 74, 75, 82, 83, 90, 91,
	4, 5, 12, 13, 20, 21, 28, 29, 36, 37, 44, 45, 52, 53, 60, 61, 68, 69, 76, 77, 84, 85, 92, 93,
	6, 7, 14, 15, 22, 23, 30, 31, 38, 39, 46, 47, 54, 55, 62, 63, 70, 71, 78, 79, 86, 87, 94, 95,
}

var encoderStateTransition = []byte{
	0, 8, 4, 12, 2, 10, 6, 14,
	4, 12, 2, 10, 6, 14, 0, 8,
	1, 9, 5, 13, 3, 11, 7, 15,
	5, 13, 3, 11, 7, 15, 1, 9,
	3, 11, 7, 15, 1, 9, 5, 13,
	7, 15, 1, 9, 5, 13, 3, 11,
	2, 10, 6, 14, 0, 8, 4, 12,
	6, 14, 0, 8, 4, 12, 2, 10,
}

var constellationPoints = map[[2]int8]byte{
	{1, -1}:  0,
	{-1, -1}: 1,
	{3, -3}:  2,
	{-3, -3}: 3,
	{-3, -1}: 4,
	{3, -1}:  5,
	{-1, -3}: 6,
	{1, -3}:  7,
	{-3, 3}:  8,
	{3, 3}:   9,
	{-1, 1}:  10,
	{1, 1}:   11,
	{1, 3}:   12,
	{-1, 3}:  13,
	{3, 1}:   14,
	{-3, 1}:  15,
}

var constellationPointsReverse = func() map[byte][2]int8 {
	reverseMap := make(map[byte][2]int8)
	for k, v := range constellationPoints {
		reverseMap[v] = k
	}
	return reverseMap
}()

type Trellis34 struct {
}

func New() *Trellis34 {
	return &Trellis34{}
}

func (t *Trellis34) dibitsToBits(dibits [98]int8) [196]byte {
	var bits [196]byte

	for i := 0; i < 98; i++ {
		o := i * 2
		switch dibits[i] {
		case 3:
			bits[o] = 0
			bits[o+1] = 1
		case 1:
			bits[o] = 0
			bits[o+1] = 0
		case -1:
			bits[o] = 1
			bits[o+1] = 0
		case -3:
			bits[o] = 1
			bits[o+1] = 1
		}
	}

	return bits
}

func (t *Trellis34) bitsToDibits(bits [196]byte) [98]int8 {
	var dibits [98]int8

	for i := 0; i < 196; i += 2 {
		o := i / 2
		switch {
		case bits[i] == 0 && bits[i+1] == 1:
			dibits[o] = 3
		case bits[i] == 0 && bits[i+1] == 0:
			dibits[o] = 1
		case bits[i] == 1 && bits[i+1] == 0:
			dibits[o] = -1
		case bits[i] == 1 && bits[i+1] == 1:
			dibits[o] = -3
		}
	}

	return dibits
}

func (t *Trellis34) Decode(bits [196]byte) [144]byte {
	dibits := t.bitsToDibits(bits)
	deinterleavedDibits := t.deinterleave(dibits)
	points := t.dibitsToPoints(deinterleavedDibits)
	tribits := t.pointsToTribits(points)
	decoded := t.tribitsToBits(tribits)

	return decoded
}

func (t *Trellis34) deinterleave(dibits [98]int8) [98]int8 {
	var out [98]int8

	for i := 0; i < 196; i++ {
		out[interleaveMatrix[i]] = dibits[i]
	}

	return out
}

func (t *Trellis34) dibitsToPoints(dibits [98]int8) [49]byte {
	var points [49]byte

	for i := 0; i < 98; i += 2 {
		o := int(i / 2)
		points[o] = constellationPoints[[2]int8{dibits[i], dibits[i+1]}]
	}

	return points
}

func (t *Trellis34) pointsToTribits(points [49]byte) [49]byte {
	var tribits [49]byte
	last := byte(0)

	for i := 0; i < 49; i++ {
		start := last * 8
		matches := false

		for j := start; j < start+8; j++ {
			if points[i] == encoderStateTransition[j] {
				matches = true
				last = byte(math.Abs(float64((j - start) % 255)))
				tribits[i] = last
			}
		}

		if !matches {
			fmt.Printf("Trellis data corrupted, index %d constellation point %v\n", i, points[i])
		}
	}

	return tribits
}

func (t *Trellis34) tribitsToBits(tribits [49]byte) [144]byte {
	var bits [144]byte

	for i := 0; i < 144; i += 3 {
		o := i / 3
		if (tribits[o] & 0x4) > 0 {
			bits[i] = 1
		} else {
			bits[i] = 0
		}

		if (tribits[o] & 0x2) > 0 {
			bits[i+1] = 1
		} else {
			bits[i+1] = 0
		}

		if (tribits[o] & 0x1) > 0 {
			bits[i+2] = 1
		} else {
			bits[i+2] = 0
		}
	}

	return bits
}
