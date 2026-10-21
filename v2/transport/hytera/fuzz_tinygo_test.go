//go:build tinygo

package hytera_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fuzzSeeds() {
		checkPacket(t, s)
	}
}
