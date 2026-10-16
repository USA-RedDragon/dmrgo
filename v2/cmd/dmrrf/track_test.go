package main

import (
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// TestLLR8Symmetric checks that the 8-bit store of a burst rounds a ratio
// and its negation to negated values and keeps the sign of every nonzero
// ratio, so that it leans to neither bit value.
func TestLLR8Symmetric(t *testing.T) {
	t.Parallel()
	var v [layer2.BurstBits]int16
	for i := range v {
		v[i] = int16(i - layer2.BurstBits/2)
	}
	var l llr8
	l.set(&v)
	for i, x := range v {
		j := layer2.BurstBits - i
		if j < len(v) && v[j] == -x && l[j] != -l[i] {
			t.Errorf("ratio %d stores as %d, ratio %d as %d", x, l[i], -x, l[j])
		}
		if x != 0 && (l[i] > 0) != (x > 0) {
			t.Errorf("ratio %d stores as %d", x, l[i])
		}
	}
	v[0], v[1] = 32000, -32000
	l.set(&v)
	if l[0] != -l[1] {
		t.Errorf("saturated ratios store as %d and %d", l[0], l[1])
	}
}
