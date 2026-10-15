package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
)

func fragmentsFromBytes(b [4][4]byte) [4][32]bit.Bit {
	var fr [4][32]bit.Bit
	for i := range b {
		copy(fr[i][:], bit.UnpackBits(b[i][:]))
	}
	return fr
}

// TestEmbeddedLC_CapturedSuperframe decodes the embedded LC fragments of
// voice bursts B..E from layer2/testdata/voice.bin.
func TestEmbeddedLC_CapturedSuperframe(t *testing.T) {
	t.Parallel()
	fr := fragmentsFromBytes([4][4]byte{
		{0x00, 0x00, 0x11, 0x09},
		{0x0F, 0x12, 0x96, 0x96},
		{0x09, 0x0C, 0x35, 0x95},
		{0x84, 0xAA, 0x2B, 0xA5},
	})
	info, res := bptc.DecodeEmbeddedLC(fr)
	if res.Uncorrectable || res.ErrorsCorrected != 0 {
		t.Fatalf("captured superframe: result=%+v, want clean", res)
	}
	// Figure B.3: rows 0 and 1 hold 11 LC bits, rows 2..6 hold 10 LC bits
	// followed by one checksum bit.
	var lc []bit.Bit
	var cs []bit.Bit
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
	got := bit.PackBits(lc)
	if string(got) != string(want) {
		t.Errorf("LC % X, want % X", got, want)
	}
	sum := 0
	for _, b := range got {
		sum += int(b)
	}
	if c := bit.BitsToInt(cs, 0, 5); c != sum%31 {
		t.Errorf("checksum %d, want %d", c, sum%31)
	}
}

func embeddedLCInfo() [77]bit.Bit {
	var info [77]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i*7 + i/3) & 1)
	}
	return info
}

func flipEmbedded(fr [4][32]bit.Bit, pos ...int) [4][32]bit.Bit {
	for _, p := range pos {
		fr[p/32][p%32] ^= 1
	}
	return fr
}

func TestEmbeddedLC_AllSingleAndDoubleErrorsCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbeddedLC(info)
	for k := 1; k <= 2; k++ {
		forEachErrorPattern(128, k, func(pos []int) {
			got, res := bptc.DecodeEmbeddedLC(flipEmbedded(fr, pos...))
			if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
				t.Fatalf("errors at %v: data match=%v result=%+v", pos, got == info, res)
			}
		})
	}
}

func TestEmbeddedLC_TripleErrorsNeverSilent(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbeddedLC(info)
	forEachErrorPattern(128, 3, func(pos []int) {
		got, res := bptc.DecodeEmbeddedLC(flipEmbedded(fr, pos...))
		if !res.Uncorrectable && (got != info || res.ErrorsCorrected != 3) {
			t.Fatalf("errors at %v: accepted wrong result, data match=%v result=%+v", pos, got == info, res)
		}
	})
}

func TestEmbeddedLC_ParityRowErrorCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbeddedLC(info)
	for col := 0; col < 16; col++ {
		// Stream index col*8+7 is row 7 (the column parity row) of that column.
		got, res := bptc.DecodeEmbeddedLC(flipEmbedded(fr, col*8+7))
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Fatalf("PC row col %d: data match=%v result=%+v", col, got == info, res)
		}
	}
}
