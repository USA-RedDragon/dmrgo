package rf

import (
	"math"
	"testing"
)

func TestAnchorZero(t *testing.T) {
	t.Parallel()
	m := newCPMModel(1, 5)
	syms := []int{1, -1, 3, -3, 1, 3, -1, -3}
	x := make([]complex64, len(syms)*m.sps)
	rot := m.anchor(x, syms)
	if math.IsNaN(float64(real(rot))) || math.IsNaN(float64(imag(rot))) {
		t.Fatalf("anchor of zero samples = %v", rot)
	}
}
