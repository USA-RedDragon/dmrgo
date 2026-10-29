package fsktx

import "slices"

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
	c.Pulse = PulseConfig{Span: 8, PerSymbol: 40, Gain: 1.0004, Shape: tierShape()}
	c.Shape, c.NoiseZerosHz = 2, [4]float64{7000, 21000}
	c.Corr, c.CorrHiHz, c.Corr48k, c.ShapeSync = 6, 12000, true, 3
	c.Boost, c.BoostPairs = 0.009, 48
}

func (c *IQConfig) designZero() bool {
	p := &c.Pulse
	return !c.Exact && p.Span == 0 && p.Window == 0 && p.Beta == 0 && p.Comp == 0 && p.Gain == 0 && p.Shape == nil &&
		p.PerSymbol == 0 && p.EndTaper == 0 && !p.Flat && p.FlatGain == 0 && c.Shape == 0 && !c.ShapePred &&
		c.PredKern == 0 && c.PredGain == 0 && c.NoiseShape == 0 && c.NoiseZerosHz == [4]float64{} && c.NoisePoleR == 0 &&
		!c.Rotate
}

func (c *IQConfig) tierDesign() {
	g := 1.0095
	if c.Tier == Tier2 {
		g = 1.0058
	}
	c.Exact = true
	c.Pulse = PulseConfig{Span: predPulse.Span, Window: predPulse.Window, Beta: predPulse.Beta, Flat: predPulse.Flat, Gain: g}
	c.ShapePred, c.PredKern, c.Rotate = true, 3, true
}

func tierShape() []float64 {
	return slices.Clone(tierShapeTab[:])
}
