package rf

import (
	"math"
	"math/rand/v2"
	"testing"
)

func rcPulse(t float64) float64 {
	if math.Abs(math.Abs(2*rrcAlpha*t)-1) < 1e-9 {
		return math.Pi / 4 * math.Sin(math.Pi/(2*rrcAlpha)) / (math.Pi / (2 * rrcAlpha))
	}
	s := 1.0
	if t != 0 {
		s = math.Sin(math.Pi*t) / (math.Pi * t)
	}
	return s * math.Cos(math.Pi*rrcAlpha*t) / (1 - 4*rrcAlpha*rrcAlpha*t*t)
}

func TestLagrange6Strobes(t *testing.T) {
	t.Parallel()
	for k := range 6 {
		p := func(x float32) float32 { return float32(math.Pow(float64(x), float64(k))) }
		for _, mu := range []float32{-1, -0.75, -0.5, -0.25, 0, 0.25, 0.5, 0.75} {
			got := lagrange6[site0](p(-3), p(-2), p(-1), p(0), p(1), p(2), mu)
			if d := math.Abs(float64(got - p(mu))); d > 1e-3*math.Max(1, float64(p(mu))) {
				t.Errorf("x^%d at %v: %v, want %v", k, mu, got, p(mu))
			}
		}
	}
	rng := rand.New(rand.NewPCG(1, 2))
	levels := []float64{-3, -1, 1, 3}
	syms := make([]float64, 400)
	for i := range syms {
		syms[i] = levels[rng.IntN(4)]
	}
	copy(syms[200:], []float64{3, -3, 3, -3, 3, -1, 1, -1, 3, 3})
	y := func(t float64) float64 {
		var v float64
		for k, a := range syms {
			if d := t - float64(k); math.Abs(d) < 12 {
				v += a * rcPulse(d)
			}
		}
		return v
	}
	var lag, cr float64
	n := 0
	for k := 20; k < len(syms)-20; k++ {
		s := func(j int) float32 { return float32(y(float64(k) - 0.25 + 0.5*float64(j))) }
		want := syms[k]
		l := float64(lagrange6[site0](s(-3), s(-2), s(-1), s(0), s(1), s(2), 0.5))
		p0, p1, p2, p3 := s(-1), s(0), s(1), s(2)
		mu := float32(0.5)
		c := float64(p1 + 0.5*mu*(p2-p0+mu*(2*p0-5*p1+4*p2-p3+mu*(3*(p1-p2)+p3-p0))))
		lag += (l - want) * (l - want)
		cr += (c - want) * (c - want)
		n++
	}
	lag, cr = math.Sqrt(lag/float64(n)), math.Sqrt(cr/float64(n))
	t.Logf("RMS error halfway: lagrange6 %.3f, Catmull-Rom %.3f", lag, cr)
	if lag > 0.04 {
		t.Errorf("lagrange6 misses the symbols by %.3f RMS, want at most 0.04", lag)
	}
}
