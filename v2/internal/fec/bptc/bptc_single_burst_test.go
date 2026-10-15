package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
)

func TestSingleBurstBPTC_EncodeDecodeRoundTrip(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	for i := range info {
		info[i] = bit.Bit(i % 2)
	}

	txBits := bptc.EncodeSingleBurst(info, false)
	decoded, result := bptc.DecodeSingleBurst(txBits, false)

	if result.Uncorrectable {
		t.Fatal("DecodeSingleBurst reported uncorrectable errors on clean encode")
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

	txBits := bptc.EncodeSingleBurst(info, true)
	decoded, result := bptc.DecodeSingleBurst(txBits, true)

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
	_, result := bptc.DecodeSingleBurst(txBits, false)
	if result.BitsChecked != 32 {
		t.Errorf("BitsChecked = %d, want 32", result.BitsChecked)
	}
}

func TestSingleBurstBPTC_AllZero(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit

	txBits := bptc.EncodeSingleBurst(info, false)
	decoded, result := bptc.DecodeSingleBurst(txBits, false)

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

	txBits := bptc.EncodeSingleBurst(info, false)
	decoded, result := bptc.DecodeSingleBurst(txBits, false)

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

	txBits := bptc.EncodeSingleBurst(info, false)

	txBits[10] ^= 1

	decoded, result := bptc.DecodeSingleBurst(txBits, false)

	if result.Uncorrectable || result.ErrorsCorrected != 1 {
		t.Errorf("single-bit error: result=%+v, want 1 correction", result)
	}
	if decoded != info {
		t.Error("decoded does not match after correction")
	}
}

func TestSingleBurstBPTC_OutputSize(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	txBits := bptc.EncodeSingleBurst(info, false)
	if got, res := bptc.DecodeSingleBurst(txBits, false); got != info || res.HasErrors() {
		t.Errorf("round trip: data match=%v result=%+v", got == info, res)
	}
}

func TestSingleBurstBPTC_OutputBitsValid(t *testing.T) {
	t.Parallel()
	var info [11]bit.Bit
	info[0] = 1
	info[10] = 1

	txBits := bptc.EncodeSingleBurst(info, false)
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

	nonRC := bptc.EncodeSingleBurst(info, false)
	rc := bptc.EncodeSingleBurst(info, true)

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

	tx1 := bptc.EncodeSingleBurst(info, false)
	decoded, _ := bptc.DecodeSingleBurst(tx1, false)
	tx2 := bptc.EncodeSingleBurst(decoded, false)

	if tx1 != tx2 {
		t.Error("encode-decode-encode should produce identical output")
	}
}

func TestSingleBurstBPTC_DifferentData(t *testing.T) {
	t.Parallel()
	var info1, info2 [11]bit.Bit
	info1[0] = 1
	info2[0] = 0

	tx1 := bptc.EncodeSingleBurst(info1, false)
	tx2 := bptc.EncodeSingleBurst(info2, false)

	if tx1 == tx2 {
		t.Error("different info should produce different transmit bits")
	}
}
