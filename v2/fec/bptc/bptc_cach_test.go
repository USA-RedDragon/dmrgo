package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
)

func TestCACHBPTC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	// Build a 36-bit info pattern
	var info [36]bit.Bit
	for i := range info {
		info[i] = bit.Bit(i % 2)
	}

	fragments := bptc.EncodeCACHBPTC(info)
	decoded, fecResult := bptc.DecodeCACHBPTC(fragments)

	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	if decoded != info {
		t.Error("round-trip failed: decoded doesn't match original")
	}
}

func TestCACHBPTC_BitsChecked(t *testing.T) {
	t.Parallel()
	var fragments [4][17]bit.Bit
	_, fecResult := bptc.DecodeCACHBPTC(fragments)
	if fecResult.BitsChecked != 68 {
		t.Errorf("expected BitsChecked=68, got %d", fecResult.BitsChecked)
	}
}

func TestCACHBPTC_AllZero(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	fragments := bptc.EncodeCACHBPTC(info)
	decoded, fecResult := bptc.DecodeCACHBPTC(fragments)

	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	for i, b := range decoded {
		if b != 0 {
			t.Errorf("bit %d: expected 0, got %d", i, b)
		}
	}
}

func TestCACHBPTC_AllOnes(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	for i := range info {
		info[i] = 1
	}

	fragments := bptc.EncodeCACHBPTC(info)
	decoded, fecResult := bptc.DecodeCACHBPTC(fragments)

	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	for i, b := range decoded {
		if b != 1 {
			t.Errorf("bit %d: expected 1, got %d", i, b)
		}
	}
}

func TestCACHBPTC_FragmentSize(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	fragments := bptc.EncodeCACHBPTC(info)

	if len(fragments) != 4 {
		t.Errorf("expected 4 fragments, got %d", len(fragments))
	}
	for i, frag := range fragments {
		if len(frag) != 17 {
			t.Errorf("fragment %d: expected 17 bits, got %d", i, len(frag))
		}
	}
}

func TestCACHBPTC_FragmentBitsValid(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	info[0] = 1
	info[15] = 1
	info[35] = 1

	fragments := bptc.EncodeCACHBPTC(info)
	for i, frag := range fragments {
		for j, b := range frag {
			if b != 0 && b != 1 {
				t.Errorf("fragment %d bit %d: invalid value %d", i, j, b)
			}
		}
	}
}

func TestCACHBPTC_SingleBitErrorCorrection(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	info[0] = 1
	info[5] = 1
	info[10] = 1
	info[20] = 1
	info[30] = 1

	fragments := bptc.EncodeCACHBPTC(info)

	// Corrupt one bit in the first fragment (within a data row)
	fragments[0][0] ^= 1

	decoded, fecResult := bptc.DecodeCACHBPTC(fragments)
	if fecResult.Uncorrectable {
		t.Fatal("single bit error should be correctable")
	}
	if fecResult.ErrorsCorrected == 0 {
		t.Error("expected at least one error corrected")
	}
	if decoded != info {
		t.Error("corrected data doesn't match original")
	}
}

func TestCACHBPTC_StabilityEncodeDecode(t *testing.T) {
	t.Parallel()
	// Encode → decode → encode should produce identical fragments
	var info [36]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i * 7) % 2)
	}

	fragments1 := bptc.EncodeCACHBPTC(info)
	decoded, fecResult := bptc.DecodeCACHBPTC(fragments1)
	if fecResult.Uncorrectable {
		t.Fatal("unexpected uncorrectable error")
	}
	fragments2 := bptc.EncodeCACHBPTC(decoded)

	if fragments1 != fragments2 {
		t.Error("encode→decode→encode produced different fragments")
	}
}

func TestCACHBPTC_DifferentPatterns(t *testing.T) {
	t.Parallel()
	patterns := [][36]bit.Bit{}
	for p := 0; p < 4; p++ {
		var info [36]bit.Bit
		for i := range info {
			info[i] = bit.Bit((i + p) % 2)
		}
		patterns = append(patterns, info)
	}

	for _, info := range patterns {
		fragments := bptc.EncodeCACHBPTC(info)
		decoded, fecResult := bptc.DecodeCACHBPTC(fragments)
		if fecResult.Uncorrectable {
			t.Error("unexpected uncorrectable error")
		}
		if decoded != info {
			t.Error("round-trip failed for pattern")
		}
	}
}

func TestCACHBPTC_ColumnParityError(t *testing.T) {
	t.Parallel()
	var info [36]bit.Bit
	fragments := bptc.EncodeCACHBPTC(info)

	// Corrupt two bits in the same column across different rows
	// This creates an undetectable column error if both are in data rows
	fragments[0][0] ^= 1
	fragments[1][0] ^= 1

	_, fecResult := bptc.DecodeCACHBPTC(fragments)
	// With two errors, the result may or may not be correctable depending
	// on the specific positions, but at minimum errors should be detected
	if fecResult.ErrorsCorrected == 0 && !fecResult.Uncorrectable {
		t.Error("expected error detection for multi-bit corruption")
	}
}
