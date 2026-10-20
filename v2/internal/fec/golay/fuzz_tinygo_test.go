//go:build tinygo

package golay

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range golaySeeds() {
		checkGolay(t, s.data, s.mask)
	}
}
