package bptc_test

import (
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func refDecodeSingleBurst(tx uint32, oddParity bool) (uint16, fec.Result) {
	var b [32]unpacked.Bit
	unpacked.PutUint(b[:], uint64(tx))
	info, res := ref.DecodeSingleBurst(b, oddParity)
	return uint16(unpacked.Uint(info[:])), res
}

func TestSingleBurstMatchesReference(t *testing.T) {
	t.Parallel()

	check := func(tx uint32, odd bool) {
		got, gotRes := bptc.DecodeSingleBurst(tx, odd)
		want, wantRes := refDecodeSingleBurst(tx, odd)
		if got != want || gotRes != wantRes {
			t.Fatalf("DecodeSingleBurst(%08x, %v) = %03x %+v, want %03x %+v", tx, odd, got, gotRes, want, wantRes)
		}
	}
	for _, odd := range []bool{false, true} {
		for info := range uint16(1 << 11) {
			var b [11]unpacked.Bit
			unpacked.PutUint(b[:], uint64(info))
			refTx := ref.EncodeSingleBurst(b, odd)
			tx := bptc.EncodeSingleBurst(info, odd)
			if tx != uint32(unpacked.Uint(refTx[:])) {
				t.Fatalf("EncodeSingleBurst(%03x, %v) = %08x, want %v", info, odd, tx, refTx)
			}
			for i := range 33 {
				for j := i + 1; j < 34; j++ {
					check(tx^flip32(i)^flip32(j), odd)
				}
			}
		}
	}
	r := rand.New(rand.NewSource(1))
	for range 4_000_000 {
		check(r.Uint32(), r.Intn(2) == 1)
	}
}

// flip32 returns the mask of transmit bit i of a 32-bit word, or 0 for i of
// 32 or more.
func flip32(i int) uint32 {
	if i >= 32 {
		return 0
	}
	return 1 << (31 - i)
}
