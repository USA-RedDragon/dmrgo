//go:build tinygo

package golay

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range golaySeeds() {
		checkGolay(t, s.data, s.mask)
	}
}
