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

// TestIQShapeS8 checks that the noise shaping of AppendS8 keeps the 8-bit
// I/Q on the rounded 16-bit I/Q on average.
func TestIQShapeS8(t *testing.T) {
	q, err := NewIQ(IQConfig{Rate: 96000, Tier: Tier2, Exact: true, NoiseShape: 2, NoiseZerosHz: [4]float64{12500, 25000}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewIQ(IQConfig{Rate: 96000, Tier: Tier2, Exact: true})
	if err != nil {
		t.Fatal(err)
	}
	q.Key(true)
	r.Key(true)
	var a []byte
	var b []int16
	for i := range 2000 {
		s := int8(2*(i*7%4) - 3)
		a = q.AppendS8(a, s)
		b = r.AppendS16(b, s)
	}
	var sum float64
	for i := range a {
		sum += float64(int8(a[i])) - float64(b[i])/256
	}
	if m := sum / float64(len(a)); math.Abs(m) > 0.05 {
		t.Errorf("mean error %.3f steps", m)
	}
}
