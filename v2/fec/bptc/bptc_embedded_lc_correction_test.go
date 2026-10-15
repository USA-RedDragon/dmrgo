package bptc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
)

func fragmentsFromBytes(b [4][4]byte) [4][32]bit.Bit {
	var fr [4][32]bit.Bit
	for i := range b {
		copy(fr[i][:], bit.Unpack(b[i][:]))
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
	info, res := bptc.DecodeEmbedded(fr)
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
	got := bit.Pack(lc)
	if string(got) != string(want) {
		t.Errorf("LC % X, want % X", got, want)
	}
	sum := 0
	for _, b := range got {
		sum += int(b)
	}
	if c := int(bit.Uint(cs[:5])); c != sum%31 {
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

func TestEmbeddedLC_UpToThreeErrorsCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbedded(info)
	for k := 1; k <= 3; k++ {
		forEachErrorPattern(128, k, func(pos []int) {
			got, res := bptc.DecodeEmbedded(flipEmbedded(fr, pos...))
			if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
				t.Fatalf("errors at %v: data match=%v result=%+v", pos, got == info, res)
			}
		})
	}
}

func TestEmbeddedLC_ParityRowErrorCorrected(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbedded(info)
	for col := 0; col < 16; col++ {
		// Stream index col*8+7 is row 7 (the column parity row) of that column.
		got, res := bptc.DecodeEmbedded(flipEmbedded(fr, col*8+7))
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Fatalf("PC row col %d: data match=%v result=%+v", col, got == info, res)
		}
	}
}

// TestEmbeddedLC_TieResolvedByCheck puts 4 errors on a row codeword in row
// 0, so the received bits are as near to the codeword with that row codeword
// added to rows 0 and 7. DecodeEmbedded rejects the tie and a check that
// accepts only one of the two information words resolves it.
func TestEmbeddedLC_TieResolvedByCheck(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	rx := flipEmbedded(bptc.EncodeEmbedded(info), 0*8, 11*8, 14*8, 15*8)
	if _, res := bptc.DecodeEmbedded(rx); !res.Uncorrectable {
		t.Fatalf("tie accepted: %+v", res)
	}
	got, res := bptc.DecodeEmbeddedWithCheck(rx, func(i [77]bit.Bit) bool { return i == info })
	if got != info || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("data match=%v result=%+v", got == info, res)
	}
	other := info
	other[0] ^= 1
	got, res = bptc.DecodeEmbeddedWithCheck(rx, func(i [77]bit.Bit) bool { return i == other })
	if got != other || res.Uncorrectable || res.ErrorsCorrected != 4 {
		t.Fatalf("other codeword: data match=%v result=%+v", got == other, res)
	}
}

// TestEmbeddedLC_RandomWordsRejected checks that random words, which are
// mostly more than EmbeddedMaxErrors from every codeword, are rejected.
func TestEmbeddedLC_RandomWordsRejected(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewSource(3))
	accepted := 0
	for n := 0; n < 2000; n++ {
		var rx [4][32]bit.Bit
		for i := 0; i < 128; i++ {
			rx[i/32][i%32] = bit.Bit(r.Intn(2))
		}
		if _, res := bptc.DecodeEmbedded(rx); !res.Uncorrectable {
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

// TestEmbeddedLCStrict_RowErrors checks the MMDVMHost rules: one error in
// each of rows 0..6 is corrected, and an error in row 7 or two errors in one
// row make the LC uncorrectable. Stream index col*8+row is that matrix bit.
func TestEmbeddedLCStrict_RowErrors(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fr := bptc.EncodeEmbedded(info)
	for p := 0; p < 128; p++ {
		got, res := bptc.DecodeEmbeddedStrict(flipEmbedded(fr, p))
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
	got, res := bptc.DecodeEmbeddedStrict(flipEmbedded(fr, 0, 9, 18, 27, 36, 45, 54))
	if got != info || res.Uncorrectable || res.ErrorsCorrected != 7 {
		t.Fatalf("one error per row: data match=%v result=%+v", got == info, res)
	}
	if _, res := bptc.DecodeEmbeddedStrict(flipEmbedded(fr, 0, 8)); !res.Uncorrectable {
		t.Fatal("two errors in row 0 accepted")
	}
}
