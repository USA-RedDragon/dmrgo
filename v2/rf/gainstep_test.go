package rf

import (
	"math"
	"math/rand/v2"
	"testing"
)

func gainStepRun(x []complex64) []complex64 {
	var g gainStep
	g.init()
	out := make([]complex64, 0, len(x))
	for _, v := range x {
		if y, ok := g.push(v); ok {
			out = append(out, y)
		}
	}
	g.drain(func(v complex64) { out = append(out, v) })
	return out
}

// TestGainStep checks that gainStep turns a 10 dB step of a tone into a
// slope, ends at the new level, and passes noise and the 1.25 ms ramp of a burst unchanged.
func TestGainStep(t *testing.T) {
	t.Parallel()
	if d := math.Exp(-1 / (gainStepSeconds * basebandRate)); math.Abs(d-gainStepDecay) > 1e-8 {
		t.Fatalf("gainStepDecay %v, want %v", gainStepDecay, d)
	}
	const n, at = 4800, 1000
	tone := func(i int) complex64 {
		s, c := math.Sincos(2 * math.Pi * 12500 * float64(i) / basebandRate)
		return complex64(complex(c, s))
	}
	x := make([]complex64, n)
	for i := range x {
		g := float32(1)
		if i >= at {
			g = float32(math.Pow(10, -0.5))
		}
		x[i] = tone(i) * complex(g, 0)
	}
	y := gainStepRun(x)
	if len(y) != n {
		t.Fatalf("%d samples out of %d", len(y), n)
	}
	for i := 1; i < n; i++ {
		a, b := cabs(y[i-1]), cabs(y[i])
		if b < a*0.99 || b > a*1.001 {
			t.Fatalf("sample %d: level %.4f after %.4f", i, b, a)
		}
	}
	if e := cabs(y[n-1]) / cabs(x[n-1]); math.Abs(float64(e)-1) > 1e-3 {
		t.Errorf("level at the end %.4f of the input", e)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range x {
		x[i] = complex(float32(rng.NormFloat64()), float32(rng.NormFloat64()))
	}
	for i := 0; i < 60; i++ {
		e := 0.5 - 0.5*math.Cos(math.Pi*float64(i)/60)
		x[2000+i] = tone(i) * complex(float32(e), 0)
	}
	for i := 2060; i < n; i++ {
		x[i] = tone(i)
	}
	y = gainStepRun(x)
	for i := range x {
		if y[i] != x[i] {
			t.Fatalf("sample %d of noise and a ramp changed: %v to %v", i, x[i], y[i])
		}
	}
}

func cabs(v complex64) float32 { return float32(math.Hypot(float64(real(v)), float64(imag(v)))) }
