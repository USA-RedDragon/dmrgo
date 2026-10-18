//go:build tinygo

package bptc_test

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range bptc196Seeds(t) {
		checkBPTC196(t, s.data, s.reserved, s.mask)
	}
	for _, s := range embeddedLCSeeds(t) {
		checkEmbeddedLC(t, s.data, s.mask, s.parity)
	}
	for _, s := range cachSeeds(t) {
		checkCACH(t, s.info, s.mask)
	}
	for _, s := range singleBurstSeeds(t) {
		checkSingleBurst(t, s.info, s.odd, s.mask)
	}
}
