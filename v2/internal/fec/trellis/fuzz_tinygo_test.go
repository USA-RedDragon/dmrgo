//go:build tinygo

package trellis

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range trellisSeeds(t) {
		checkTrellis(t, s.data, s.mask)
	}
}
