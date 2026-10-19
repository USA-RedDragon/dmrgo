package fsktx

import "math"

// Sizes of the out-of-band phase predictor of IQConfig.ShapePred, in
// shaper samples at iqShaperHz: each symbol adds predF products of its
// symbol and the next three, each times a kernel of predK samples from the
// start of the symbol. The output lags by predDelay samples so that every
// kernel of a sample is in.
const (
	predF     = 9
	predK     = 320
	predRing  = 1024
	predDelay = 40
	predSpan  = 3
)

// predictor adds to the shaped deviation a phase correction that is a
// fixed polynomial function of the symbols, fitted to the corrections of
// the optimizer of IQConfig.Shape (shapefit_test.go). The kernels have
// nothing below 3 kHz, as the optimizer's correction.
type predictor struct {
	f, acc  [predRing]float64
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
		x0, x1, x2, x3 := p.x[0], p.x[1], p.x[2], p.x[3]
		x00 := x0 * x0
		ft := [predF]float64{x0, x00, x00 * x0, x0 * x1, x0 * x2, x0 * x3, x00 * x1, x0 * x1 * x1, x0 * x1 * x2}
		base := (p.nSym - predSpan) * iqShaperHz / SymbolRate
		for k, c := range ft {
			if c == 0 {
				continue
			}
			kr := &shapeKern[k]
			for t, v := range kr {
				p.acc[(base+t)&(predRing-1)] += c * v
			}
		}
	}
	p.nSym++
	for i, v := range buf {
		p.f[p.nIn&(predRing-1)] = v
		p.nIn++
		m := p.nIn - 1 - predDelay
		if m < 0 {
			buf[i] = 0
			continue
		}
		a := p.acc[m&(predRing-1)]
		p.acc[m&(predRing-1)] = 0
		buf[i] = p.f[m&(predRing-1)] + (a-p.prevAcc)*iqShaperHz/(2*math.Pi)
		p.prevAcc = a
	}
}
