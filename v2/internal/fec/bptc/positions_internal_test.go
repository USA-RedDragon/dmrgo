package bptc

import (
	"math/bits"
	"testing"
)

const (
	h1511Pos = 0x358b271f4a6e9dc0
	h139Pos  = 0x1369050d284c7ba0
)

func nibblePos(positions uint64, syndrome int) int {
	return int(positions>>(4*(syndrome&15))&0xF) - 1
}

func checkPositions(t *testing.T, name string, n, syndromes int, syndrome func(p int) int, pos func(s int) int) {
	t.Helper()
	want := make([]int, syndromes)
	for s := range want {
		want[s] = -1
	}
	for p := range n {
		want[syndrome(p)] = p
	}
	for s, w := range want {
		if got := pos(s); got != w {
			t.Errorf("%s syndrome %d: position %d, want %d", name, s, got, w)
		}
	}
}

func TestHammingPositionsLocateSingleErrors(t *testing.T) {
	t.Parallel()

	checkPositions(t, "15,11", 15, 16, func(p int) int { return rowSyndrome15_11(1 << p) },
		func(s int) int { return nibblePos(h1511Pos, s) })
	checkPositions(t, "13,9", 13, 16, func(p int) int {
		var rows [13]uint16
		rows[p] = 1
		c := columnSyndromes(&rows)
		return int(c[0]&1 | c[1]&1<<1 | c[2]&1<<2 | c[3]&1<<3)
	}, func(s int) int { return nibblePos(h139Pos, s) })
	for s := range 16 {
		if int(h139Fix[s]) != nibblePos(h139Pos, s) || int(h1511Fix[s]) != nibblePos(h1511Pos, s) {
			t.Errorf("syndrome %d: tables %d %d, positions %d %d", s, h139Fix[s], h1511Fix[s], nibblePos(h139Pos, s), nibblePos(h1511Pos, s))
		}
	}
	checkPositions(t, "16,11", 16, 32, func(p int) int { return rowSyndrome16_11(1 << p) },
		func(s int) int { return wordPos(h1611PosLo, h1611PosHi, h1611PosTop, s) })
	checkPositions(t, "17,12", 17, 32, func(p int) int { return rowSyndrome17_12(1 << p) },
		func(s int) int { return wordPos(h1712PosLo, h1712PosHi, h1712PosTop, s) })
}

func TestEmbeddedLCColumnsAndWeights(t *testing.T) {
	t.Parallel()

	for c := range 16 {
		if got, want := int(column16_11(c)), rowSyndrome16_11(1<<c); got != want {
			t.Errorf("column %d: syndrome %d, want %d", c, got, want)
		}
	}
	least := [32]int{}
	for s := range least {
		least[s] = 17
	}
	for p := range 1 << 16 {
		s := rowSyndrome16_11(uint16(p))
		least[s] = min(least[s], bits.OnesCount16(uint16(p)))
	}
	for s, want := range least {
		if got := minWeight16_11(s); got != want {
			t.Errorf("syndrome %d: least weight %d, want %d", s, got, want)
		}
	}
}
