//go:build !tinygo

package rf

import (
	"math"
	"testing"
)

func TestRxClockStepSettles(t *testing.T) {
	t.Parallel()
	row := mxRowOf(t, "agc.none.-2500.ms.pi.-500ppm.high.cu8-2048k.40dB")
	s, err := mxBuild(row)
	if err != nil {
		t.Fatal(err)
	}
	in := mxInputOf(s, row)
	got, _, err := mxReceive(in, mxRx{row.val(mxFormat), Tier1})
	if err != nil {
		t.Fatal(err)
	}
	worst := 0.0
	for _, b := range got {
		at := b.Seconds + in.delay
		if j := mxNearest(s, at); j >= 0 {
			worst = math.Max(worst, math.Abs(at-s.truth[j].at))
		}
	}
	if worst > 21e-6 {
		t.Errorf("worst Seconds error %.1f us at -500 ppm, want at most 21 us", worst*1e6)
	}
}
