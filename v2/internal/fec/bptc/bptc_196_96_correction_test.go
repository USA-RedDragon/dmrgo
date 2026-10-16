package bptc

import (
	"math/bits"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func gridSyndromesZero(rows *[13]uint16) bool {
	s := columnSyndromes(rows)
	if (s[0]|s[1]|s[2]|s[3])&gridRowMask != 0 {
		return false
	}
	for r := range rows {
		if rowSyndrome15_11(rows[r]) != 0 {
			return false
		}
	}
	return true
}

func randomBPTC19696(r *rand.Rand) ([96]unpacked.Bit, [196]unpacked.Bit) {
	var data [96]unpacked.Bit
	for i := range data {
		data[i] = unpacked.Bit(r.Intn(2))
	}
	return data, encode196Bits(data)
}

func TestBPTC19696_EncodedGridIsProductCodeword(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(6))
	for n := 0; n < 1000; n++ {
		_, enc := randomBPTC19696(r)
		var g [196]unpacked.Bit
		for i := range g {
			g[i] = enc[dm[i]]
		}
		if rows := gridRows(&g); !gridSyndromesZero(&rows) {
			t.Fatalf("encoded grid has a nonzero row or column syndrome")
		}
	}
}

func TestBPTC19696_AllDoubleErrorsCorrected(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(10))
	data, enc := randomBPTC19696(r)
	for p := 1; p < 196; p++ {
		for q := p + 1; q < 196; q++ {
			rx := enc
			rx[dm[p]] ^= 1
			rx[dm[q]] ^= 1
			got, res := decode196Bits(rx)
			if got != data || res.Uncorrectable || res.ErrorsCorrected != 2 {
				t.Fatalf("grid bits %d,%d: data match=%v result=%+v", p, q, got == data, res)
			}
		}
	}
}

func TestBPTC19696_AcceptedOutputIsCodeword(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(9))
	for n := 0; n < 20000; n++ {
		_, enc := randomBPTC19696(r)
		rx := enc
		for _, p := range r.Perm(196)[:4+r.Intn(6)] {
			rx[p] ^= 1
		}
		var g [196]unpacked.Bit
		for i := range g {
			g[i] = rx[dm[i]]
		}
		received := gridRows(&g)
		rows := received
		if unc := hammingCorrect(&rows); !unc && !gridSyndromesZero(&rows) {
			t.Fatalf("accepted a grid with nonzero syndromes")
		}
		diff := 0
		for r := range rows {
			diff += bits.OnesCount16(rows[r] ^ received[r])
		}
		if _, res := decode196Bits(rx); res.ErrorsCorrected != diff {
			t.Fatalf("ErrorsCorrected %d, but %d bits changed", res.ErrorsCorrected, diff)
		}
	}
}

// TestBPTC19696_ColumnsFirstCorrection checks a 4-error pattern that row
// decoding first turns into a wrong codeword and column decoding first
// corrects.
func TestBPTC19696_ColumnsFirstCorrection(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(11))
	data, enc := randomBPTC19696(r)
	for _, p := range []int{124, 128, 172, 173} {
		enc[dm[p]] ^= 1
	}
	got, res := decode196Bits(enc)
	if got != data || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("data match=%v result=%+v", got == data, res)
	}
}
