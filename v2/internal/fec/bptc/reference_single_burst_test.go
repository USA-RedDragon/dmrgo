package bptc_test

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
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

	check := func(tx uint32, odd bool) string {
		got, gotRes := bptc.DecodeSingleBurst(tx, odd)
		want, wantRes := refDecodeSingleBurst(tx, odd)
		if got != want || gotRes != wantRes {
			return fmt.Sprintf("DecodeSingleBurst(%08x, %v) = %03x %+v, want %03x %+v", tx, odd, got, gotRes, want, wantRes)
		}
		return ""
	}
	type word struct {
		tx  uint32
		odd bool
	}
	testmode.Cases(t, 2<<11, func(i int) word { return word{uint32(i % (1 << 11)), i >= 1<<11} }, func(c *word) string {
		info, odd := uint16(c.tx), c.odd
		var b [11]unpacked.Bit
		unpacked.PutUint(b[:], uint64(info))
		refTx := ref.EncodeSingleBurst(b, odd)
		tx := bptc.EncodeSingleBurst(info, odd)
		if tx != uint32(unpacked.Uint(refTx[:])) {
			return fmt.Sprintf("EncodeSingleBurst(%03x, %v) = %08x, want %v", info, odd, tx, refTx)
		}
		for i := range 33 {
			for j := i + 1; j < 34; j++ {
				if msg := check(tx^flip32(i)^flip32(j), odd); msg != "" {
					return msg
				}
			}
		}
		return ""
	})
	r := rand.New(rand.NewSource(1))
	testmode.Cases(t, 4_000_000, func(int) word {
		tx := r.Uint32()
		return word{tx, r.Intn(2) == 1}
	}, func(c *word) string { return check(c.tx, c.odd) })
}

func flip32(i int) uint32 {
	if i >= 32 {
		return 0
	}
	return 1 << (31 - i)
}
