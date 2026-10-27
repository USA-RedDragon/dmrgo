package bptc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
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
		{0x00, 0x00, 0x11, 0x09},
		{0x0F, 0x12, 0x96, 0x96},
		{0x09, 0x0C, 0x35, 0x95},
		{0x84, 0xAA, 0x2B, 0xA5},
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
	want := []byte{0x03, 0x00, 0x00, 0x00, 0x27, 0x06, 0x30, 0xB4, 0x3C}
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
		forEachErrorPattern(128, k, func(pos []int) {
			got, res := decodeEmbedded(flipEmbedded(fr, pos...))
			if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
				t.Fatalf("errors at %v: data match=%v result=%+v", pos, got == info, res)
			}
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

func TestEmbeddedLC_RandomWordsRejected(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	accepted := 0
	for n := 0; n < 2000; n++ {
		var rx [4][32]unpacked.Bit
		for i := 0; i < 128; i++ {
			rx[i/32][i%32] = unpacked.Bit(r.Intn(2))
		}
		if _, res := decodeEmbedded(rx); !res.Uncorrectable {
			if res.ErrorsCorrected > bptc.EmbeddedMaxErrors {
				t.Fatalf("corrected %d errors", res.ErrorsCorrected)
			}
			accepted++
		}
	}
	if accepted > 2 {
		t.Errorf("accepted %d of 2000 random words", accepted)
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
