package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type exactStage struct {
	xt  *xtrellis
	buf []complex64
	app []float32
}

func newExactStage(m *cpmModel, opt *options, maxRun int) *exactStage {
	np := opt.PhaseStates
	if np == 0 {
		np = phaseStates
	}
	xt := newXTrellis(m, np, maxRun)
	xt.logmap = opt.LogMAP
	if opt.DriftPenalty > 0 {
		xt.pen = opt.DriftPenalty
	}
	xt.jump, xt.fade, xt.below = opt.PhaseJump, opt.PhaseJumpFade, opt.PhaseJumpBelow
	if opt.DriftSteps > 0 {
		xt.drifts = int(opt.DriftSteps)
	}
	return &exactStage{xt: xt, buf: make([]complex64, maxRun*m.sps), app: make([]float32, 4*maxRun)}
}

func (e *exactStage) exec(steps int, rot complex64, scale, kappa float32) []float32 {
	m := e.xt.m
	e.xt.run(e.buf[m.d*m.sps:], steps, rot, scale, kappa, e.app)
	return e.app[:4*steps]
}

func fixedPrior(lp *[4]float32, fixed *layer2.RawBurst, known *[layer2.BurstBits]bool, k int, w float32) {
	for b := range 2 {
		i := 2*k + b
		if !known[i] {
			continue
		}
		bit := unpacked.Bit(fixed[i/8] >> (7 - i%8) & 1)
		for d := range 4 {
			b0, b1 := dibitOfDigit(d)
			v := b0
			if b == 1 {
				v = b1
			}
			if v == bit {
				lp[d] += w
			}
		}
	}
}

func extPrior(lp *[4]float32, e0, e1, w float32) {
	for d := range 4 {
		b0, b1 := dibitOfDigit(d)
		lp[d] = bitPrior(w*e0, b0) + bitPrior(w*e1, b1)
	}
}

func bitPrior(l float32, b unpacked.Bit) float32 {
	if b == 0 {
		return l / 2
	}
	return -l / 2
}
