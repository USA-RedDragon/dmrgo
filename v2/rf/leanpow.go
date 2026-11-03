//go:build !tinygo

package rf

type leanPow struct {
	p *iqPower
}

func newIQPower(hz int, lag float64) *iqPower {
	return &iqPower{per: max(hz*iqPowerMs/1000, 1), rate: float64(hz), lag: lag}
}

func (r *leanCore) setPower(p *iqPower) {
	r.pow.p = p
}

func (r *leanCore) slotPower(base int64) (float32, bool) {
	if r.pow.p == nil {
		return 0, false
	}
	t0 := tickSeconds(r.burstTicks(base))
	return r.pow.p.mean(t0+leanBurstSec/4, t0+3*leanBurstSec/4)
}
