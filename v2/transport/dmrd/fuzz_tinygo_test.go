//go:build tinygo

package dmrd_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkPacket(t, s)
	}
	freeDMRSeeds(func(b []byte) { fuzzFreeDMROptions(t, b) })
}
