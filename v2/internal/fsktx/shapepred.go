package fsktx

import "math"

// Sizes of the out-of-band phase predictor of IQConfig.ShapePred, in
// shaper samples at iqShaperHz: each symbol adds predF products of its
// symbol and the next three, each times a kernel of predK samples from
// predLo samples after the start of the symbol. With predLo of at least
// predSpan symbols the kernels of a sample are in before the sample, so
// the predictor adds no lag.
const (
	predF    = 15
	predLo   = 60
	predK    = 200
	predRing = 64
	predSpan = 3
	predSPS  = iqShaperHz / SymbolRate
)

// predPulse is the pulse, with IQConfig.Exact, that the kernels fit: the
// correction depends on the pulse, and the kernels of a sample start
// predLo samples after its symbol, past the half-length of the pulse only
// with a span of 16 symbols.
var predPulse = PulseConfig{Span: 16, Window: 3, Beta: 17, Flat: true}

// predFits reports whether the kernels fit the pulse c of the tier t.
func predFits(t Tier, c PulseConfig) bool {
	p := pulseOf(t, c)
	return p.span == predPulse.Span && c.Window == predPulse.Window && c.Beta == predPulse.Beta &&
		c.Flat == predPulse.Flat && c.Comp == 0 && (c.Gain == 0 || c.Gain == 1) && c.Shape == nil
}

// predFeatures returns the products of the symbol x[0] and the next three
// that weigh the kernels of x[0].
func predFeatures(x [predSpan + 1]float64) [predF]float64 {
	x0, x1, x2, x3 := x[0], x[1], x[2], x[3]
	x00 := x0 * x0
	return [predF]float64{
		x0, x00, x0 * x1, x0 * x2, x0 * x3,
		x00 * x0, x00 * x1, x00 * x2, x00 * x3,
		x0 * x1 * x1, x0 * x1 * x2, x0 * x1 * x3,
		x0 * x2 * x2, x0 * x2 * x3, x0 * x3 * x3,
	}
}

// predictor adds to the shaped deviation a phase correction that is a
// fixed polynomial function of the symbols, fitted to the corrections of
// the optimizer of IQConfig.Shape (shapefit_test.go). The kernels have
// nothing below 3 kHz, as the optimizer's correction.
type predictor struct {
	kern *[predK][predF]float64
	acc  [predK + predSPS]float64
	psi  [predRing]float64
	x    [predSpan + 1]float64
	nIn  int
	nSym int
	prev float64
}

// process takes the symbol s and its shaped deviation in buf and adds the
// correction to it.
func (p *predictor) process(buf []float64, s int8) {
	copy(p.x[:], p.x[1:])
	switch s {
	case -3, -1, 1, 3:
		p.x[predSpan] = float64(s) / 3
	default:
		p.x[predSpan] = 0
	}
	if p.nSym >= predSpan {
		ft := predFeatures(p.x)
		a := p.acc[:predK]
		for t := range a {
			k := &p.kern[t]
			a[t] += ((ft[0]*k[0] + ft[1]*k[1]) + (ft[2]*k[2] + ft[3]*k[3])) +
				((ft[4]*k[4] + ft[5]*k[5]) + (ft[6]*k[6] + ft[7]*k[7])) +
				((ft[8]*k[8] + ft[9]*k[9]) + (ft[10]*k[10] + ft[11]*k[11])) +
				((ft[12]*k[12] + ft[13]*k[13]) + ft[14]*k[14])
		}
		m := (p.nSym-predSpan)*predSPS + predLo
		for i, v := range p.acc[:predSPS] {
			p.psi[(m+i)&(predRing-1)] = v
		}
		copy(p.acc[:], p.acc[predSPS:])
		clear(p.acc[predK:])
	}
	p.nSym++
	for i := range buf {
		j := p.nIn & (predRing - 1)
		v := p.psi[j]
		p.psi[j] = 0
		p.nIn++
		buf[i] += (v - p.prev) * iqShaperHz / (2 * math.Pi)
		p.prev = v
	}
}
