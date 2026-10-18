//go:build tinygo

package hamming

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range hamming743Seeds() {
		checkHamming743(t, s.data, s.mask)
	}
}
