package fsktx

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"
)

func TestAudioOptRates(t *testing.T) {
	t.Parallel()
	for _, hz := range []int{8000, 12000, 24000, 44100, 48000, 96000} {
		t.Run(fmt.Sprint(hz), func(t *testing.T) {
			t.Parallel()
			a, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier1, Opt: 2})
			if err != nil {
				t.Fatalf("%d: %v", hz, err)
			}
			if (a.opt != nil) != aoptRate(hz) {
				t.Fatalf("%d: optimizer %v", hz, a.opt != nil)
			}
			var out []int16
			for i := range 4800 {
				out = a.AppendSymbol(out, int8(i%4*2-3))
			}
			if len(out) != hz {
				t.Fatalf("%d: %d samples", hz, len(out))
			}
		})
	}
}

func TestAudioOptGradient(t *testing.T) {
	t.Parallel()
	o := newAudioOpt(1, 48000)
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range o.f {
		o.f[i] = 1944 * float64(rng.IntN(4)*2-3) / 3
		o.x[i] = rng.NormFloat64() * 20
	}
	x := make([]float64, o.win-o.ctx)
	for i := range x {
		x[i] = rng.NormFloat64() * 20
	}
	g := make([]float64, o.win-o.ctx)
	gn := make([]float64, o.win-o.ctx)
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

func TestAudioOptDelay(t *testing.T) {
	t.Parallel()
	for _, hz := range []int{20000, 22050, 24000, 32000, 44100, 48000, 96000} {
		o := newAudioOpt(1, hz)
		if d := float64(o.delay) * SymbolRate / float64(hz); d < 60 || d > 140 {
			t.Errorf("%d Hz: delay %.1f symbols", hz, d)
		}
	}
}

func TestAudioOptBounded(t *testing.T) {
	t.Parallel()
	for _, hz := range []int{24000, 48000} {
		mk := func() *Audio {
			a, err := NewAudio(AudioConfig{Hz: hz, Tier: Tier1, Opt: 2, Shape: 2, NoiseZerosHz: [4]float64{7000, 21000}})
			if err != nil {
				t.Fatal(err)
			}
			return a
		}
		rng := rand.New(rand.NewPCG(5, 6))
		sym := func() int8 { return int8(rng.IntN(4)*2 - 3) }
		a, b := mk(), mk()
		for range 1024 {
			a.AppendSymbol(nil, sym())
		}
		var oa, ob []int16
		for i := range 32 + 600 {
			s := int8(0)
			if i >= 32 {
				s = sym()
			}
			oa = a.AppendSymbol(oa, s)
			ob = b.AppendSymbol(ob, s)
		}
		from := 132*hz/SymbolRate + a.opt.delay
		for i := from; i < len(oa); i++ {
			if oa[i] != ob[i] {
				t.Fatalf("%d Hz: sample %d differs", hz, i)
			}
		}
	}
}
