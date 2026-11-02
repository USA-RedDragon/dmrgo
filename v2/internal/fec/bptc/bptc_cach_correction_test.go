package bptc_test

import (
	"math/bits"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func cachInfo() [36]unpacked.Bit {
	var info [36]unpacked.Bit
	for i := range info {
		info[i] = unpacked.Bit((i*5 + i/4) & 1)
	}
	return info
}

func flipCACH(fr [4][17]unpacked.Bit, pos ...int) [4][17]unpacked.Bit {
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

func flipCACHWord(fr [4]uint32, pos ...int) [4]uint32 {
	for _, p := range pos {
		fr[p/17] ^= 1 << (16 - p%17)
	}
	return fr
}

func TestCACHBPTC_QuadErrorsWrongOnlyAtDistanceTwo(t *testing.T) {
	t.Parallel()
	const info = 0x5A5A5A5A5
	fr := bptc.EncodeCACH(info)
	wrong := 0
	forEachErrorPattern(68, 4, func(pos []int) {
		got, res := bptc.DecodeCACH(flipCACHWord(fr, pos...))
		if res.Uncorrectable || got == info {
			return
		}
		wrong++
		if res.ErrorsCorrected > 2 {
			t.Fatalf("errors at %v: decoded to another codeword %d bits from the received word", pos, res.ErrorsCorrected)
		}
	})
	if wrong == 0 {
		t.Fatal("no weight-4 pattern lies within 2 bits of another codeword")
	}
}

func TestCACHBPTC_RandomErrorsRarelyMiscorrected(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(1, 2))
	var accepted, wrong int
	for range 20000 {
		info := rng.Uint64() & (1<<36 - 1)
		fr := bptc.EncodeCACH(info)
		for i := range 68 {
			if rng.Float64() < 0.08 {
				fr = flipCACHWord(fr, i)
			}
		}
		got, res := bptc.DecodeCACH(fr)
		if res.Uncorrectable {
			continue
		}
		accepted++
		if got != info {
			wrong++
		}
	}
	if accepted == 0 || 25*wrong > accepted {
		t.Errorf("8%% bit errors: %d of %d accepted payloads wrong", wrong, accepted)
	}
}

func TestCACHBPTC_AcceptedWithinReportedDistance(t *testing.T) {
	t.Parallel()
	rng := rand.New(rand.NewPCG(3, 4))
	near := 0
	for _, p := range []float64{0.02, 0.04, 0.08} {
		for range 5000 {
			fr := bptc.EncodeCACH(rng.Uint64() & (1<<36 - 1))
			for i := range 68 {
				if rng.Float64() < p {
					fr = flipCACHWord(fr, i)
				}
			}
			got, res := bptc.DecodeCACH(fr)
			if res.Uncorrectable {
				continue
			}
			c := bptc.EncodeCACH(got)
			d := 0
			for i := range c {
				d += bits.OnesCount32(c[i] ^ fr[i])
			}
			if d != res.ErrorsCorrected {
				t.Fatalf("decoded codeword is %d bits from the received word, result says %d", d, res.ErrorsCorrected)
			}
			if d <= 2 {
				near++
			}
		}
	}
	if near == 0 {
		t.Fatal("no received word within 2 bits of a codeword")
	}
}
