package rf

import "testing"

// TestXTrellisSetH checks that setH turns the phase of each symbol by the
// nearest step to the modulation index, and that the default step is that
// of 0.27.
func TestXTrellisSetH(t *testing.T) {
	m := newCPMModel(1, 10)
	for _, c := range []struct {
		np, def int
		h       float64
		step    int
	}{{phaseStates, 27, 0.243, 24}, {phaseStates, 27, 0.297, 30}, {52, 7, 0.243, 6}, {52, 7, 0.27, 7}} {
		x := newXTrellis(m, c.np, 8)
		if x.step != c.def {
			t.Errorf("np %d: default step %d, want %d", c.np, x.step, c.def)
		}
		x.setH(c.h)
		if x.step != c.step {
			t.Errorf("np %d h %v: step %d, want %d", c.np, c.h, x.step, c.step)
		}
		if got, want := x.off[3], 3*c.step%c.np; got != want {
			t.Errorf("np %d h %v: off[3] %d, want %d", c.np, c.h, got, want)
		}
	}
}

// TestXTrellisJumpCosts checks that the fade jump cost falls with the
// square of the lesser correlation of two symbols over the mean.
func TestXTrellisJumpCosts(t *testing.T) {
	x := newXTrellis(newCPMModel(1, 10), 52, 4)
	x.jump, x.fade = 8, true
	copy(x.jk, []float32{2, 2, 1, 3})
	x.jumpCosts(4, 8)
	want := []float32{8, 2, 2, 18}
	for k, w := range want {
		if x.jk[k] != w {
			t.Errorf("jk[%d] = %v, want %v", k, x.jk[k], w)
		}
	}
	x.fade = false
	x.jumpCosts(4, 8)
	for k := range 4 {
		if x.jk[k] != 8 {
			t.Errorf("no fade: jk[%d] = %v, want 8", k, x.jk[k])
		}
	}
}
