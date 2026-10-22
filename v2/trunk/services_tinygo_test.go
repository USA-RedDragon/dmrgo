//go:build tinygo

package trunk_test

import "testing"

func TestServicesFuzzCorpus(t *testing.T) {
	t.Parallel()
	msPool := newMSSvcPool(t)
	for _, s := range msSvcSeeds() {
		runMSServices(t, msPool, s)
	}
	inPool := newTSCCSvcPool(t)
	for _, in := range tsccSvcSeeds() {
		runTSCCServices(t, inPool, in)
	}
}
