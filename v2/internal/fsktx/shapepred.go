package fsktx

import "math"

// Sizes of the out-of-band phase predictor of IQConfig.ShapePred, in
// shaper samples at iqShaperHz: each symbol adds predF products of its
// symbol and the next three, each times a kernel of predK samples from the
// start of the symbol. The output lags by predDelay samples so that every
// kernel of a sample is in.
const (
	predF     = 15
	predK     = 320
	predRing  = 64
	predDelay = 30
	predSpan  = 3
	predSPS   = iqShaperHz / SymbolRate
)

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
	acc     [predK + predSPS]float64
	f       [predRing]float64
	x       [predSpan + 1]float64
	nIn     int
	nSym    int
	prevAcc float64
}

// process takes the symbol s and its shaped deviation in buf and replaces
// it with the delayed corrected deviation.
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
			k := &shapeKern[t]
			a[t] += ((ft[0]*k[0] + ft[1]*k[1]) + (ft[2]*k[2] + ft[3]*k[3])) +
				((ft[4]*k[4] + ft[5]*k[5]) + (ft[6]*k[6] + ft[7]*k[7])) +
				((ft[8]*k[8] + ft[9]*k[9]) + (ft[10]*k[10] + ft[11]*k[11])) +
				((ft[12]*k[12] + ft[13]*k[13]) + ft[14]*k[14])
		}
	}
	p.nSym++
	for i, v := range buf {
		p.f[p.nIn&(predRing-1)] = v
		p.nIn++
		m := p.nIn - 1 - predDelay
		if m < 0 || i >= predSPS {
			buf[i] = 0
			continue
		}
		a := p.acc[i]
		buf[i] = p.f[m&(predRing-1)] + (a-p.prevAcc)*iqShaperHz/(2*math.Pi)
		p.prevAcc = a
	}
	if p.nSym > predSpan {
		copy(p.acc[:], p.acc[predSPS:])
		clear(p.acc[predK:])
	}
}
