//go:build tinygo

package quadraticresidue

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range qr1676Seeds() {
		checkQR1676(t, s.data, s.mask)
	}
}
