package golay

import (
	"fmt"
	"math/bits"
	"math/rand"
	"testing"

	ref "github.com/USA-RedDragon/dmrgo/v2/internal/fecref/golay"
)

func TestGolay23127And24128EncodeMatchReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	for d := range 1 << 16 {
		data := uint16(d)
		if got, want := Encode23127(data), ref.Encode23127(data); got != want {
			t.Fatalf("Encode23127(%#x) = %#x, want %#x", data, got, want)
		}
		if got, want := Encode24128(data), ref.Encode24128(data); got != want {
			t.Fatalf("Encode24128(%#x) = %#x, want %#x", data, got, want)
		}
	}
}

func TestGolay23127DecodeMatchesReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	everyWord(t, 1<<23, func(w uint32) string {
		data, res := Decode23127(w)
		if wantData, wantRes := ref.Decode23127(w); data != wantData || res != wantRes {
			return fmt.Sprintf("Decode23127(%#x) = %#x %+v, want %#x %+v", w, data, res, wantData, wantRes)
		}
		if res.Uncorrectable || bits.OnesCount32(Encode23127(data)^w) != res.ErrorsCorrected || res.ErrorsCorrected > 3 {
			return fmt.Sprintf("Decode23127(%#x) = %#x %+v: not the codeword within 3 bits", w, data, res)
		}
		return ""
	})
	for _, w := range []uint32{1 << 23, 1<<31 | 0x2A5A5A, 0xFFFFFFFF} {
		data, res := Decode23127(w)
		if wantData, wantRes := ref.Decode23127(w); data != wantData || res != wantRes {
			t.Fatalf("Decode23127(%#x) = %#x %+v, want %#x %+v", w, data, res, wantData, wantRes)
		}
	}
}

func TestGolay24128DecodeMatchesReferenceOnEveryWord(t *testing.T) {
	t.Parallel()

	everyWord(t, 1<<24, func(w uint32) string {
		data, res := Decode24128(w)
		if wantData, wantRes := ref.Decode24128(w); data != wantData || res != wantRes {
			return fmt.Sprintf("Decode24128(%#x) = %#x %+v, want %#x %+v", w, data, res, wantData, wantRes)
		}
		return ""
	})
	for _, w := range []uint32{1 << 24, 1<<31 | 0xA5A5A5, 0xFFFFFFFF} {
		data, res := Decode24128(w)
		if wantData, wantRes := ref.Decode24128(w); data != wantData || res != wantRes {
			t.Fatalf("Decode24128(%#x) = %#x %+v, want %#x %+v", w, data, res, wantData, wantRes)
		}
	}
}

func benchWords(n, maxErrors int, encode func(uint16) uint32) []uint32 {
	r := rand.New(rand.NewSource(5))
	words := make([]uint32, 256)
	for i := range words {
		w := encode(uint16(r.Intn(4096)))
		for _, p := range r.Perm(n)[:i%(maxErrors+1)] {
			w ^= 1 << p
		}
		words[i] = w
	}
	return words
}

func BenchmarkGolay23127(b *testing.B) {
	words := benchWords(23, 3, Encode23127)
	b.Run("Decode", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = Decode23127(words[i&255])
			i++
		}
	})
	b.Run("Encode", func(b *testing.B) {
		d := uint16(0)
		for b.Loop() {
			d++
			_ = Encode23127(d)
		}
	})
}

func BenchmarkGolay24128(b *testing.B) {
	words := benchWords(24, 4, Encode24128)
	b.Run("Decode", func(b *testing.B) {
		i := 0
		for b.Loop() {
			_, _ = Decode24128(words[i&255])
			i++
		}
	})
	b.Run("Encode", func(b *testing.B) {
		d := uint16(0)
		for b.Loop() {
			d++
			_ = Encode24128(d)
		}
	})
}
