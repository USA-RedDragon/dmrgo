//go:build !tinygo

package rf

type leanPow struct {
	p *iqPower
}

func (r *leanCore) setPower(p *iqPower) {
	r.pow.p = p
}

func (r *leanCore) slotPower(base int64) (on, off float32, ok bool) {
	if r.pow.p == nil {
		return 0, 0, false
	}
	return r.slotPowerOf(r.pow.p, base)
}

func wantPower(*options) bool { return true }
