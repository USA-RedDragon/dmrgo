package fsktx

import "math"

const (
	predF     = 15
	predLo    = 60
	predK     = 200
	predRing  = 64
	predSpan  = 3
	predSPS   = iqShaperHz / SymbolRate
	predC     = 6
	predMaskC = 1<<0 | 1<<5 | 1<<9 | 1<<10 | 1<<12 | 1<<13
)

var predPulse = PulseConfig{Span: 16, Window: 3, Beta: 17, Flat: true}

func predFits(t Tier, c PulseConfig) bool {
	p := pulseOf(t, c)
	return p.span == predPulse.Span && c.Window == predPulse.Window && c.Beta == predPulse.Beta &&
		c.Flat == predPulse.Flat && c.FlatGain == 0 && c.Comp == 0 && (c.Gain == 0 || c.Gain == 1) && c.Shape == nil
}

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

func predFeaturesC(x [predSpan + 1]float64) [predC]float64 {
	x0, x1, x2, x3 := x[0], x[1], x[2], x[3]
	return [predC]float64{x0, x0 * x0 * x0, x0 * x1 * x1, x0 * x1 * x2, x0 * x2 * x2, x0 * x2 * x3}
}

type predictor struct {
	kern *[predK][predF]float64
	kc   *[predK][predC]float64
	acc  [predK + predSPS]float64
	psi  [predRing]float64
	x    [predSpan + 1]float64
	gain float64
	nIn  int
	nSym int
	prev float64
}

func (p *predictor) process(buf []float64, s int8) {
	copy(p.x[:], p.x[1:])
	switch s {
	case -3, -1, 1, 3:
		p.x[predSpan] = float64(s) / 3
	default:
		p.x[predSpan] = 0
	}
	if p.nSym >= predSpan {
		a := p.acc[:predK]
		if p.kc != nil {
			p.addC(a)
		} else {
			p.addAll(a)
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
		buf[i] += p.gain * (v - p.prev) * iqShaperHz / (2 * math.Pi)
		p.prev = v
	}
}

func (p *predictor) addAll(a []float64) {
	ft := predFeatures(p.x)
	for t := range a {
		k := &p.kern[t]
		a[t] += ((ft[0]*k[0] + ft[1]*k[1]) + (ft[2]*k[2] + ft[3]*k[3])) +
			((ft[4]*k[4] + ft[5]*k[5]) + (ft[6]*k[6] + ft[7]*k[7])) +
			((ft[8]*k[8] + ft[9]*k[9]) + (ft[10]*k[10] + ft[11]*k[11])) +
			((ft[12]*k[12] + ft[13]*k[13]) + ft[14]*k[14])
	}
}

func (p *predictor) addC(a []float64) {
	ft := predFeaturesC(p.x)
	for t := range a {
		k := &p.kc[t]
		a[t] += (ft[0]*k[0] + ft[1]*k[1]) + (ft[2]*k[2] + ft[3]*k[3]) + (ft[4]*k[4] + ft[5]*k[5])
	}
}
