package golay

import (
	"fmt"
	"math/bits"
	"testing"

	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/golay"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
)

func bits20(cw uint32) [20]unpacked.Bit {
	var b [20]unpacked.Bit
	unpacked.PutUint(b[:], uint64(cw))
	return b
}

func TestGolay2087MatchesReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	everyWord(t, 1<<20, func(cw uint32) string {
		got, res := Decode2087(cw)
		want, wantRes := ref.Decode2087(bits20(cw))
		if got != uint32(unpacked.Uint(want[:])) || res != wantRes {
			return fmt.Sprintf("Decode2087(%#05x) = %#05x %+v, want %#05x %+v", cw, got, res, unpacked.Uint(want[:]), wantRes)
		}
		if Check2087(cw) != ref.Check2087(bits20(cw)) {
			return fmt.Sprintf("Check2087(%#05x) differs from the reference", cw)
		}
		return ""
	})
	for d := range 256 {
		data := bits20(uint32(d) << 12)
		want := ref.Encode2087([8]unpacked.Bit(data[:8]))
		if got := Encode2087(uint8(d)); got != uint32(unpacked.Uint(want[:])) {
			t.Fatalf("Encode2087(%#02x) = %#05x, want %#05x", d, got, unpacked.Uint(want[:]))
		}
	}
}

func TestGolay2087CorrectsUpToThreeErrors(t *testing.T) {
	t.Parallel()

	patterns := []uint32{0}
	for i := range 20 {
		patterns = append(patterns, 1<<i)
		for j := range i {
			patterns = append(patterns, 1<<i|1<<j)
			for k := range j {
				patterns = append(patterns, 1<<i|1<<j|1<<k)
			}
		}
	}
	for d := range 256 {
		cw := Encode2087(uint8(d))
		for _, e := range patterns {
			got, res := Decode2087(cw ^ e)
			if got != cw || res.ErrorsCorrected != bits.OnesCount32(e) || res.Uncorrectable {
				t.Fatalf("data %#02x error %#05x: %#05x %+v", d, e, got, res)
			}
		}
	}
}

func BenchmarkGolay2087(b *testing.B) {
	cw := Encode2087(0x5A) ^ 0x10
	b.Run("Decode", func(b *testing.B) {
		for b.Loop() {
			cw ^= 1 << 7
			_, _ = Decode2087(cw)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		d := uint8(0)
		for b.Loop() {
			d++
			_ = Encode2087(d)
		}
	})
}
