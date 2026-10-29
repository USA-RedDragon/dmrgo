package fsktx

import "math"

// Peak returns the most that the output of an MCUAudio of t at level
// reaches from its offset, in codes, over every run of symbols.
func (t AudioTable) Peak(level int) int {
	lo, hi := t.bounds(int32((level<<15 + tableUnits/2) / tableUnits))
	return max(-lo, hi)
}

// Peak returns the most that the output of a Tier1 or Tier2 Audio of
// Offset 0 reaches from 0 over every run of symbols, in codes, and 0 for
// Tier0.
func (a *Audio) Peak() float64 {
	if a == nil || a.k == nil {
		return 0
	}
	lo, hi := a.bounds()
	return max(-lo, hi)
}

// PeakAt returns the Peak of an Audio of the config of a with Level
// level and Offset 0, and the error of NewAudio for that config, without
// a new design of its filters.
func (a *Audio) PeakAt(level int) (float64, error) {
	if a == nil || a.k == nil {
		return 0, errTier
	}
	if level < 1 || level > 16000 {
		return 0, errLevel
	}
	scale, offset := float64(level)/(3*DeviationHz), 0.0
	if level >= a.lv.carrierMin {
		offset += a.lv.carrierHz * scale
	}
	shape, fb, fbp, _ := a.lv.noiseShape(level)
	x := a.peak*scale + shapeSlack(shape, &fb, &fbp)
	lo, hi := offset-x, offset+x
	if hi > math.MaxInt16 || lo < math.MinInt16 {
		return 0, errOffset
	}
	return max(-lo, hi), nil
}

type levelDesign struct {
	shape, low, minLevel, hz int
	zeros, lowZeros          [4]float64
	lowPoleR                 float64
	carrierHz                float64
	carrierMin               int
}

func (c *AudioConfig) noiseShape(level int) (int, [8]float64, [8]float64, bool) {
	d := levelDesign{shape: c.Shape, low: c.ShapeLow, minLevel: c.ShapeMinLevel, zeros: c.NoiseZerosHz,
		lowZeros: c.NoiseLowZerosHz, lowPoleR: c.NoiseLowPoleR, hz: c.Hz}
	return d.noiseShape(level)
}

func (d *levelDesign) noiseShape(level int) (shape int, fb, fbp [8]float64, poles bool) {
	shape = min(max(d.shape, 0), 4)
	fb = ntf(shape, d.zeros, float64(d.hz))
	if level < d.minLevel {
		shape = min(max(d.low, 0), 4)
		fb = ntf(shape, d.lowZeros, float64(d.hz))
		if r := d.lowPoleR; r > 0 && r < 1 {
			poles = true
			g := r
			for i := range fbp {
				fbp[i] = fb[i] * g
				g *= r
			}
		}
	}
	return shape, fb, fbp, poles
}

func shapeSlack(shape int, fb, fbp *[8]float64) float64 {
	if shape <= 0 {
		return 0
	}
	s := 0.5
	for _, f := range fb[:2*shape] {
		s += 0.5 * math.Abs(f)
	}
	for _, f := range fbp[:2*shape] {
		s += 0.5 * math.Abs(f)
	}
	return s
}
