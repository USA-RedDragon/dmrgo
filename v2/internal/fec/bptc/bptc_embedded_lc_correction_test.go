package bptc_test

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func fragmentsFromBytes(b [4][4]byte) [4][32]unpacked.Bit {
	var fr [4][32]unpacked.Bit
	for i := range b {
		copy(fr[i][:], unpacked.Unpack(b[i][:]))
	}
	return fr
}

func TestEmbeddedLC_CapturedSuperframe(t *testing.T) {
	t.Parallel()
	fr := fragmentsFromBytes([4][4]byte{
		{0x05, 0x06, 0x12, 0x0F},
		{0x0C, 0x14, 0x96, 0x93},
		{0x0F, 0x06, 0x14, 0x96},
		{0x88, 0x84, 0x0C, 0x88},
	})
	info, res := decodeEmbedded(fr)
	if res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Fatalf("captured superframe: result=%+v, want clean", res)
	}
	// ETSI TS 102 361-1 Figure B.3
	var lc []unpacked.Bit
	var cs []unpacked.Bit
	for row := 0; row < 7; row++ {
		r := info[row*11 : row*11+11]
		if row < 2 {
			lc = append(lc, r...)
			continue
		}
		lc = append(lc, r[:10]...)
		cs = append(cs, r[10])
	}
	want := []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x2D, 0xED, 0xCF}
	got := unpacked.Pack(lc)
	if string(got) != string(want) {
		t.Errorf("LC % X, want % X", got, want)
	}
	sum := 0
	for _, b := range got {
		sum += int(b)
	}
	if c := int(unpacked.Uint(cs[:5])); c != sum%31 {
		t.Errorf("checksum %d, want %d", c, sum%31)
	}
}

func embeddedLCInfo() [77]unpacked.Bit {
	var info [77]unpacked.Bit
	for i := range info {
		info[i] = unpacked.Bit((i*7 + i/3) & 1)
	}
	return info
}

func flipEmbedded(fr [4][32]unpacked.Bit, pos ...int) [4][32]unpacked.Bit {
	for _, p := range pos {
		fr[p/32][p%32] ^= 1
	}
	return fr
}

func TestEmbeddedLC_UpToThreeErrorsCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := encodeEmbedded(info)
	for k := 1; k <= testmode.N(3, 3, 2); k++ {
		var patterns [][]int
		forEachErrorPattern(128, k, func(pos []int) { patterns = append(patterns, slices.Clone(pos)) })
		testmode.Cases(t, len(patterns), func(i int) []int { return patterns[i] }, func(pos *[]int) string {
			got, res := decodeEmbedded(flipEmbedded(fr, *pos...))
			if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
				return fmt.Sprintf("errors at %v: data match=%v result=%+v", *pos, got == info, res)
			}
			return ""
		})
	}
}

func TestEmbeddedLC_ParityRowErrorCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := encodeEmbedded(info)
	for col := 0; col < 16; col++ {
		got, res := decodeEmbedded(flipEmbedded(fr, col*8+7))
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Fatalf("PC row col %d: data match=%v result=%+v", col, got == info, res)
		}
	}
}

func TestEmbeddedLC_TieResolvedByCheck(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	rx := flipEmbedded(encodeEmbedded(info), 0*8, 11*8, 14*8, 15*8)
	if _, res := decodeEmbedded(rx); !res.Uncorrectable {
		t.Fatalf("tie accepted: %+v", res)
	}
	got, res := decodeEmbeddedWithCheck(rx, func(i [77]unpacked.Bit) bool { return i == info })
	if got != info || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("data match=%v result=%+v", got == info, res)
	}
	other := info
	other[0] ^= 1
	got, res = decodeEmbeddedWithCheck(rx, func(i [77]unpacked.Bit) bool { return i == other })
	if got != other || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("other codeword: data match=%v result=%+v", got == other, res)
	}
}

const (
	embeddedLCRandomAcceptRate = 2.72e-4
	embeddedLCFalseFailureRate = 1e-6
)

func binomialUpperBar(n int, p, alpha float64) int {
	lgN, _ := math.Lgamma(float64(n + 1))
	tail := 1.0
	for k := 0; k <= n; k++ {
		lgK, _ := math.Lgamma(float64(k + 1))
		lgNK, _ := math.Lgamma(float64(n - k + 1))
		tail -= math.Exp(lgN - lgK - lgNK + float64(k)*math.Log(p) + float64(n-k)*math.Log1p(-p))
		if tail <= alpha {
			return k
		}
	}
	return n
}

func TestEmbeddedLC_RandomWordsRejected(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(int64(testseed.New(t).Uint64())))
	n := testmode.N(50_000, 50_000, 5_000)
	bar := binomialUpperBar(n, embeddedLCRandomAcceptRate, embeddedLCFalseFailureRate)
	accepted := 0
	for range n {
		rx := [4]uint32{r.Uint32(), r.Uint32(), r.Uint32(), r.Uint32()}
		if _, res := bptc.DecodeEmbedded(rx); !res.Uncorrectable {
			if res.ErrorsCorrected > bptc.EmbeddedMaxErrors {
				t.Fatalf("corrected %d errors", res.ErrorsCorrected)
			}
			accepted++
		}
	}
	if accepted > bar {
		t.Errorf("accepted %d of %d random words, want at most %d: a random word is within %d bits of exactly one nearest codeword with probability %.3g (the unique coset leaders of weight %d or less over 2^51 cosets), so more than %d accepts has probability below %g",
			accepted, n, bar, bptc.EmbeddedMaxErrors, embeddedLCRandomAcceptRate, bptc.EmbeddedMaxErrors, bar, embeddedLCFalseFailureRate)
	}
}

func TestEmbeddedLCStrict_RowErrors(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := encodeEmbedded(info)
	for p := 0; p < 128; p++ {
		got, res := decodeEmbeddedStrict(flipEmbedded(fr, p))
		if p%8 == 7 {
			if !res.Uncorrectable {
				t.Fatalf("row 7 error at %d accepted", p)
			}
			continue
		}
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Fatalf("error at %d: data match=%v result=%+v", p, got == info, res)
		}
	}
	got, res := decodeEmbeddedStrict(flipEmbedded(fr, 0, 9, 18, 27, 36, 45, 54))
	if got != info || res.Uncorrectable || res.ErrorsCorrected != 7 {
		t.Fatalf("one error per row: data match=%v result=%+v", got == info, res)
	}
	if _, res := decodeEmbeddedStrict(flipEmbedded(fr, 0, 8)); !res.Uncorrectable {
		t.Fatal("two errors in row 0 accepted")
	}
}
