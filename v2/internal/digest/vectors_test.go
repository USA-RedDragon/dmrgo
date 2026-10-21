package digest_test

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/digest"
)

func sha1Sum(p []byte) []byte {
	var d digest.SHA1
	d.Reset()
	d.Write(p)
	s := d.Sum()
	return s[:]
}

func sha256Sum(p []byte) []byte {
	var d digest.SHA256
	d.Reset()
	d.Write(p)
	s := d.Sum()
	return s[:]
}

type rspEntry struct {
	fields map[string]string
}

func readRSP(t *testing.T, name string) []rspEntry {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []rspEntry
	cur := rspEntry{fields: map[string]string{}}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<16), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] == '#' || line[0] == '[' {
			if len(cur.fields) > 0 {
				out = append(out, cur)
				cur = rspEntry{fields: map[string]string{}}
			}
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			k, v, ok = strings.Cut(line, ":")
		}
		if !ok {
			t.Fatalf("%s: line %q", name, line)
		}
		cur.fields[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if len(cur.fields) > 0 {
		out = append(out, cur)
	}
	return out
}

func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestCAVPMessages(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		file string
		sum  func([]byte) []byte
	}{
		{"SHA1ShortMsg.rsp", sha1Sum}, {"SHA1LongMsg.rsp", sha1Sum},
		{"SHA256ShortMsg.rsp", sha256Sum}, {"SHA256LongMsg.rsp", sha256Sum},
	} {
		n := 0
		for _, e := range readRSP(t, c.file) {
			bits, err := strconv.Atoi(e.fields["Len"])
			if err != nil {
				t.Fatalf("%s: %v", c.file, err)
			}
			msg := unhex(t, e.fields["Msg"])[:bits/8]
			if got := c.sum(msg); !bytes.Equal(got, unhex(t, e.fields["MD"])) {
				t.Errorf("%s Len %d: %x, want %s", c.file, bits, got, e.fields["MD"])
			}
			n++
		}
		if n < 8 {
			t.Fatalf("%s: %d entries", c.file, n)
		}
	}
}

func TestCAVPMonteCarlo(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		file string
		sum  func([]byte) []byte
	}{{"SHA1Monte.rsp", sha1Sum}, {"SHA256Monte.rsp", sha256Sum}} {
		entries := readRSP(t, c.file)
		seed := unhex(t, entries[0].fields["Seed"])
		for _, e := range entries[1:] {
			md := [3][]byte{seed, seed, seed}
			for range 1000 {
				msg := append(append(append([]byte(nil), md[0]...), md[1]...), md[2]...)
				md = [3][]byte{md[1], md[2], c.sum(msg)}
			}
			seed = md[2]
			if !bytes.Equal(seed, unhex(t, e.fields["MD"])) {
				t.Fatalf("%s COUNT %s: %x, want %s", c.file, e.fields["COUNT"], seed, e.fields["MD"])
			}
		}
		if len(entries) != 101 {
			t.Fatalf("%s: %d checkpoints", c.file, len(entries)-1)
		}
	}
}

func TestFIPS180Examples(t *testing.T) {
	t.Parallel()

	million := bytes.Repeat([]byte("a"), 1000000)
	two := []byte("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")
	for _, c := range []struct {
		msg        []byte
		sha1, s256 string
	}{
		{[]byte("abc"), "a9993e364706816aba3e25717850c26c9cd0d89d", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
		{two, "84983e441c3bd26ebaae4aa1f95129e5e54670f1", "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"},
		{million, "34aa973cd4c4daa4f61eeb2bdbad27316534016f", "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0"},
	} {
		if got := hex.EncodeToString(sha1Sum(c.msg)); got != c.sha1 {
			t.Errorf("SHA-1 of %d octets: %s", len(c.msg), got)
		}
		if got := hex.EncodeToString(sha256Sum(c.msg)); got != c.s256 {
			t.Errorf("SHA-256 of %d octets: %s", len(c.msg), got)
		}
	}
}

func TestRFC2202(t *testing.T) {
	t.Parallel()

	aa80 := bytes.Repeat([]byte{0xAA}, 80)
	for i, c := range []struct {
		key, data []byte
		want      string
	}{
		{bytes.Repeat([]byte{0x0B}, 20), []byte("Hi There"), "b617318655057264e28bc0b6fb378c8ef146be00"},
		{[]byte("Jefe"), []byte("what do ya want for nothing?"), "effcdf6ae5eb2fa2d27416d5f184df9c259a7c79"},
		{bytes.Repeat([]byte{0xAA}, 20), bytes.Repeat([]byte{0xDD}, 50), "125d7342b9ac11cd91a39af48aa17b4f63f175d3"},
		{[]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25}, bytes.Repeat([]byte{0xCD}, 50), "4c9007f4026250c6bc8414f9bf50c86c2d7235da"},
		{bytes.Repeat([]byte{0x0C}, 20), []byte("Test With Truncation"), "4c1a03424b55e07fe7f27be1d58bb9324a9a5a04"},
		{aa80, []byte("Test Using Larger Than Block-Size Key - Hash Key First"), "aa4ae5e15272d00e95705637ce8a3b55ed402112"},
		{aa80, []byte("Test Using Larger Than Block-Size Key and Larger Than One Block-Size Data"), "e8e99d0f45237d786d6bbaa7965c7808bbff1a91"},
	} {
		var m digest.HMACSHA1
		m.Init(c.key)
		m.Write(c.data)
		if got := m.Sum(); hex.EncodeToString(got[:]) != c.want {
			t.Errorf("test case %d: %x, want %s", i+1, got, c.want)
		}
	}
}

func TestBLAKE2bKAT(t *testing.T) {
	t.Parallel()

	entries := readRSP(t, "blake2b-kat.txt")
	if len(entries) != 256 {
		t.Fatalf("%d entries", len(entries))
	}
	var d digest.BLAKE2b
	for _, e := range entries {
		in, key, want := unhex(t, e.fields["in"]), unhex(t, e.fields["key"]), unhex(t, e.fields["hash"])
		if !d.Init(64, key) {
			t.Fatal("Init failed")
		}
		d.Write(in)
		var out [64]byte
		d.Sum(out[:])
		if !bytes.Equal(out[:], want) {
			t.Errorf("in of %d octets: %x, want %x", len(in), out, want)
		}
	}
}

func TestRFC7693(t *testing.T) {
	t.Parallel()

	var d digest.BLAKE2b
	d.Init(64, nil)
	d.Write([]byte("abc"))
	var out [64]byte
	d.Sum(out[:])
	want := "ba80a53f981c4d0d6a2797b69f12f6e94c212f14685ac4b74b12bb6fdbffa2d17d87c5392aab792dc252d5de4533cc9518d38aa8dbf1925ab92386edd4009923"
	if got := hex.EncodeToString(out[:]); got != want {
		t.Fatalf("%s", got)
	}
}
