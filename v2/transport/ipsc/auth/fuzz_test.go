//go:build !tinygo

package auth_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/auth"
)

func FuzzKey(f *testing.F) {
	f.Add("1234abcd", []byte{0x96, 0, 0, 0, 1})
	f.Add("", []byte{})
	f.Add("0123456789abcdef0123456789ABCDEF01234567", make([]byte, 80))
	f.Fuzz(func(t *testing.T, s string, p []byte) {
		k, err := auth.ParseKey(s)
		if err != nil {
			return
		}
		key := auth.NewKey(k)
		signed, _ := key.Sign(append([]byte(nil), p...))
		body, ok := key.Verify(signed)
		if !ok || !bytes.Equal(body, p) {
			t.Fatalf("signed packet does not verify: key %q packet %x", s, p)
		}
		signed[len(signed)-1] ^= 1
		if _, ok := key.Verify(signed); ok {
			t.Fatalf("changed trailer verifies: key %q packet %x", s, p)
		}
	})
}
