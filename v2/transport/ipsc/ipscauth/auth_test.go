package ipscauth_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/ipsc/ipscauth"
)

func TestSignMatchesPython(t *testing.T) {
	t.Parallel()

	k, err := ipscauth.ParseKey("123456789abcdef1234")
	if err != nil {
		t.Fatal(err)
	}
	if hex.EncodeToString(k[:]) != "00000000000000000000000123456789abcdef1234"[2:] {
		t.Fatalf("key %x", k)
	}
	key := ipscauth.NewKey(k)
	for _, tt := range []struct{ body, trailer string }{
		{"900004ce2e6a0000805c04080400", "5a09decc4db02fba02dc"},
		{"96003010af6a000080dc04020401", "712d51b8061daed9d7ce"},
	} {
		b, _ := hex.DecodeString(tt.body)
		p, _ := key.Sign(append([]byte{}, b...))
		if got := hex.EncodeToString(p[len(b):]); got != tt.trailer {
			t.Errorf("%s: %s, want %s", tt.body, got, tt.trailer)
		}
		body, ok := key.Verify(p)
		if !ok || !bytes.Equal(body, b) {
			t.Errorf("%s: does not verify", tt.body)
		}
		p[3] ^= 1
		if _, ok := key.Verify(p); ok {
			t.Errorf("%s: changed packet verifies", tt.body)
		}
	}
	if _, ok := key.Verify([]byte{1, 2}); ok {
		t.Error("short packet verifies")
	}
}

func TestParseKeyErrors(t *testing.T) {
	t.Parallel()

	for _, s := range []string{"xyz", "0123456789012345678901234567890123456789A"} {
		if _, err := ipscauth.ParseKey(s); err == nil {
			t.Errorf("%q: no error", s)
		}
	}
	k, err := ipscauth.ParseKey("0123456789ABCDEF0123456789abcdef01234567")
	if err != nil || hex.EncodeToString(k[:]) != "0123456789abcdef0123456789abcdef01234567" {
		t.Fatalf("%x %v", k, err)
	}
}

func TestKeyNil(t *testing.T) {
	t.Parallel()

	var k *ipscauth.Key
	if out, err := k.Sign([]byte{1, 2, 3}); err == nil || len(out) != 3 {
		t.Fatal("nil Key signs")
	}
	if body, ok := k.Verify(make([]byte, 20)); ok || body != nil {
		t.Fatal("nil Key verifies")
	}
}
