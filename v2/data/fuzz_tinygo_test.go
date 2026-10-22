//go:build tinygo

package data_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()
	p := pool(t)
	for _, in := range rxSeeds() {
		checkRxScript(t, in, p)
	}
	tp, msgs := txPool(t), txMessages(t)
	for _, in := range txSeeds() {
		checkTxScript(t, &in, tp, msgs)
	}
}
