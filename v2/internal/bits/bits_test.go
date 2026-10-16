package bits

import (
	"math/rand"
	"testing"
)

func refField(b []byte, start, n int) uint64 {
	var v uint64
	for i := range n {
		v = v<<1 | uint64(b[(start+i)/8]>>(7-(start+i)%8)&1)
	}
	return v
}

func TestFieldMatchesBitByBit(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(1))
	b := make([]byte, 33)
	r.Read(b)
	for start := 0; start < 264; start++ {
		for n := 0; n <= 64 && start+n <= 264; n++ {
			want := refField(b, start, n)
			if n <= 57 {
				if got := Field(b, start, n); got != want {
					t.Fatalf("Field(%d, %d) = %x, want %x", start, n, got, want)
				}
			}
			if got := Field64(b, start, n); got != want {
				t.Fatalf("Field64(%d, %d) = %x, want %x", start, n, got, want)
			}
		}
	}
}

func TestSetFieldWritesOnlyTheField(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(2))
	for range 20000 {
		n := r.Intn(65)
		start := r.Intn(264 - n + 1)
		v := r.Uint64()
		b := make([]byte, 33)
		r.Read(b)
		want := append([]byte(nil), b...)
		for i := range n {
			SetBit(want, start+i, byte(v>>(n-1-i)))
		}
		SetField64(b, start, n, v)
		if string(b) != string(want) {
			t.Fatalf("SetField64(%d, %d, %x) = %x, want %x", start, n, v, b, want)
		}
	}
}

func TestCopy(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewSource(3))
	for range 5000 {
		src := make([]byte, 33)
		dst := make([]byte, 33)
		r.Read(src)
		r.Read(dst)
		n := r.Intn(200)
		ss, ds := r.Intn(264-n+1), r.Intn(264-n+1)
		want := append([]byte(nil), dst...)
		for i := range n {
			SetBit(want, ds+i, Bit(src, ss+i))
		}
		Copy(dst, ds, src, ss, n)
		if string(dst) != string(want) {
			t.Fatalf("Copy(%d, %d, %d) = %x, want %x", ds, ss, n, dst, want)
		}
	}
}
