package digest_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

func TestAllocsDigest(t *testing.T) {
	msg := testData(200)
	key := testData(80)
	var s1 digest.SHA1
	var s2 digest.SHA256
	var m digest.HMACSHA1
	var b digest.BLAKE2b
	var out [64]byte
	n := testing.AllocsPerRun(100, func() {
		s1.Reset()
		s1.Write(msg)
		a := s1.Sum()
		s2.Reset()
		s2.Write(msg)
		c := s2.Sum()
		m.Init(key)
		m.Write(msg)
		h := m.Sum()
		b.Init(16, key[:64])
		b.Write(msg)
		b.Sum(out[:])
		_ = digest.Equal(a[:], h[:]) || digest.Equal(c[:16], out[:16])
	})
	if n != 0 {
		t.Fatalf("%v allocations", n)
	}
}
