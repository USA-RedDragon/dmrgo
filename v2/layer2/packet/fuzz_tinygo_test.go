//go:build tinygo

package packet_test

import "testing"

// TestTinyGoFuzzSeeds replays the seeds of the fuzz tests, which TinyGo
// does not run.
func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	s := newSession()
	for i, seed := range roundTripSeeds(t) {
		if !s.setup(t, seed.kind, seed.fields, seed.dst, seed.src, seed.payload) {
			t.Fatalf("seed %d does not segment", i)
		}
		s.run(seed.channel, seed.kind&0x20 != 0 && seed.kind&3 != 0)
		if s.fail != "" {
			t.Fatalf("round trip seed %d: %s", i, s.fail)
		}
	}
	r := newAnyBursts()
	for _, in := range burstSeeds(t) {
		r.feed(t, false, in)
		r.feed(t, true, in)
	}
	for _, seed := range responseDataSeeds() {
		checkResponseData(t, seed.b0, seed.b1, seed.blocks)
	}
}
