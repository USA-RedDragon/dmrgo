package capigen

import "testing"

func TestBoundPhrase(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		doc, want string
		ok        bool
	}{
		{"It allocates only when dst has no room for TrailerLen more octets.", "TrailerLen", true},
		{"It returns at most len(p) octets.", "len(p)", true},
		{"It writes the signature into dst, keeps the length of dst, and does not allocate.", "0", true},
		{"It does not allocate.", "", false},
	} {
		got, ok := boundPhrase(c.doc)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %q %v, want %q %v", c.doc, got, ok, c.want, c.ok)
		}
	}
}
