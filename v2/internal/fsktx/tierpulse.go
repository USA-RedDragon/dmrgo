package fsktx

import (
	"math"
	"math/cmplx"
	"slices"

	"github.com/USA-RedDragon/dmrgo/v2/internal/fmath"
)

func (c *AudioConfig) designZero() bool {
	p := &c.Pulse
	return !c.Exact && c.ExactPow == 0 && p.Span == 0 && p.Window == 0 && p.Beta == 0 && p.Comp == 0 && p.Gain == 0 &&
		p.Shape == nil && p.PerSymbol == 0 && p.EndTaper == 0 && !p.Flat && p.FlatGain == 0 && c.Shape == 0 &&
		c.NoiseZerosHz == [4]float64{} && c.Opt == 0 && c.Corr == 0 && c.CorrHiHz == 0 && c.CorrTaps == 0 && !c.Corr48k &&
		c.ShapeSync == 0 && c.ShapeMinLevel == 0 && c.ShapeLow == 0 && c.NoiseLowZerosHz == [4]float64{} &&
		c.NoiseLowPoleR == 0 && c.CarrierHz == 0 && c.CarrierMinLevel == 0 && c.Boost == 0 && c.BoostPairs == 0 && c.Rates == nil
}

const tierDesignHz = 24000

func (c *AudioConfig) tierDesign() {
	c.Exact, c.ExactPow = true, 0.5
	c.Pulse = PulseConfig{Span: 8, PerSymbol: 40, Gain: 1, Shape: tierShape()}
	c.Shape, c.NoiseZerosHz = 2, [4]float64{7000, 21000}
	c.Corr, c.CorrHiHz, c.Corr48k, c.ShapeSync = 6, 12000, true, 3
	c.BoostPairs = 48
}

func tierBoost(hz int) (float64, bool) {
	for _, r := range tierBoostTab {
		if int(r.hz) == hz {
			return r.boost, true
		}
	}
	return 0, false
}

func (c AudioConfig) toneBoost() (float64, bool) {
	const probe, level, from = 0.01, 15000, 120
	q := int(etsiHz) / gcd(c.Hz, int(etsiHz))
	periods := q * ((10 + q - 1) / q)
	n := periods * c.Hz / int(etsiHz)
	syms := 4*periods + 4
	var t [2][3]complex128
	per := float64(c.Hz) / etsiHz
	for i, b := range [2]float64{0, probe} {
		p := c
		p.Tier, p.Boost, p.Level, p.Offset, p.CarrierHz = Tier1, b, level, 0, 0
		p.Shape, p.NoiseZerosHz, p.PreEmphasisHz, p.Droop = 0, [4]float64{}, [2]float64{}, DroopConfig{}
		a, err := NewAudio(p)
		if err != nil {
			return 0, false
		}
		var out []int16
		for k := range from + syms {
			s := int8(3)
			if k%4 >= 2 {
				s = -3
			}
			out = a.AppendSymbol(out, s)
		}
		start := from * c.Hz / SymbolRate
		if start+n > len(out) {
			return 0, false
		}
		for k := range t[i] {
			for j, v := range out[start : start+n] {
				t[i][k] += complex(float64(v), 0) * cmplx.Rect(1, -2*math.Pi*float64((k+1)*j)/per)
			}
			t[i][k] *= complex(2*sinc(float64(k+1)*etsiHz/float64(c.Hz))/(float64(n)*a.scale*etsiDeviationHz), 0)
		}
	}
	peak := func(b float64) float64 {
		var top float64
		for j := range 720 {
			var y float64
			for k := range t[0] {
				pl := (t[1][k] - t[0][k]) / probe
				y += real(((1+complex(b, 0))*pl + t[0][k] - pl) * cmplx.Rect(1, 2*math.Pi*float64((k+1)*j)/720))
			}
			top = fmath.Max(top, math.Abs(y))
		}
		return top
	}
	lo, hi := -0.1, 0.1
	if peak(lo) > 1 || peak(hi) < 1 {
		return 0, false
	}
	for range 50 {
		if m := (lo + hi) / 2; peak(m) < 1 {
			lo = m
		} else {
			hi = m
		}
	}
	return (lo + hi) / 2, true
}

func (c *IQConfig) designZero() bool {
	p := &c.Pulse
	return !c.Exact && p.Span == 0 && p.Window == 0 && p.Beta == 0 && p.Comp == 0 && p.Gain == 0 && p.Shape == nil &&
		p.PerSymbol == 0 && p.EndTaper == 0 && !p.Flat && p.FlatGain == 0 && c.Shape == 0 && !c.ShapePred &&
		c.PredKern == 0 && c.PredGain == 0 && c.NoiseShape == 0 && c.NoiseZerosHz == [4]float64{} && c.NoisePoleR == 0 &&
		!c.Rotate
}

func (c *IQConfig) tierDesign() {
	c.Exact = true
	c.Pulse = PulseConfig{Span: predPulse.Span, Window: predPulse.Window, Beta: predPulse.Beta, Flat: predPulse.Flat, Gain: 1}
	c.ShapePred, c.PredKern, c.Rotate = true, 3, true
}

func tierShape() []float64 {
	return slices.Clone(tierShapeTab[:])
}
