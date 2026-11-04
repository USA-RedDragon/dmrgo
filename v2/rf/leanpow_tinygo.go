//go:build tinygo

package rf

type leanPow struct{}

func (r *leanCore) setPower(p *iqPower) {
	if x, ok := r.ext.(*softExt); ok && r.hasExt() {
		x.pow = p
	}
}

func (r *leanCore) slotPower(base int64) (on, off float32, ok bool) {
	x, ok := r.ext.(*softExt)
	if !r.hasExt() || !ok || x.pow == nil {
		return 0, 0, false
	}
	return r.slotPowerOf(x.pow, base)
}

func wantPower(opt *options) bool { return opt.SoftExt }
