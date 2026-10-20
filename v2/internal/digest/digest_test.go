package digest_test

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

func testData(n int) []byte {
	p := make([]byte, n)
	for i := range p {
		p[i] = byte(i * 7 % 251)
	}
	return p
}

func TestBLAKE2bMatchesPython(t *testing.T) {
	t.Parallel()

	tests := []struct {
		n            int
		keyed, plain string
	}{
		{0, "7726dd13669efe07de93c1cad50e382a", "786a02f742015903c6c6fd852552d272912f4740e15847618a86e217f71f5419d25e1031afee585313896444934eb04b903a685b1448b755d56f701afe9be2ce"},
		{1, "958ecb7a4af8eb57ebc2afd4384344f9", "2fa3f686df876995167e7c2e5d74c4c7b6e48f8068fe0e44208344d480f7904c36963e44115fe3eb2a3ac8694c28bcb4f5a0f3276f2e79487d8219057a506e4b"},
		{63, "c31bae6c64e317b9fbb5835897503a59", "6672eec62f1ab4cbff898e0864b2d56ea4f1fbd0a0c7700377d391b411f4989dae44c82a36cf72c16553ac5e045de141cb128cc2607bd64070fac5bcc969dd64"},
		{64, "544e2abf9312dcae99070ed41b654457", "92f27f12b0e0171f4e6aec16f31f3865397e7dcdee8f72091d52be2e16af7e060b0de35ae33a4f5d98f7ddbb5ecb5979c7e00aab585ed8f9eee371704984d3d6"},
		{65, "a6695a946c792cceaa9282ee7bdbd20a", "798f4faee1547f633b2b37973e2e1540b808568f22ee463c0a81a32fcfe4e3945b6031f325f33055d6f52c8e01d6a8fac9a725a55b6dc93cb3e5142dff843a5b"},
		{127, "848ca637d9569cd4acdf852d662bd0c6", "ec2b4ebd83459a83ba49bdfaea2cf0df796dfa2ecd38c826ea407c6e6f655405b42d21eac642d131b866a14fce46de8adbf0341a35d79c3f0a54a54095330291"},
		{128, "daece433244bb4b26b4ce6f7eb140e7c", "3c2d4866bfb0691d821b7c8ff0bca5550132168058d777cddf8674b896aa2c34a98f340aa52cab29464bdeb22e0a0c889ccc35456e1ea350ea157ff5b5ebc27d"},
		{129, "02d388c297e0ce3b235e866a21b7581f", "2298fdc7acc41f58a2833279c6e4439aef2fe12c161891cb6568163732d079f1090e994da16838874ba0adc784091deeda825bb8dc9a99b1c711579700d4c3bf"},
		{255, "581e9ebb8e1a4201a181dd51602465b4", "cbef65fb62b4d72781646534eefa7b2087ea7fb07d47eabbf16e5c8956e57cc8831654d96cf0898add8cab835f9f5f6f66018f6de31101e030d8cacf4f296338"},
		{256, "e9505a41d458a2e2da65f72cb638ea89", "b0087fc9e88e473ae9da7b28e54f4840e3e60d322341ee6487df263f0f8a50c71d992ab6d6ea8f14957758fdf3a0008b48e2b7c8ec265b912c4e0d369709e96e"},
		{1000, "760efde09cb789d625b1348fe32cb2d8", "b611add97bcaee5ccef087e5afe33b18a680ada1bc8fbd70da5664e54dec31b118fbca4ea5ab704f61802c952d74679bdd9b210b77740955c872c23504c9c5f9"},
	}
	data := testData(1000)
	var d digest.BLAKE2b
	for _, tt := range tests {
		if !d.Init(16, []byte("passw0rd")) {
			t.Fatal("Init failed")
		}
		for i := 0; i < tt.n; i += 50 {
			d.Write(data[i:min(i+50, tt.n)])
		}
		var out [64]byte
		d.Sum(out[:])
		if got := hex.EncodeToString(out[:16]); got != tt.keyed {
			t.Errorf("keyed %d: %s, want %s", tt.n, got, tt.keyed)
		}
		d.Init(64, nil)
		d.Write(data[:tt.n])
		d.Sum(out[:])
		if got := hex.EncodeToString(out[:]); got != tt.plain {
			t.Errorf("plain %d: %s, want %s", tt.n, got, tt.plain)
		}
	}
	if d.Init(0, nil) || d.Init(65, nil) || d.Init(16, make([]byte, 65)) {
		t.Error("Init accepts a bad size or key")
	}
}

func TestBLAKE2bRFC7693(t *testing.T) {
	t.Parallel()

	var d digest.BLAKE2b
	d.Init(64, nil)
	d.Write([]byte("abc"))
	var out [64]byte
	d.Sum(out[:])
	const want = "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"
	if got := hex.EncodeToString(out[:]); got != want {
		t.Fatalf("%s", got)
	}
}

func TestSHAMatchesStandardLibrary(t *testing.T) {
	t.Parallel()

	r := rand.New(rand.NewPCG(1, 2))
	var s1 digest.SHA1
	var s2 digest.SHA256
	for range 300 {
		p := make([]byte, r.IntN(400))
		for i := range p {
			p[i] = byte(r.Uint32())
		}
		s1.Reset()
		s2.Reset()
		cut := r.IntN(len(p) + 1)
		s1.Write(p[:cut])
		s1.Write(p[cut:])
		s2.Write(p[:cut])
		s2.Write(p[cut:])
		if s1.Sum() != sha1.Sum(p) {
			t.Fatalf("SHA-1 of %d octets", len(p))
		}
		if s2.Sum() != sha256.Sum256(p) {
			t.Fatalf("SHA-256 of %d octets", len(p))
		}
		key := p[:r.IntN(len(p)+1)]
		var m digest.HMACSHA1
		m.Init(key)
		m.Write(p)
		ref := hmac.New(sha1.New, key)
		ref.Write(p)
		if got := m.Sum(); hex.EncodeToString(got[:]) != hex.EncodeToString(ref.Sum(nil)) {
			t.Fatalf("HMAC-SHA1 key %d octets, message %d", len(key), len(p))
		}
		m.Reset()
		m.Write(p[:cut])
		m.Write(p[cut:])
		if got := m.Sum(); hex.EncodeToString(got[:]) != hex.EncodeToString(ref.Sum(nil)) {
			t.Fatalf("HMAC-SHA1 after Reset")
		}
	}
}
