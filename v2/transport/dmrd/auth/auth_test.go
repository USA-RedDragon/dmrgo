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

	b := auth.NewBridge([]byte("passw0rd"))
	tests := []struct {
		p      []byte
		sigAt  int
		sigHex string
	}{
		{packet("DMRD", 53, 73), 53, "ed07b50531fa7a96159f796fa6d6934fb70e9b90"},
		{packet("DMRE", 69, 85), 69, "b4f519b8627e52b5f79319f463ea15fb"},
		{packet("DMRE", 73, 89), 73, "8f9cfabfdf7007dc4c00be83e48f7b7e"},
		{packet("BCKA", 4, 24), 4, "5577d4a35e2fe3b307b2cfe3a79da4745baf45fb"},
		{packet("BCSQ", 11, 31), 11, "4a41d40a117097a8093ba2fb4b19ea3266b61a6e"},
		{packet("BCVE", 5, 25), 5, "ae865fe930ad93f8da278237b85b2e7483682ef5"},
	}
	for _, tt := range tests {
		if b.Verify(tt.p) {
			t.Errorf("%s/%d: unsigned packet verifies", tt.p[:4], len(tt.p))
		}
		if err := b.Sign(tt.p); err != nil {
			t.Fatalf("%s/%d: %v", tt.p[:4], len(tt.p), err)
		}
		if got := hex.EncodeToString(tt.p[tt.sigAt:]); got != tt.sigHex {
			t.Errorf("%s/%d: %s, want %s", tt.p[:4], len(tt.p), got, tt.sigHex)
		}
		if !b.Verify(tt.p) {
			t.Errorf("%s/%d: signed packet does not verify", tt.p[:4], len(tt.p))
		}
		tt.p[len(tt.p)-1] ^= 1
		if b.Verify(tt.p) {
			t.Errorf("%s/%d: changed signature verifies", tt.p[:4], len(tt.p))
		}
	}
}

func TestBridgeLongKey(t *testing.T) {
	t.Parallel()

	b := auth.NewBridge(bytes.Repeat([]byte("k"), 100))
	p := packet("DMRD", 53, 73)
	if err := b.Sign(p); err != nil || hex.EncodeToString(p[53:]) != "e264f16a07726418bde5cf09e2d802a92f0c341c" {
		t.Fatalf("%x %v", p[53:], err)
	}
	if err := b.Sign(packet("DMRE", 69, 85)); err == nil {
		t.Error("BLAKE2b with a 100-octet key: no error")
	}
	for _, p := range [][]byte{nil, []byte("RPTL1234"), packet("DMRD", 53, 72)} {
		if err := b.Sign(p); err == nil || b.Verify(p) {
			t.Errorf("%q: signs or verifies", p)
		}
	}
}
