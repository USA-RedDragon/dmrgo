package rf

import (
	"math"
	"testing"
)

func TestSpectrumWeakDirectMode(t *testing.T) {
	t.Parallel()
	fm, env, _ := directCalls(t, 2)
	for seed := range uint64(6) {
		x := noisyFM(fm, env, 1400, 0.5, 70+seed)
		s := newSpectrum(basebandRate)
		at := -1
		for i, v := range x[:min(len(x), 2*basebandRate)] {
			if s.push(v) && s.OK && math.Abs(s.Hz-1400) < 200 {
				at = i
				break
			}
		}
		if at < 0 {
			t.Errorf("seed %d: no estimate found the carrier in 2 s", seed)
		}
	}
}
