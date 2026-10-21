// Package xcrypto_test compares package digest with golang.org/x/crypto.
// It is a module of its own, so the v2 module does not depend on
// golang.org/x/crypto.
package xcrypto_test

import (
	"bytes"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
	"golang.org/x/crypto/blake2b"
)

func randBytes(r *rand.Rand, n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(r.Uint32())
	}
	return p
}

func checkBLAKE2b(t *testing.T, r *rand.Rand, size int, key, msg []byte) {
	t.Helper()
	ref, err := blake2b.New(size, key)
	if err != nil {
		t.Fatalf("x/crypto size %d key %d: %v", size, len(key), err)
	}
	ref.Write(msg)
	want := ref.Sum(nil)
	var d digest.BLAKE2b
	if !d.Init(size, key) {
		t.Fatalf("Init size %d key %d", size, len(key))
	}
	for range 2 {
		p := msg
		for len(p) > 0 {
			n := min(len(p), r.IntN(200))
			d.Write(p[:n])
			p = p[n:]
		}
		out := make([]byte, size)
		d.Sum(out)
		if !bytes.Equal(out, want) {
			t.Fatalf("size %d key %x message %x: %x, want %x", size, key, msg, out, want)
		}
		d.Reset()
	}
}

func TestDifferentialBLAKE2b(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(8, 9))
	sizes := make([]int, 0, 300)
	for n := range 260 {
		sizes = append(sizes, n)
	}
	sizes = append(sizes, 383, 384, 385, 511, 512, 513, 1023, 1024, 1025, 4096)
	for _, n := range sizes {
		checkBLAKE2b(t, r, 1+r.IntN(64), randBytes(r, r.IntN(65)), randBytes(r, n))
	}
	for size := 1; size <= 64; size++ {
		for _, k := range []int{0, 1, 16, 63, 64} {
			checkBLAKE2b(t, r, size, randBytes(r, k), randBytes(r, r.IntN(300)))
		}
	}
}

func FuzzBLAKE2b(f *testing.F) {
	for _, n := range []int{0, 1, 127, 128, 129, 255, 256, 257} {
		f.Add(uint8(16), make([]byte, n%65), make([]byte, n), uint64(n))
	}
	f.Fuzz(func(t *testing.T, size uint8, key, msg []byte, seed uint64) {
		s := 1 + int(size)%64
		if len(key) > 64 {
			key = key[:64]
		}
		checkBLAKE2b(t, rand.New(rand.NewPCG(seed, 10)), s, key, msg)
	})
}
