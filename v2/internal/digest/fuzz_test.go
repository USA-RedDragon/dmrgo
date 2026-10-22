//go:build !tinygo

package digest_test

import (
	"math/rand/v2"
	"testing"
)

const (
	maxPattern = 64
	maxMsg     = 4096
	maxKey     = 128
)

func FuzzDifferential(f *testing.F) {
	for _, n := range []int{0, 1, 55, 56, 63, 64, 65, 119, 120, 127, 128, 129} {
		f.Add(uint64(n), uint16(n), []byte(nil), make([]byte, n%70))
	}
	buf := make([]byte, maxMsg)
	f.Fuzz(func(t *testing.T, seed uint64, n uint16, pattern, key []byte) {
		msg := buf[:int(n)%maxMsg]
		pattern = pattern[:min(len(pattern), maxPattern)]
		clear(msg)
		for i := 0; len(pattern) > 0 && i < len(msg); {
			i += copy(msg[i:], pattern)
		}
		key = key[:min(len(key), maxKey)]
		r := rand.New(rand.NewPCG(seed, 7))
		checkSHA(t, r, msg)
		checkHMAC(t, r, key, msg)
	})
}
