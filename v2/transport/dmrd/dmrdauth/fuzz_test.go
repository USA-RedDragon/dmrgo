//go:build !tinygo

package dmrdauth_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/dmrdauth"
)

func FuzzBridge(f *testing.F) {
	f.Add([]byte("passw0rd"), packet("DMRD", 53, 73))
	f.Add([]byte("k"), packet("DMRE", 69, 85))
	f.Add(make([]byte, 70), packet("BCSQ", 11, 31))
	f.Fuzz(func(t *testing.T, key, p []byte) {
		b := dmrdauth.NewBridgeKey(key)
		err := b.SignInPlace(p)
		if _, ok := b.Verify(p); err == nil && !ok {
			t.Fatalf("signed packet does not verify: key %x packet %x", key, p)
		}
	})
}
