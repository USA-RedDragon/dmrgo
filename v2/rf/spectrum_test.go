package rf

import (
	"math"
	"testing"
)

// TestSpectrumWeakDirectMode feeds the spectrum estimator direct mode
// voice calls at 0.5 dB C/N, whose carrier is on in one slot of two and
// off between calls, and checks that it finds the carrier within 2 s, to
// the 200 Hz of the coherent search, for each noise seed. One estimate of
// specFrames frames found it at this C/N for none of them.
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
