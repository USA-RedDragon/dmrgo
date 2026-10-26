package privacy

import (
	"bytes"
	"crypto/aes"
	"crypto/rc4"
	"encoding/hex"
	"math/rand/v2"
	"testing"
)

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestARC4RFC6229(t *testing.T) {
	t.Parallel()

	for _, v := range []struct {
		key string
		out map[int]string
	}{
		{"0102030405", map[int]string{
			0:    "b2396305f03dc027ccc3524a0a1118a8",
			16:   "6982944f18fc82d589c403a47a0d0919",
			240:  "28cb1132c96ce286421dcaadb8b69eae",
			256:  "1cfcf62b03eddb641d77dfcf7f8d8c93",
			1008: "45129048e6a0ed0b56b490338f078da5",
		}},
		{"833222772a", map[int]string{
			0:    "80ad97bdc973df8a2e879e92a497efda",
			256:  "f0daf5bab3d596839857846f73fbfe5a",
			1024: "7828d074119cab5c22b294d7a9bfa0bb",
		}},
	} {
		var c arc4
		c.init(unhex(t, v.key))
		var stream [1040]byte
		for i := range stream {
			stream[i] = c.next()
		}
		for off, want := range v.out {
			w := unhex(t, want)
			if got := stream[off : off+16]; !bytes.Equal(got, w) {
				t.Errorf("key %s offset %d: %x, want %x", v.key, off, got, w)
			}
		}
	}
}

func TestARC4MatchesStandardLibrary(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	for range 200 {
		key := make([]byte, 1+r.IntN(32))
		for i := range key {
			key[i] = byte(r.Uint32())
		}
		ref, err := rc4.NewCipher(key)
		if err != nil {
			t.Fatal(err)
		}
		want := make([]byte, 600)
		ref.XORKeyStream(want, want)
		var c arc4
		c.init(key)
		for i := range want {
			if got := c.next(); got != want[i] {
				t.Fatalf("key %x octet %d: %02x, want %02x", key, i, got, want[i])
			}
		}
	}
}

func TestOFBSP80038A(t *testing.T) {
	t.Parallel()

	for _, v := range []struct {
		name, key string
		out       []string
	}{
		{"F.4.1", "2b7e151628aed2a6abf7158809cf4f3c", []string{
			"50fe67cc996d32b6da0937e99bafec60", "d9a4dada0892239f6b8b3d7680e15674",
			"a78819583f0308e7a6bf36b1386abf23", "c6d3416d29165c6fcb8e51a227ba994e",
		}},
		{"F.4.5", "603deb1015ca71be2b73aef0857d77811f352c073b6108d72d9810a30914dff4", []string{
			"b7bf3a5df43989dd97f0fa97ebce2f4a", "e1c656305ed1a7a6563805746fe03edc",
			"41635be625b48afc1666dd42a09d96e7", "f7b93058b8bce0fffea41bf0012cd394",
		}},
	} {
		b, err := aes.NewCipher(unhex(t, v.key))
		if err != nil {
			t.Fatal(err)
		}
		blk := [16]byte(unhex(t, "000102030405060708090a0b0c0d0e0f"))
		var got [64]byte
		ofb(b, &blk, got[:])
		for i, want := range v.out {
			if w := unhex(t, want); !bytes.Equal(got[i*16:i*16+16], w) {
				t.Errorf("%s block %d: %x, want %x", v.name, i+1, got[i*16:i*16+16], w)
			}
		}
	}
}

func TestMessageIndicatorSteps(t *testing.T) {
	t.Parallel()

	for _, v := range []struct{ mi, next uint32 }{{0x531699C4, 0x178D0B9B}, {0x178D0B9B, 0x7F900958}} {
		if got := arc4Step(v.mi); got != v.next {
			t.Errorf("ARC4 step of %08X: %08X, want %08X", v.mi, got, v.next)
		}
	}
	for _, v := range []struct {
		mi uint32
		iv string
	}{
		{0x25E36674, "25e36674e04e2a47cd32f1f4052aec80"},
		{0xE04E2A47, "e04e2a47cd32f1f4052aec80e57be41c"},
	} {
		if iv := aesIV(v.mi); !bytes.Equal(iv[:], unhex(t, v.iv)) {
			t.Errorf("IV of %08X: %x, want %s", v.mi, iv, v.iv)
		}
	}
}
