package rf

import "testing"

// TestNPMLAveragesFirstSyncs checks that the noise correlation of the first
// syncs of a stream is their mean, so one stray first fit does not hold for
// the next ten syncs. It fails when the estimate after five fits is more than
// 0.01 from their mean.
func TestNPMLAveragesFirstSyncs(t *testing.T) {
	t.Parallel()
	fits := []float32{0.2, -0.6, -0.55, -0.6, -0.5}
	var q npml
	var mean float32
	for _, r := range fits {
		q.add(r)
		mean += r / float32(len(fits))
	}
	if d := q.rho - mean; d > 0.01 || d < -0.01 {
		t.Errorf("rho %.3f after %v, want their mean %.3f", q.rho, fits, mean)
	}
}
