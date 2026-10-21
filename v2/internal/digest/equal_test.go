package digest_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

func TestEqual(t *testing.T) {
	t.Parallel()

	for n := range 70 {
		a := testData(n + 3)[3:]
		if !digest.Equal(a, append([]byte(nil), a...)) {
			t.Fatalf("%x differs from itself", a)
		}
		for i := range n {
			for _, bit := range []byte{0x01, 0x80} {
				b := append([]byte(nil), a...)
				b[i] ^= bit
				if digest.Equal(a, b) {
					t.Fatalf("%x equals %x", a, b)
				}
			}
		}
		if digest.Equal(a, append(append([]byte(nil), a...), 0)) {
			t.Fatalf("%x equals a longer slice", a)
		}
	}
	if !digest.Equal(nil, []byte{}) {
		t.Fatal("nil differs from empty")
	}
}
