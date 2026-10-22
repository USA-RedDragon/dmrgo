//go:build !tinygo

package csbk_test

import "testing"

func TestAllocsSeeds(t *testing.T) {
	g := newRig(t)
	for i, in := range fuzzSeeds() {
		if n := testing.AllocsPerRun(20, func() { runScript(t, g, in, 0, 0, 0) }); n != 0 {
			t.Errorf("seed %d: %v allocations per run", i, n)
		}
	}
}
