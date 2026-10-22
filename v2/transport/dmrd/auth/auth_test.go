package auth_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd"
	"github.com/USA-RedDragon/dmrgo/v2/transport/dmrd/auth"
)

func TestTokenHBRPSpecExample(t *testing.T) {
	t.Parallel()

	got, err := auth.Token(dmrd.DialectHBRP, 0x0A7ED498, []byte("DL5DI"))
	if err != nil || hex.EncodeToString(got[:]) != "cbf0e29abbd11c6573825d36a664e3064441f91fc815ac5dc5ca570d4c4a5f85" {
		t.Fatalf("%x %v", got, err)
	}
}

func TestTokenMMDVM(t *testing.T) {
	t.Parallel()

	got, err := auth.Token(dmrd.DialectMMDVM, 0xDEADBEEF, []byte("passw0rd"))
	if err != nil || hex.EncodeToString(got[:]) != "7dc06de70732a897f9b0a50886a3e875c8cedb39c66b393867adfb20621dfcd6" {
		t.Fatalf("%x %v", got, err)
	}
	if _, err := auth.Token(dmrd.DialectLocal, 1, nil); err == nil {
		t.Error("local dialect: no error")
	}
	if !auth.VerifyToken(dmrd.DialectMMDVM, 0xDEADBEEF, []byte("passw0rd"), got[:]) {
		t.Error("VerifyToken rejects its token")
	}
	for i := range got {
		bad := got
		bad[i] ^= 0x10
		if auth.VerifyToken(dmrd.DialectMMDVM, 0xDEADBEEF, []byte("passw0rd"), bad[:]) {
			t.Errorf("VerifyToken accepts a change in octet %d", i)
		}
	}
	if auth.VerifyToken(dmrd.DialectMMDVM, 0xDEADBEEF, []byte("passw0rd"), got[:31]) || auth.VerifyToken(dmrd.DialectLocal, 0xDEADBEEF, []byte("passw0rd"), got[:]) {
		t.Error("VerifyToken accepts a short token or the local dialect")
	}
	long := bytes.Repeat([]byte("x"), 300)
	got, err = auth.Token(dmrd.DialectMMDVM, 0xDEADBEEF, long)
	if err != nil || got != sha256.Sum256(append([]byte{0xDE, 0xAD, 0xBE, 0xEF}, long...)) {
		t.Errorf("300-octet password: %x %v", got, err)
	}
}

func packet(tag string, msgLen, n int) []byte {
	p := make([]byte, n)
	copy(p, tag)
	for i := 4; i < msgLen; i++ {
		p[i] = byte(i)
	}
	return p
}

func TestBridgeSignaturesMatchPython(t *testing.T) {
	t.Parallel()

	b := auth.NewBridgeKey([]byte("passw0rd"))
	tests := []struct {
		p      []byte
		sigAt  int
		sigHex string
	}{
		{packet("DMRD", 53, 73), 53, "ed07b50531fa7a96159f796fa6d6934fb70e9b90"},
		{packet("DMRE", 69, 85), 69, "6ee460d8b78607eef430890b04a781ad"},
		{packet("DMRE", 73, 89), 73, "ef26784919d293e2d58fd85242e40666"},
		{packet("BCKA", 4, 24), 4, "5577d4a35e2fe3b307b2cfe3a79da4745baf45fb"},
		{packet("BCSQ", 11, 31), 11, "4a41d40a117097a8093ba2fb4b19ea3266b61a6e"},
		{packet("BCVE", 5, 25), 5, "ae865fe930ad93f8da278237b85b2e7483682ef5"},
	}
	for _, tt := range tests {
		if _, ok := b.Verify(tt.p); ok {
			t.Errorf("%s/%d: unsigned packet verifies", tt.p[:4], len(tt.p))
		}
		if err := b.SignInPlace(tt.p); err != nil {
			t.Fatalf("%s/%d: %v", tt.p[:4], len(tt.p), err)
		}
		if got := hex.EncodeToString(tt.p[tt.sigAt:]); got != tt.sigHex {
			t.Errorf("%s/%d: %s, want %s", tt.p[:4], len(tt.p), got, tt.sigHex)
		}
		if body, ok := b.Verify(tt.p); !ok || !bytes.Equal(body, tt.p[:tt.sigAt]) {
			t.Errorf("%s/%d: signed packet does not verify, body %x", tt.p[:4], len(tt.p), body)
		}
		tt.p[len(tt.p)-1] ^= 1
		if _, ok := b.Verify(tt.p); ok {
			t.Errorf("%s/%d: changed signature verifies", tt.p[:4], len(tt.p))
		}
	}
}

func TestBridgeLongKey(t *testing.T) {
	t.Parallel()

	b := auth.NewBridgeKey(bytes.Repeat([]byte("k"), 100))
	p := packet("DMRD", 53, 73)
	if err := b.SignInPlace(p); err != nil || hex.EncodeToString(p[53:]) != "074d5ec72685e6bd0cbb917aabb83108726b943a" {
		t.Fatalf("%x %v", p[53:], err)
	}
	e := packet("DMRE", 69, 85)
	if err := b.SignInPlace(e); err != nil || hex.EncodeToString(e[69:]) != "a943a3ca14b6731ef293c0eed48a9760" {
		t.Fatalf("DMRE with a 100-octet key: %x %v", e[69:], err)
	}
	if _, ok := auth.NewBridgeKey(bytes.Repeat([]byte("k"), auth.KeyLen)).Verify(e); !ok {
		t.Error("the key is not cut at KeyLen")
	}
	for _, p := range [][]byte{nil, []byte("RPTL1234"), packet("DMRD", 53, 72)} {
		err := b.SignInPlace(p)
		_, ok := b.Verify(p)
		if err == nil || ok {
			t.Errorf("%q: signs or verifies", p)
		}
	}
}

func TestBridgeKeyNil(t *testing.T) {
	t.Parallel()

	var b *auth.BridgeKey
	p := packet("DMRD", 53, 73)
	if err := b.SignInPlace(p); err == nil {
		t.Fatal("nil BridgeKey signs")
	}
	if body, ok := b.Verify(p); ok || body != nil {
		t.Fatal("nil BridgeKey verifies")
	}
}
