//go:build tinygo

package trunk_test

import "testing"

func TestServicesFuzzCorpus(t *testing.T) {
	t.Parallel()
	msPool := newMSSvcPool(t)
	for _, in := range msSvcSeeds() {
		runMSServices(t, msPool, in)
	}
	inPool := newTSCCSvcPool(t)
	for _, in := range tsccSvcSeeds() {
		runTSCCServices(t, inPool, in)
	}
}
