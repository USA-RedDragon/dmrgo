package reedsolomon

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/reedsolomon"
)

type reedSolomonSeed struct {
	data []byte
	mask []byte
}

func reedSolomonSeeds() []reedSolomonSeed {
	return []reedSolomonSeed{
		{[]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x09, 0x00, 0x30, 0x39}, []byte{}},
		{[]byte{0x00, 0x10, 0x00, 0x00, 0x00, 0x09, 0x00, 0x30, 0x39}, []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0x81}},
		{[]byte{0x03, 0x00, 0x00, 0x00, 0x2a, 0x00, 0x30, 0x39, 0x00}, []byte{0, 0x04, 0, 0, 0, 0, 0x40}},
		{[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, []byte{0x11, 0x22, 0x33}},
	}
}

func checkReedSolomon(t testing.TB, data []byte, mask []byte) {
	t.Helper()
	var d [9]byte
	copy(d[:], data)
	var e [12]byte
	copy(e[:], mask)

	cw := Encode(d)
	if want := ref.Encode(d); cw != want {
		t.Fatalf("Encode(%x) = %x, reference %x", d, cw, want)
	}
	var rx [12]byte
	symbols, n := 0, 0
	for i := range rx {
		rx[i] = cw[i] ^ e[i]
		if e[i] != 0 {
			symbols++
			n += bits.OnesCount8(e[i])
		}
	}
	got, res := Decode(rx)
	switch {
	case symbols <= 1 && (got != d || res != (fec.Result{BitsChecked: 96, ErrorsCorrected: n})):
		t.Fatalf("errors %x: got %x %+v, want %x", e, got, res, d)
	case symbols == 2 && !res.Uncorrectable:
		t.Fatalf("errors %x: accepted %x %+v", e, got, res)
	}
	if res.Uncorrectable {
		if got != [9]byte(rx[:9]) || res.ErrorsCorrected != 0 {
			t.Fatalf("Decode(%x) = %x %+v, want the received data", rx, got, res)
		}
	} else {
		dec := Encode(got)
		changed, dist := 0, 0
		for i := range dec {
			if dec[i] != rx[i] {
				changed++
				dist += bits.OnesCount8(dec[i] ^ rx[i])
			}
		}
		if changed > 1 || dist != res.ErrorsCorrected {
			t.Fatalf("Decode(%x) = %x %+v: codeword %d symbols and %d bits away", rx, got, res, changed, dist)
		}
	}
	if wantData, wantRes := ref.Decode(rx); got != wantData || res != wantRes {
		t.Fatalf("Decode(%x) = %x %+v, reference %x %+v", rx, got, res, wantData, wantRes)
	}
	ok := Check(rx)
	if ok != ref.Check(rx) || symbols <= 3 && ok != (symbols == 0) {
		t.Fatalf("Check(%x) = %v with %d symbol errors", rx, ok, symbols)
	}
}

func FuzzReedSolomon(f *testing.F) {
	for _, s := range reedSolomonSeeds() {
		f.Add(s.data, s.mask)
	}
	f.Fuzz(func(t *testing.T, data []byte, mask []byte) {
		checkReedSolomon(t, data, mask)
	})
}
