package layer2_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestCACHInterleave_RoundTrip(t *testing.T) {
	t.Parallel()
	for pos := 0; pos < layer2.CACHBits; pos++ {
		var logical [layer2.CACHBits]bit.Bit
		logical[pos] = 1

		tx := layer2.CACHInterleave(logical)
		recovered := layer2.CACHDeinterleave(tx)

		for i := 0; i < layer2.CACHBits; i++ {
			if recovered[i] != logical[i] {
				t.Errorf("pos=%d: bit %d: got %d, want %d", pos, i, recovered[i], logical[i])
			}
		}
	}
}

func TestCACHDeinterleave_RoundTrip(t *testing.T) {
	t.Parallel()
	for pos := 0; pos < layer2.CACHBits; pos++ {
		var tx [layer2.CACHBits]bit.Bit
		tx[pos] = 1

		logical := layer2.CACHDeinterleave(tx)
		recovered := layer2.CACHInterleave(logical)

		for i := 0; i < layer2.CACHBits; i++ {
			if recovered[i] != tx[i] {
				t.Errorf("pos=%d: bit %d: got %d, want %d", pos, i, recovered[i], tx[i])
			}
		}
	}
}

func TestCACHInterleave_TACTSpread(t *testing.T) {
	t.Parallel()
	var logical [layer2.CACHBits]bit.Bit
	for i := 0; i < 7; i++ {
		logical[i] = 1
	}

	tx := layer2.CACHInterleave(logical)

	contiguous := 0
	for i := 0; i < layer2.CACHBits; i++ {
		if tx[i] == 1 {
			contiguous++
		} else {
			break
		}
	}

	if contiguous > 2 {
		t.Errorf("TACT bits not spread: %d contiguous at start of tx", contiguous)
	}

	total := 0
	for i := 0; i < layer2.CACHBits; i++ {
		if tx[i] == 1 {
			total++
		}
	}
	if total != 7 {
		t.Errorf("expected 7 set bits, got %d", total)
	}
}

func TestCACHInterleave_ATPosition(t *testing.T) {
	t.Parallel()
	var logical [layer2.CACHBits]bit.Bit
	logical[0] = 1

	tx := layer2.CACHInterleave(logical)
	if tx[23] != 1 {
		t.Errorf("AT should be at tx[23], but tx[23]=%d", tx[23])
	}
}

func TestCACHDeinterleave_PreservesData(t *testing.T) {
	t.Parallel()
	var logical [layer2.CACHBits]bit.Bit
	logical[0] = 1
	logical[2] = 1
	logical[4] = 1
	logical[6] = 1
	logical[23] = 1

	tx := layer2.CACHInterleave(logical)
	recovered := layer2.CACHDeinterleave(tx)

	if recovered[0] != 1 {
		t.Error("AT not recovered")
	}
	if recovered[2] != 1 {
		t.Error("LS(1) not recovered")
	}
	if recovered[4] != 1 {
		t.Error("H(2) not recovered")
	}
	if recovered[6] != 1 {
		t.Error("H(0) not recovered")
	}
	if recovered[23] != 1 {
		t.Error("P(0) not recovered")
	}

	for i := 0; i < layer2.CACHBits; i++ {
		if i == 0 || i == 2 || i == 4 || i == 6 || i == 23 {
			continue
		}
		if recovered[i] != 0 {
			t.Errorf("bit %d should be 0, got %d", i, recovered[i])
		}
	}
}

func TestCACHDeinterleave_WithDecodeCACH(t *testing.T) {
	t.Parallel()
	cach := layer2.CACH{}
	cach.TACT.AccessType = true
	cach.TACT.TDMAChannel = false
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		cach.Payload[i] = bit.Bit(i % 2)
	}

	logical := layer2.EncodeCACH(&cach)
	tx := layer2.CACHInterleave(logical)
	deinterleaved := layer2.CACHDeinterleave(tx)
	decoded := layer2.DecodeCACH(deinterleaved)

	if decoded.TACT.AccessType != true {
		t.Error("AccessType not recovered")
	}
	if decoded.TACT.TDMAChannel != false {
		t.Error("TDMAChannel should be false")
	}
	for i := 0; i < layer2.CACHPayloadBits; i++ {
		if decoded.Payload[i] != bit.Bit(i%2) {
			t.Errorf("payload bit %d: got %d, want %d", i, decoded.Payload[i], i%2)
		}
	}
}

func TestCACHInterleave_InverseConsistency(t *testing.T) {
	t.Parallel()
	for i := 0; i < layer2.CACHBits; i++ {
		var logical [layer2.CACHBits]bit.Bit
		logical[i] = 1

		tx := layer2.CACHInterleave(logical)

		count := 0
		txPos := -1
		for j := 0; j < layer2.CACHBits; j++ {
			if tx[j] == 1 {
				count++
				txPos = j
			}
		}
		if count != 1 {
			t.Errorf("logical[%d]: expected 1 set bit in tx, got %d", i, count)
			continue
		}

		var tx2 [layer2.CACHBits]bit.Bit
		tx2[txPos] = 1
		logical2 := layer2.CACHDeinterleave(tx2)

		if logical2[i] != 1 {
			t.Errorf("logical[%d] -> tx[%d] -> logical[%d] should be 1", i, txPos, i)
		}
	}
}
