package bptc

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func gridSyndromesZero(g *[196]bit.Bit) bool {
	for c := 0; c < 15; c++ {
		if columnSyndrome(g, c) != 0 {
			return false
		}
	}
	for r := 0; r < 13; r++ {
		if rowSyndrome(g, r) != 0 {
			return false
		}
	}
	return true
}

func randomBPTC19696(r *rand.Rand) ([96]bit.Bit, [196]bit.Bit) {
	var data [96]bit.Bit
	for i := range data {
		data[i] = bit.Bit(r.Intn(2))
	}
	return data, Encode196(data)
}

func TestBPTC19696_EncodedGridIsProductCodeword(t *testing.T) {
	r := rand.New(rand.NewSource(6))
	for n := 0; n < 1000; n++ {
		_, enc := randomBPTC19696(r)
		var g [196]bit.Bit
		for i := range g {
			g[i] = enc[dm[i]]
		}
		if !gridSyndromesZero(&g) {
			t.Fatalf("encoded grid has a nonzero row or column syndrome")
		}
	}
}

func TestBPTC19696_AllDoubleErrorsCorrected(t *testing.T) {
	r := rand.New(rand.NewSource(10))
	data, enc := randomBPTC19696(r)
	for p := 1; p < 196; p++ {
		for q := p + 1; q < 196; q++ {
			rx := enc
			rx[dm[p]] ^= 1
			rx[dm[q]] ^= 1
			got, res := Decode196(rx)
			if got != data || res.Uncorrectable || res.ErrorsCorrected != 2 {
				t.Fatalf("grid bits %d,%d: data match=%v result=%+v", p, q, got == data, res)
			}
		}
	}
}

func TestBPTC19696_AcceptedOutputIsCodeword(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	for n := 0; n < 20000; n++ {
		_, enc := randomBPTC19696(r)
		rx := enc
		for _, p := range r.Perm(196)[:4+r.Intn(6)] {
			rx[p] ^= 1
		}
		var g [196]bit.Bit
		for i := range g {
			g[i] = rx[dm[i]]
		}
		corrected, flips, unc := hamming_correct(g)
		if !unc && !gridSyndromesZero(&corrected) {
			t.Fatalf("accepted a grid with nonzero syndromes")
		}
		diff := 0
		for i := range g {
			diff += int(g[i] ^ corrected[i])
		}
		if flips != diff {
			t.Fatalf("ErrorsCorrected %d, but %d bits changed", flips, diff)
		}
	}
}

// TestBPTC19696_ColumnsFirstCorrection checks a 4-error pattern that row
// decoding first turns into a wrong codeword and column decoding first
// corrects.
func TestBPTC19696_ColumnsFirstCorrection(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	data, enc := randomBPTC19696(r)
	for _, p := range []int{124, 128, 172, 173} {
		enc[dm[p]] ^= 1
	}
	got, res := Decode196(enc)
	if got != data || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("data match=%v result=%+v", got == data, res)
	}
}
