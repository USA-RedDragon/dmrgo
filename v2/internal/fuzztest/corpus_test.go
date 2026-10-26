package fuzztest_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
)

func TestCorpus(t *testing.T) {
	t.Parallel()
	var c, b, u uint8
	var w uint32
	var ok bool
	var raw []byte
	n := 0
	fuzztest.Corpus(t, "FuzzValues", func() { n++ }, &c, &b, &u, &w, &ok, &raw)
	if n != 1 || c != 'x' || b != 5 || u != 7 || w != 9 || !ok || string(raw) != "ab" {
		t.Fatalf("Corpus: %d files, %d %d %d %d %v %q", n, c, b, u, w, ok, raw)
	}
}
