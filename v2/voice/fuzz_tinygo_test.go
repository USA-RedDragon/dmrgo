//go:build tinygo

package voice_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fuzztest"
)

func roundTripCorpus(t *testing.T) []rtInput {
	t.Helper()
	var out []rtInput
	var in rtInput
	fuzztest.Corpus(t, "FuzzTransmitterRoundTrip", func() { out = append(out, in) },
		&in.flags, &in.cc, &in.headers, &in.terms, &in.so, &in.target, &in.source, &in.n, &in.alias, &in.ramp, &in.text, &in.frames, &in.drop)
	return out
}

func TestFuzzCorpus(t *testing.T) {
	t.Parallel()
	p := newPool(t)
	for _, in := range append(seeds(p), rxCorpus(t)...) {
		runScript(t, p, &in)
	}
	for _, in := range append(roundTripSeeds(), roundTripCorpus(t)...) {
		runRoundTrip(t, &in)
	}
}
