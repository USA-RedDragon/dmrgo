//go:build tinygo

package reedsolomon

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range reedSolomonSeeds() {
		checkReedSolomon(t, s.data, s.mask)
	}
}
