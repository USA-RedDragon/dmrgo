//go:build !tinygo

package rf

import "testing"

type unscaledRing struct{ toneRing }

func (unscaledRing) cohScale() float32 { return 0 }

// TestCohBurstClearsPreLLRs checks that a burst whose coherent detection
// fails leaves no pre-coherent LLRs for the embedded LC test of the next
// fragment, which would else take them from an earlier burst. It fails when
// they stay marked as fresh.
func TestCohBurstClearsPreLLRs(t *testing.T) {
	t.Parallel()
	opt := tierOptions(Tier2)
	x := newSoftExt(0, 10)
	s := newCohStageOf(&opt)
	s.src = &unscaledRing{}
	x.setCoh(s)
	x.preOK = true
	if x.cohBurst(&leanCore{}, &Burst{}, &x.ch[0], 0, false) {
		t.Fatal("coherent detection with no scale took the burst")
	}
	if x.preOK {
		t.Error("the pre-coherent LLRs of an earlier burst stay marked as fresh")
	}
}
