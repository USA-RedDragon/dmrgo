//go:build !tinygo

package rf

type leanPow struct {
	p *iqPower
}

func newIQPower(hz int, lag float64) *iqPower {
	return &iqPower{per: max(hz*iqPowerUs/1000000, 1), rate: float64(hz), lag: lag}
}

func (r *leanCore) setPower(p *iqPower) {
	r.pow.p = p
}

func (r *leanCore) slotPower(base int64) (on, off float32, ok bool) {
	if r.pow.p == nil {
		return 0, 0, false
	}
	t0 := tickSeconds(r.burstTicks(base))
	if on, ok = r.pow.p.mean(t0+leanBurstSec/4, t0+3*leanBurstSec/4); !ok {
		return 0, 0, false
	}
	off = on
	for k := -3; k <= 3; k++ {
		t := t0 + float64(k)*iqPowerUs/1e6
		if p, ok := r.pow.p.mean(t, t); ok {
			off = min32(off, p)
		}
	}
	return on, off, true
}
