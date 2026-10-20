//go:build tinygo

package hamming

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range hamming743Seeds() {
		checkHamming743(t, s.data, s.mask)
	}
}
