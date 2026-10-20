//go:build tinygo

package voice_test

import "testing"

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
