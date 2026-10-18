//go:build tinygo

package reedsolomon

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range reedSolomonSeeds() {
		checkReedSolomon(t, s.data, s.mask)
	}
}
