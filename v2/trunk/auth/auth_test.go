package auth_test

import (
	"fmt"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/trunk/auth"
)

var testKey = [auth.KeyLen]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F, 0x10}

func TestResponseTable6_15(t *testing.T) {
	t.Parallel()

	if got := auth.Response(0x7A17C0, testKey, 0); got != 0xE8CA1D {
		t.Errorf("Response = %06X, want E8CA1D", got)
	}
}

func TestResponseTable6_16(t *testing.T) {
	t.Parallel()

	if got := auth.Response(0x7A17C0, testKey, 0x010203); got != 0xE9C81E {
		t.Errorf("Response with PSN = %06X, want E9C81E", got)
	}
}

func TestResponseHighBits(t *testing.T) {
	t.Parallel()

	if auth.Response(0xFF7A17C0, testKey, 0xFF000000) != 0xE8CA1D {
		t.Error("bits above 24 changed the response")
	}
	if auth.Response(0x7A17C1, testKey, 0) == 0xE8CA1D {
		t.Error("another challenge gave the same response")
	}
}

func TestAllocs(t *testing.T) {
	if n := testing.AllocsPerRun(100, func() { _ = auth.Response(0x123456, testKey, 0) }); n != 0 {
		t.Errorf("%.1f allocations, want 0", n)
	}
}

func FuzzResponse(f *testing.F) {
	f.Add(uint32(0x7A17C0), testKey[:], uint32(0))
	f.Fuzz(func(t *testing.T, challenge uint32, key []byte, psn uint32) {
		var k [auth.KeyLen]byte
		copy(k[:], key)
		r := auth.Response(challenge, k, psn)
		if r > 0xFFFFFF || r^auth.Response(challenge, k, 0) != psn&0xFFFFFF || r != rc4Response(challenge, k, psn) {
			t.Fatalf("Response(%06X, %x, %06X) = %06X", challenge, k, psn, r)
		}
	})
}

func rc4Response(challenge uint32, k [auth.KeyLen]byte, psn uint32) uint32 {
	key := append([]byte{byte(challenge >> 16), byte(challenge >> 8), byte(challenge)}, k[:]...)
	s := make([]int, 256)
	for i := range s {
		s[i] = i
	}
	j := 0
	for i := range 256 {
		j = (j + s[i] + int(key[i%len(key)])) % 256
		s[i], s[j] = s[j], s[i]
	}
	i, j := 0, 0
	var out []byte
	for range 259 {
		i = (i + 1) % 256
		j = (j + s[i]) % 256
		s[i], s[j] = s[j], s[i]
		out = append(out, byte(s[(s[i]+s[j])%256]))
	}
	return (uint32(out[256])<<16 | uint32(out[257])<<8 | uint32(out[258])) ^ psn&0xFFFFFF
}

// ExampleResponse computes the answer of an MS to the challenge of a
// C_AHOY, with the test vector of ETSI TS 102 361-4 Table 6.15.
func ExampleResponse() {
	key := [auth.KeyLen]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	fmt.Printf("%06X\n", auth.Response(0x7A17C0, key, 0))
	// Output:
	// E8CA1D
}
