package fmath

import (
	"math"
	"testing"
)

func TestMatchesBuiltin(t *testing.T) {
	t.Parallel()

	vals := []float64{math.Inf(-1), -2, -1, math.Copysign(0, -1), 0, 1, 2, math.Inf(1), math.NaN()}
	for _, x := range vals {
		for _, y := range vals {
			if got, want := Max(x, y), max(x, y); math.Float64bits(got) != math.Float64bits(want) && !(math.IsNaN(got) && math.IsNaN(want)) {
				t.Errorf("Max(%v, %v) = %v, want %v", x, y, got, want)
			}
			if got, want := Min(x, y), min(x, y); math.Float64bits(got) != math.Float64bits(want) && !(math.IsNaN(got) && math.IsNaN(want)) {
				t.Errorf("Min(%v, %v) = %v, want %v", x, y, got, want)
			}
		}
	}
}
