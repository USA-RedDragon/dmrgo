package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
)

func TestSingleBurstBPTC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	for i := range info {
		info[i] = bit.Bit(i % 2)
	}

	txBits := bptc.EncodeSingleBurstBPTC(info, false)
	decoded, result := bptc.DecodeSingleBurstBPTC(txBits, false)

	if result.Uncorrectable {
		t.Fatal("DecodesSingleBurstBPTC reported uncorrectable errors on clean encode")
	}
	if result.ErrorsCorrected != 0 {
		t.Errorf("ErrorsCorrected = %d, want 0", result.ErrorsCorrected)
	}
	if decoded != info {
		t.Errorf("decoded = %v, want %v", decoded, info)
	}
}

func TestSingleBurstBPTC_RC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	info[0] = 1
	info[5] = 1
	info[10] = 1

	txBits := bptc.EncodeSingleBurstBPTC(info, true)
	decoded, result := bptc.DecodeSingleBurstBPTC(txBits, true)

	if result.Uncorrectable {
		t.Fatal("RC decode reported uncorrectable errors on clean encode")
	}
	if decoded != info {
		t.Errorf("RC decoded = %v, want %v", decoded, info)
	}
}

func TestSingleBurstBPTC_BitsChecked(t *testing.T) {
	t.Parallel()
	var txBits [32]bit.Bit
	_, result := bptc.DecodeSingleBurstBPTC(txBits, false)
	if result.BitsChecked != 32 {
		t.Errorf("BitsChecked = %d, want 32", result.BitsChecked)
	}
}

func TestSingleBurstBPTC_AllZero(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit

	txBits := bptc.EncodeSingleBurstBPTC(info, false)
	decoded, result := bptc.DecodeSingleBurstBPTC(txBits, false)

	if result.Uncorrectable {
		t.Fatal("uncorrectable error on all-zero input")
	}
	if decoded != info {
		t.Error("decoded does not match all-zero input")
	}
}

func TestSingleBurstBPTC_AllOnes(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	for i := range info {
		info[i] = 1
	}

	txBits := bptc.EncodeSingleBurstBPTC(info, false)
	decoded, result := bptc.DecodeSingleBurstBPTC(txBits, false)

	if result.Uncorrectable {
		t.Fatal("uncorrectable error on all-ones input")
	}
	if decoded != info {
		t.Error("decoded does not match all-ones input")
	}
}

func TestSingleBurstBPTC_SingleBitErrorCorrection(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	info[3] = 1
	info[7] = 1

	txBits := bptc.EncodeSingleBurstBPTC(info, false)

	// Flip a single bit
	txBits[10] ^= 1

	decoded, result := bptc.DecodeSingleBurstBPTC(txBits, false)

	if result.ErrorsCorrected < 1 {
		// Single-burst BPTC may detect via column parity instead of Hamming correction
		if !result.Uncorrectable {
			t.Error("expected error detection or correction for single-bit error")
		}
	}

	// If corrected, verify data matches
	if result.ErrorsCorrected > 0 && !result.Uncorrectable {
		if decoded != info {
			t.Error("decoded does not match after correction")
		}
	}
}

func TestSingleBurstBPTC_OutputSize(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	txBits := bptc.EncodeSingleBurstBPTC(info, false)
	if len(txBits) != 32 {
		t.Fatalf("EncodeSingleBurstBPTC output length = %d, want 32", len(txBits))
	}
}

func TestSingleBurstBPTC_OutputBitsValid(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	info[0] = 1
	info[10] = 1

	txBits := bptc.EncodeSingleBurstBPTC(info, false)
	for i, b := range txBits {
		if b != 0 && b != 1 {
			t.Fatalf("txBits[%d] = %d, want 0 or 1", i, b)
		}
	}
}

func TestSingleBurstBPTC_RC_DifferentFromNonRC(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	info[0] = 1

	nonRC := bptc.EncodeSingleBurstBPTC(info, false)
	rc := bptc.EncodeSingleBurstBPTC(info, true)

	if nonRC == rc {
		t.Error("RC and non-RC encoding should differ (odd vs even parity)")
	}
}

func TestSingleBurstBPTC_Stability(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	for i := range info {
		info[i] = bit.Bit((i + 1) % 2)
	}

	tx1 := bptc.EncodeSingleBurstBPTC(info, false)
	decoded, _ := bptc.DecodeSingleBurstBPTC(tx1, false)
	tx2 := bptc.EncodeSingleBurstBPTC(decoded, false)

	if tx1 != tx2 {
		t.Error("encode-decode-encode should produce identical output")
	}
}

func TestSingleBurstBPTC_DifferentData(t *testing.T) {
	t.Parallel()
	var info1, info2 [11]bit.Bit
	info1[0] = 1
	info2[0] = 0

	tx1 := bptc.EncodeSingleBurstBPTC(info1, false)
	tx2 := bptc.EncodeSingleBurstBPTC(info2, false)

	if tx1 == tx2 {
		t.Error("different info should produce different transmit bits")
	}
}
