//go:build tinygo

package trunk_test

import "testing"

func TestFuzzCorpus(t *testing.T) {
	t.Parallel()
	pool := newPool(t)
	for _, in := range fuzzSeeds() {
		runFollower(t, pool, in)
	}
	msPool := newMSPool(t)
	for _, in := range msSeeds() {
		runMS(t, msPool, in)
	}
	inPool := newInboundPool(t)
	for _, in := range tsccSeeds() {
		runTSCC(t, inPool, in)
	}
}
