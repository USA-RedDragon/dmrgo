//go:build tinygo

package data_test

import "testing"

func TestFuzzSeeds(t *testing.T) {
	t.Parallel()
	p := pool(t)
	for _, in := range rxSeeds() {
		checkRxScript(t, in, p)
	}
}
