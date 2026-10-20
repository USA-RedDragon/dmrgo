//go:build !tinygo

package auth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

func FuzzBridge(f *testing.F) {
	f.Add([]byte("passw0rd"), packet("DMRD", 53, 73))
	f.Add([]byte("k"), packet("DMRE", 69, 85))
	f.Add(make([]byte, 70), packet("BCSQ", 11, 31))
	f.Fuzz(func(t *testing.T, key, p []byte) {
		b := auth.NewBridge(key)
		if err := b.Sign(p); err == nil && !b.Verify(p) {
			t.Fatalf("signed packet does not verify: key %x packet %x", key, p)
		}
	})
}
