//go:build tinygo

package bits

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range fieldSeeds() {
		checkField(t, s.buf, s.start, s.width, s.v, s.dst)
	}
}
