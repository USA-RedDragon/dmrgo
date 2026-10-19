package rf

import (
	"math"
	"testing"
)

// TestAnchorZero checks that the anchor rotation of samples that are all
// zero, as outside the ring, is finite: a NaN rotation made logAdd of the
// LogMAP trellis index with int(NaN) and panic.
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
