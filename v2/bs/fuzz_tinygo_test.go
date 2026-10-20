//go:build tinygo

package bs_test

import "testing"

func TestFuzzCorpus(t *testing.T) {
	t.Parallel()
	p := newPool(t)
	for _, in := range append(seeds(), corpusFiles(t, "FuzzRepeater")...) {
		runScript(t, p, in)
	}
}
