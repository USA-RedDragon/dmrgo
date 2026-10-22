//go:build tinygo

package trunk_test

import "testing"

func TestFuzzCorpus(t *testing.T) {
	t.Parallel()
	pool := newPool(t)
	for _, s := range fuzzSeeds() {
		runFollower(t, pool, s)
	}
	capPool := newCapPlusPool(t)
	for _, s := range capPlusFuzzSeeds() {
		runCapPlusFollower(t, capPool, s)
	}
	msPool := newMSPool(t)
	for _, s := range msSeeds() {
		runMS(t, msPool, s)
	}
	inPool := newInboundPool(t)
	for _, in := range tsccSeeds() {
		runTSCC(t, inPool, in)
	}
}
