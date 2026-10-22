//go:build tinygo

package trunk_test

import "testing"

func TestUDTFuzzCorpus(t *testing.T) {
	t.Parallel()
	for _, in := range appendedSeeds() {
		runAppended(t, in)
	}
	pool := newUDTPool(t)
	for _, s := range msUDTSeeds() {
		runMSUDT(t, pool, s)
	}
	tpool := newTSCCUDTPool(t)
	for _, in := range tsccUDTSeeds() {
		runTSCCUDT(t, tpool, in)
	}
}
