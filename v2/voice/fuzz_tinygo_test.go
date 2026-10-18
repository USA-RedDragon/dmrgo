//go:build tinygo

package voice_test

import "testing"

// TestFuzzCorpus runs the seeds and the testdata/fuzz inputs of the fuzz
// tests of this package, because TinyGo does not run fuzz tests.
func TestFuzzCorpus(t *testing.T) {
	t.Parallel()
	p := newPool(t)
	for _, in := range append(seeds(p), corpusFiles(t, "FuzzReceiver")...) {
		runScript(t, p, in)
	}
	for _, in := range append(roundTripSeeds(), corpusFiles(t, "FuzzTransmitterRoundTrip")...) {
		runRoundTrip(t, in)
	}
}
