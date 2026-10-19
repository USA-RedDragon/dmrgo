package fsktx

import (
	"math"
	"testing"
)

// TestFlatETSI checks that Flat gives the exact pulse the same gain at the
// tone of the deviation test pattern as at 0 Hz.
func TestFlatETSI(t *testing.T) {
	for _, c := range []PulseConfig{{Window: 3, Beta: 5}, {Window: 3, Beta: 17}, {Span: 8, Window: 3, Beta: 8}} {
		pu := pulseOf(Tier2, c)
		pu.nyquist()
		off := pu.baseGain(etsiHz / float64(SymbolRate))
		c.Flat = true
		pu = pulseOf(Tier2, c)
		pu.nyquist()
		if g := pu.baseGain(etsiHz / float64(SymbolRate)); math.Abs(g-1) > 1e-8 {
			t.Errorf("%+v: gain %.10f at %g Hz, %.10f without Flat", c, g, etsiHz, off)
		}
	}
}
