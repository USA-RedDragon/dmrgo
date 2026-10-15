package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

func TestEmbeddedLC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit
	for i := range info {
		info[i] = bit.Bit(i % 2)
	}

	fragments := bptc.EncodeEmbedded(info)
	decoded, result := bptc.DecodeEmbedded(fragments)

	if result.Uncorrectable {
		t.Fatal("DecodeEmbedded reported uncorrectable errors on clean encode")
	}
	if result.ErrorsCorrected != 0 {
		t.Errorf("ErrorsCorrected = %d, want 0 for clean encode", result.ErrorsCorrected)
	}
	if decoded != info {
		t.Error("decoded info does not match original")
	}
}

func TestEmbeddedLC_BitsChecked(t *testing.T) {
	t.Parallel()
	var fragments [4][32]bit.Bit
	_, result := bptc.DecodeEmbedded(fragments)
	if result.BitsChecked != 128 {
		t.Errorf("BitsChecked = %d, want 128", result.BitsChecked)
	}
}

func TestEmbeddedLC_AllZero(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit

	fragments := bptc.EncodeEmbedded(info)
	decoded, result := bptc.DecodeEmbedded(fragments)

	if result.Uncorrectable {
		t.Fatal("uncorrectable error on all-zero input")
	}
	if decoded != info {
		t.Error("decoded info does not match all-zero input")
	}
}

func TestEmbeddedLC_AllOnes(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit
	for i := range info {
		info[i] = 1
	}

	fragments := bptc.EncodeEmbedded(info)
	decoded, result := bptc.DecodeEmbedded(fragments)

	if result.Uncorrectable {
		t.Fatal("uncorrectable error on all-ones input")
	}
	if decoded != info {
		t.Error("decoded info does not match all-ones input")
	}
}

func TestEmbeddedLC_SingleBitErrorCorrection(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i * 7) % 2)
	}

	fragments := bptc.EncodeEmbedded(info)

	fragments[0][5] ^= 1

	decoded, result := bptc.DecodeEmbedded(fragments)

	if result.Uncorrectable {
		t.Fatal("single bit error should be correctable")
	}
	if result.ErrorsCorrected < 1 {
		t.Error("expected at least 1 error correction")
	}
	if decoded != info {
		t.Error("decoded info does not match after single-bit correction")
	}
}

func TestEmbeddedLC_FragmentCount(t *testing.T) {
	t.Parallel()
	info := embeddedLCInfo()
	fragments := bptc.EncodeEmbedded(info)

	for i := range fragments {
		rx := fragments
		rx[i][31] ^= 1
		if got, res := bptc.DecodeEmbedded(rx); got != info || res.ErrorsCorrected != 1 {
			t.Errorf("fragment %d: data match=%v result=%+v", i, got == info, res)
		}
	}
}

func TestEmbeddedLC_FragmentBitsAreValid(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit
	info[0] = 1
	info[10] = 1
	info[76] = 1

	fragments := bptc.EncodeEmbedded(info)

	for i, f := range fragments {
		for j, b := range f {
			if b != 0 && b != 1 {
				t.Fatalf("fragments[%d][%d] = %d, want 0 or 1", i, j, b)
			}
		}
	}
}

func TestEmbeddedLC_StabilityEncodeDecode(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i*3 + 1) % 2)
	}

	fragments1 := bptc.EncodeEmbedded(info)
	decoded, _ := bptc.DecodeEmbedded(fragments1)
	fragments2 := bptc.EncodeEmbedded(decoded)

	for i := 0; i < 4; i++ {
		if fragments1[i] != fragments2[i] {
			t.Errorf("fragment[%d] differs after encode-decode-encode cycle", i)
		}
	}
}

func TestEmbeddedLC_ColumnParityErrorDetection(t *testing.T) {
	t.Parallel()
	var info [77]bit.Bit

	fragments := bptc.EncodeEmbedded(info)

	fragments[0][0] ^= 1
	fragments[0][1] ^= 1

	_, result := bptc.DecodeEmbedded(fragments)

	if result.ErrorsCorrected == 0 && !result.Uncorrectable {
		t.Error("expected error detection for 2-bit error")
	}
}

func TestEmbeddedLC_DifferentPatterns(t *testing.T) {
	t.Parallel()
	var info1, info2 [77]bit.Bit
	info1[0] = 1
	info2[0] = 0

	fragments1 := bptc.EncodeEmbedded(info1)
	fragments2 := bptc.EncodeEmbedded(info2)

	allSame := true
	for i := 0; i < 4; i++ {
		if fragments1[i] != fragments2[i] {
			allSame = false
			break
		}
	}
	if allSame {
		t.Error("different info should produce different fragments")
	}
}
