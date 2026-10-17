package bptc

import (
	"math/rand"
	"testing"
)

func TestEncodeRowsAndGridBit(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	for range 200 {
		var data [12]byte
		r.Read(data[:])
		reserved := uint8(r.Intn(8))
		if got, want := EncodeRows(data, reserved), Rows(Encode196(data, reserved)); got != want {
			t.Fatalf("EncodeRows(%x, %d) = %x, want %x", data, reserved, got, want)
		}
	}
	for p := range 196 {
		row, col, ok := GridPos(p)
		if ok != (p != 0) {
			t.Fatalf("GridPos(%d) ok = %v", p, ok)
		}
		if ok && GridBit(row, col) != p {
			t.Fatalf("GridBit(GridPos(%d)) = %d", p, GridBit(row, col))
		}
	}
}
