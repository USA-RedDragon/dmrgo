//go:build tinygo

package mmdvm_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkStream(t, s)
	}
}
