//go:build tinygo

package crc_test

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range crcSeeds() {
		checkCRC(t, s.data, s.nbits, s.flip, s.wrong)
	}
}
