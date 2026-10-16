package quadraticresidue

import (
	"math/bits"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/quadraticresidue"
)

func bits16(cw uint16) [16]bit.Bit {
	var b [16]bit.Bit
	bit.PutUint(b[:], uint64(cw))
	return b
}

func TestQR1676MatchesReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	for v := range 1 << 16 {
		cw := uint16(v)
		got, res := Decode(cw)
		want, wantRes := ref.Decode(bits16(cw))
		if got != uint16(bit.Uint(want[:])) || res != wantRes {
			t.Fatalf("Decode(%#04x) = %#04x %+v, want %#04x %+v", cw, got, res, bit.Uint(want[:]), wantRes)
		}
		if Check(cw) != ref.Check(bits16(cw)) {
			t.Fatalf("Check(%#04x) differs from the reference", cw)
		}
	}
	for d := range 128 {
		data := bits16(uint16(d) << 9)
		want := ref.Encode([7]bit.Bit(data[:7]))
		if got := Encode(uint8(d)); got != uint16(bit.Uint(want[:])) {
			t.Fatalf("Encode(%#02x) = %#04x, want %#04x", d, got, bit.Uint(want[:]))
		}
	}
}

func TestQR1676CorrectsUpToTwoErrors(t *testing.T) {
	t.Parallel()

	patterns := []uint16{0}
	for i := range 16 {
		patterns = append(patterns, 1<<i)
		for j := range i {
			patterns = append(patterns, 1<<i|1<<j)
		}
	}
	for d := range 128 {
		cw := Encode(uint8(d))
		for _, e := range patterns {
			got, res := Decode(cw ^ e)
			if got != cw || res.ErrorsCorrected != bits.OnesCount16(e) || res.Uncorrectable {
				t.Fatalf("data %#02x error %#04x: %#04x %+v", d, e, got, res)
			}
		}
	}
}

func BenchmarkQR1676(b *testing.B) {
	cw := Encode(0x5A) ^ 0x20
	b.Run("Decode", func(b *testing.B) {
		for b.Loop() {
			cw ^= 1 << 3
			_, _ = Decode(cw)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		d := uint8(0)
		for b.Loop() {
			d++
			_ = Encode(d)
		}
	})
}
