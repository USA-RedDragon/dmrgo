package fsktx

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestAudioOptGradient(t *testing.T) {
	o := newAudioOpt(1, 48000)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range o.f {
		o.f[i] = 1944 * float64(rng.IntN(4)*2-3) / 3
		o.x[i] = rng.NormFloat64() * 20
	}
	x := make([]float64, aoptFree)
	for i := range x {
		x[i] = rng.NormFloat64() * 20
	}
	g := make([]float64, aoptFree)
	gn := make([]float64, aoptFree)
	o.jscale = 1
	o.eval(1000, x, g)
	var gmax float64
	for _, v := range g {
		gmax = max(gmax, math.Abs(v))
	}
	for _, i := range []int{0, 100, 400, 767} {
		h := 1e-3
		x[i] += h
		jp := o.eval(1000, x, gn)
		x[i] -= 2 * h
		jm := o.eval(1000, x, gn)
		x[i] += h
		num := (jp - jm) / (2 * h)
		if math.Abs(num-g[i]) > 1e-4*gmax {
			t.Errorf("sample %d: gradient %g, numeric %g", i, g[i], num)
		}
	}
}
