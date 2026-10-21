//go:build !tinygo

package digest_test

import (
	"math/rand/v2"
	"testing"
)

func FuzzDifferential(f *testing.F) {
	for _, n := range []int{0, 1, 55, 56, 63, 64, 65, 119, 120, 127, 128, 129} {
		f.Add(uint64(n), make([]byte, n), make([]byte, n%70))
	}
	f.Fuzz(func(t *testing.T, seed uint64, msg, key []byte) {
		r := rand.New(rand.NewPCG(seed, 7))
		checkSHA(t, r, msg)
		checkHMAC(t, r, key, msg)
	})
}
