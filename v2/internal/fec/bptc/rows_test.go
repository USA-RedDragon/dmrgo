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
		if got, want := EncodeRows(&data, reserved), Rows(Encode196(data, reserved)); got != want {
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

func TestRecode(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	for i := range 2000 {
		var data [12]byte
		r.Read(data[:])
		grid := EncodeRows(&data, uint8(r.Intn(8)))
		for range i % 12 {
			grid[r.Intn(13)] ^= 1 << r.Intn(15)
		}
		wantData, wantReserved, _ := DecodeRows(&grid)
		want := EncodeRows(&wantData, wantReserved)
		var got [12]byte
		rows := grid
		if reserved := Recode(&rows, &got); got != wantData || reserved != wantReserved || rows != want {
			t.Fatalf("Recode(%x) = %x, %d, %x, want %x, %d, %x", grid, got, reserved, rows, wantData, wantReserved, want)
		}
	}
}
