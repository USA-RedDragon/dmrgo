//go:build !tinygo

package ipscauth_test

import (
	"bytes"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
)

func FuzzKey(f *testing.F) {
	f.Add("1234abcd", []byte{0x96, 0, 0, 0, 1})
	f.Add("", []byte{})
	f.Add("0123456789abcdef0123456789ABCDEF01234567", make([]byte, 80))
	f.Fuzz(func(t *testing.T, s string, p []byte) {
		k, err := ipscauth.ParseKey(s)
		if err != nil {
			return
		}
		key := ipscauth.NewKey(k)
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
