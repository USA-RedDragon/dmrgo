//go:build !tinygo

package rf

import "testing"

// TestMatrixScoresTheCorrectedBurst checks that the matrix scores what the
// caller gets: a burst whose raw bits FEC repaired passes, and one that FEC
// could not repair, or that carries no corrected copy, fails.
func TestMatrixScoresTheCorrectedBurst(t *testing.T) {
	t.Parallel()
	var tr mxTruth
	for i := range tr.data {
		tr.data[i] = byte(17 * i)
	}
	tr.cach = [3]byte{1, 2, 3}
	tr.hasCACH, tr.bs = true, true
	wrong := tr.data
	wrong[3] ^= 0x41
	cach := tr.cach
	cach[1] ^= 0x10
	for _, c := range []struct {
		name string
		b    Burst
		fail bool
	}{
		{"repaired", Burst{Data: wrong, CACH: cach, HasCACH: true, BS: true, Fixed: tr.data, FixedCACH: tr.cach, HasFixed: true}, false},
		{"unrepaired", Burst{Data: wrong, CACH: tr.cach, HasCACH: true, BS: true, Fixed: wrong, FixedCACH: tr.cach, HasFixed: true}, true},
		{"no corrected copy", Burst{Data: wrong, CACH: tr.cach, HasCACH: true, BS: true}, true},
		{"clean", Burst{Data: tr.data, CACH: tr.cach, HasCACH: true, BS: true}, false},
	} {
		s := &mxSignal{truth: []mxTruth{tr}}
		sc := &mxScorer{s: s, first: map[int]int{}, offs: map[[2]int]int64{}, streams: map[int]map[int]bool{}, found: map[string][]string{}}
		sc.burst(&c.b, 0)
		if got := len(sc.found["bits"]) > 0 || len(sc.found["cach"]) > 0; got != c.fail {
			t.Errorf("%s: fails %v, want %v: %v", c.name, got, c.fail, sc.found)
		}
	}
}
