//go:build tinygo

package ipsc_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkPacket(t, s)
	}
}
