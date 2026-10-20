//go:build tinygo

package framer_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range searchSeeds(t) {
		checkSearch(t, s.stream, s.nbits, s.start, s.threshold)
	}
	for _, s := range framerSeeds(t) {
		checkFramer(t, s.stream, s.cuts, s.cfg)
	}
	fb := newFindsBurst(t)
	for _, s := range findsBurstSeeds {
		fb.check(t, s.noise, s.offset, s.which, s.flips, s.cfg)
	}
}
