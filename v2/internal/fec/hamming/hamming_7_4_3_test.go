package hamming

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/hamming"
)

func bits7(cw uint8) [7]bit.Bit {
	var b [7]bit.Bit
	bit.PutUint(b[:], uint64(cw))
	return b
}

func TestHamming743MatchesReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	for v := range 1 << 7 {
		cw := uint8(v)
		got, res := Decode743(cw)
		want, wantRes := ref.Decode743(bits7(cw))
		if got != uint8(bit.Uint(want[:])) || res != wantRes {
			t.Fatalf("Decode743(%#02x) = %#02x %+v, want %#02x %+v", cw, got, res, bit.Uint(want[:]), wantRes)
		}
		if Check743(cw) != ref.Check743(bits7(cw)) {
			t.Fatalf("Check743(%#02x) differs from the reference", cw)
		}
	}
	for d := range 16 {
		data := bits7(uint8(d) << 3)
		want := ref.Encode743([4]bit.Bit(data[:4]))
		if got := Encode743(uint8(d)); got != uint8(bit.Uint(want[:])) {
			t.Fatalf("Encode743(%#x) = %#02x, want %#02x", d, got, bit.Uint(want[:]))
		}
	}
}

func TestHamming743CorrectsOneError(t *testing.T) {
	t.Parallel()

	for d := range 16 {
		cw := Encode743(uint8(d))
		if !Check743(cw) {
			t.Fatalf("Encode743(%#x) = %#02x is not a codeword", d, cw)
		}
		for i := range 7 {
			got, res := Decode743(cw ^ 1<<i)
			if got != cw || res.ErrorsCorrected != 1 || res.Uncorrectable || res.BitsChecked != 7 {
				t.Fatalf("data %#x error bit %d: %#02x %+v", d, i, got, res)
			}
		}
	}
}

func BenchmarkHamming743(b *testing.B) {
	cw := Encode743(0x5) ^ 0x04
	b.Run("Decode", func(b *testing.B) {
		for b.Loop() {
			cw ^= 1 << 5
			_, _ = Decode743(cw)
		}
	})
	b.Run("Encode", func(b *testing.B) {
		d := uint8(0)
		for b.Loop() {
			d++
			_ = Encode743(d)
		}
	})
}
