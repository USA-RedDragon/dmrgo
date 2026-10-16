package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
)

func cachInfo() [36]bit.Bit {
	var info [36]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i*5 + i/4) & 1)
	}
	return info
}

func flipCACH(fr [4][17]bit.Bit, pos ...int) [4][17]bit.Bit {
	for _, p := range pos {
		fr[p/17][p%17] ^= 1
	}
	return fr
}

func TestCACHBPTC_AllSingleAndDoubleErrorsCorrected(t *testing.T) {
	t.Parallel()
	info := cachInfo()
	fr := encodeCACH(info)
	for k := 1; k <= 2; k++ {
		forEachErrorPattern(68, k, func(pos []int) {
			got, res := decodeCACH(flipCACH(fr, pos...))
			if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
				t.Fatalf("errors at %v: data match=%v result=%+v", pos, got == info, res)
			}
		})
	}
}

func TestCACHBPTC_ParityRowErrorCorrected(t *testing.T) {
	t.Parallel()
	info := cachInfo()
	fr := encodeCACH(info)
	for col := 0; col < 17; col++ {
		// Stream index col*4+3 is row 3 (the column parity row) of that column.
		got, res := decodeCACH(flipCACH(fr, col*4+3))
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Fatalf("PC row col %d: data match=%v result=%+v", col, got == info, res)
		}
	}
}

func TestCACHBPTC_TripleErrorsNeverSilent(t *testing.T) {
	t.Parallel()
	info := cachInfo()
	fr := encodeCACH(info)
	forEachErrorPattern(68, 3, func(pos []int) {
		got, res := decodeCACH(flipCACH(fr, pos...))
		if !res.Uncorrectable && (got != info || res.ErrorsCorrected != 3) {
			t.Fatalf("errors at %v: accepted wrong result, data match=%v result=%+v", pos, got == info, res)
		}
	})
}
