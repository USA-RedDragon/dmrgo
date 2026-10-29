//go:build !tinygo

package csbk_test

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/allocstest"
)

func TestAllocsSeeds(t *testing.T) {
	g := newRig(t)
	for i, in := range fuzzSeeds() {
		if n := allocstest.PerRun(t, 20, func() { runScript(t, g, in, 0, 0, 0) }); n != 0 {
			t.Errorf("seed %d: %v allocations per run", i, n)
		}
	}
}
