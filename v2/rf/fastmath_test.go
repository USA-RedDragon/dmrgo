package rf

import (
	"math"
	"testing"
)

func TestLog32(t *testing.T) {
	t.Parallel()
	for x := 1e-6; x < 1e9; x *= 1.37 {
		got, want := float64(log32(float32(x))), math.Log(float64(float32(x)))
		if math.Abs(got-want) > 2e-5*max(1, math.Abs(want)) {
			t.Fatalf("log32(%g) = %g, want %g", x, got, want)
		}
	}
}
