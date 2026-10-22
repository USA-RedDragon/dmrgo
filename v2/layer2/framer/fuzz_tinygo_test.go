//go:build tinygo

package framer_test

import "testing"

func TestTinyGoFuzzSeeds(t *testing.T) {
	t.Parallel()

	for _, s := range searchSeeds(t) {
		checkSearch(t, s.stream, s.nbits, s.start, s.threshold)
	}
	pool := loadVoice(t)
	for _, s := range framerSeeds(pool) {
		checkFramer(t, pool, s.script, nil, s.cuts, s.cfg)
	}
	fb := newFindsBurst(t)
	for _, s := range findsBurstSeeds {
		fb.check(t, s.noise, s.offset, s.which, s.flips, s.cfg)
	}
}
