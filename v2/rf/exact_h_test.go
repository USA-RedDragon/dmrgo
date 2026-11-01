package rf

import "testing"

func TestXTrellisStep(t *testing.T) {
	t.Parallel()
	m := newCPMModel(1, 10)
	for _, c := range []struct {
		np, def int
	}{{phaseStates, 27}, {52, 7}} {
		x := newXTrellis(m, c.np, 8)
		if x.step != c.def {
			t.Errorf("np %d: default step %d, want %d", c.np, x.step, c.def)
		}
		if got, want := x.off[3], 3*c.def%c.np; got != want {
			t.Errorf("np %d: off[3] %d, want %d", c.np, got, want)
		}
	}
}

func TestXTrellisJumpCosts(t *testing.T) {
	t.Parallel()
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
	x.below = 0.75
	copy(x.jk, []float32{2, 2, 1, 3})
	x.jumpCosts(4, 8)
	want = []float32{0, 2, 2, 0}
	for k, w := range want {
		if x.jk[k] != w {
			t.Errorf("below: jk[%d] = %v, want %v", k, x.jk[k], w)
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
