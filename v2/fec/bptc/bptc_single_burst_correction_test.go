package bptc_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/fec/bptc"
)

// forEachErrorPattern calls fn with every set of k distinct positions in [0, n).
func forEachErrorPattern(n, k int, fn func(pos []int)) {
	pos := make([]int, k)
	var rec func(start, depth int)
	rec = func(start, depth int) {
		if depth == k {
			fn(pos)
			return
		}
		for p := start; p < n; p++ {
			pos[depth] = p
			rec(p+1, depth+1)
		}
	}
	rec(0, 0)
}

func singleBurstInfo(m int) [11]bit.Bit {
	var info [11]bit.Bit
	for i := range info {
		info[i] = bit.Bit((m >> i) & 1)
	}
	return info
}

func TestSingleBurstBPTC_UpToThreeErrorsCorrected(t *testing.T) {
	t.Parallel()
	for _, rc := range []bool{false, true} {
		for _, m := range []int{0, 0x7FF, 0x2A5, 0x15A} {
			info := singleBurstInfo(m)
			tx := bptc.EncodeSingleBurst(info, rc)
			for k := 1; k <= 3; k++ {
				forEachErrorPattern(32, k, func(pos []int) {
					rx := tx
					for _, p := range pos {
						rx[p] ^= 1
					}
					got, res := bptc.DecodeSingleBurst(rx, rc)
					if got != info || res.Uncorrectable || res.ErrorsCorrected != k {
						t.Fatalf("rc=%v info=%03X errors at %v: data match=%v result=%+v", rc, m, pos, got == info, res)
					}
				})
			}
		}
	}
}

func TestSingleBurstBPTC_FourErrorsDetected(t *testing.T) {
	t.Parallel()
	for _, rc := range []bool{false, true} {
		info := singleBurstInfo(0x2A5)
		tx := bptc.EncodeSingleBurst(info, rc)
		forEachErrorPattern(32, 4, func(pos []int) {
			rx := tx
			for _, p := range pos {
				rx[p] ^= 1
			}
			if _, res := bptc.DecodeSingleBurst(rx, rc); !res.Uncorrectable {
				t.Fatalf("rc=%v errors at %v: not flagged, result=%+v", rc, pos, res)
			}
		})
	}
}

func TestSingleBurstBPTC_ParityRowErrorCorrected(t *testing.T) {
	t.Parallel()
	for _, rc := range []bool{false, true} {
		info := singleBurstInfo(0x021)
		tx := bptc.EncodeSingleBurst(info, rc)
		// Transmit bit 17 holds encode matrix index 1 (row 1, column 0).
		tx[17] ^= 1
		got, res := bptc.DecodeSingleBurst(tx, rc)
		if got != info || res.Uncorrectable || res.ErrorsCorrected != 1 {
			t.Errorf("rc=%v: data match=%v result=%+v", rc, got == info, res)
		}
	}
}
