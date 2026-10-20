//go:build !tinygo

package packet_test

import "testing"

func TestAllocsRoundTrip(t *testing.T) {
	s := newSession()
	for i, seed := range roundTripSeeds(t) {
		if !s.setup(t, seed.kind, seed.fields, seed.dst, seed.src, seed.payload) {
			t.Fatalf("seed %d does not segment", i)
		}
		accept := seed.kind&0x20 != 0 && seed.kind&3 != 0
		if n := testing.AllocsPerRun(5, func() { s.run(seed.channel, accept) }); n != 0 {
			t.Errorf("seed %d: %.1f allocations per session, want 0", i, n)
		}
		if s.fail != "" {
			t.Fatalf("seed %d: %s", i, s.fail)
		}
	}
}
