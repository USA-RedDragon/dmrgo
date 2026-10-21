package digest_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

func diffSizes() []int {
	sizes := make([]int, 0, 340)
	for n := range 300 {
		sizes = append(sizes, n)
	}
	for _, b := range []int{320, 384, 448, 512, 1024, 4096} {
		sizes = append(sizes, b-9, b-8, b-1, b, b+1)
	}
	return sizes
}

func writeChunks(r *rand.Rand, p []byte, w func([]byte)) {
	for len(p) > 0 {
		n := min(len(p), r.IntN(130))
		w(p[:n])
		p = p[n:]
	}
	w(nil)
}

func randBytes(r *rand.Rand, n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(r.Uint32())
	}
	return p
}

func checkSHA(t *testing.T, r *rand.Rand, msg []byte) {
	t.Helper()
	var s1 digest.SHA1
	s1.Reset()
	writeChunks(r, msg, s1.Write)
	if got, want := s1.Sum(), sha1.Sum(msg); got != want || s1.Sum() != want {
		t.Fatalf("SHA-1 of %x: %x, want %x", msg, got, want)
	}
	var s2 digest.SHA256
	s2.Reset()
	writeChunks(r, msg, s2.Write)
	if got, want := s2.Sum(), sha256.Sum256(msg); got != want || s2.Sum() != want {
		t.Fatalf("SHA-256 of %x: %x, want %x", msg, got, want)
	}
}

func checkHMAC(t *testing.T, r *rand.Rand, key, msg []byte) {
	t.Helper()
	ref := hmac.New(sha1.New, key)
	ref.Write(msg)
	want := ref.Sum(nil)
	var m digest.HMACSHA1
	m.Init(key)
	for range 2 {
		m.Reset()
		writeChunks(r, msg, m.Write)
		if got := m.Sum(); !bytes.Equal(got[:], want) {
			t.Fatalf("HMAC-SHA1 key %x of %x: %x, want %x", key, msg, got, want)
		}
	}
}

func TestDifferentialSHA(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range diffSizes() {
		checkSHA(t, r, randBytes(r, n))
	}
	for range 200 {
		checkSHA(t, r, randBytes(r, r.IntN(2000)))
	}
}

func TestDifferentialHMACSHA1(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(3, 4))
	for k := range 200 {
		checkHMAC(t, r, randBytes(r, k), randBytes(r, r.IntN(300)))
	}
	for _, n := range diffSizes() {
		checkHMAC(t, r, randBytes(r, r.IntN(100)), randBytes(r, n))
	}
}
