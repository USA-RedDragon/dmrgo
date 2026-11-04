package rf

func (r *leanCore) slotPowerOf(p *iqPower, base int64) (on, off float32, ok bool) {
	ticks, _, _ := r.burstTicks(base)
	t0 := tickSeconds(ticks)
	if on, ok = p.mean(t0+leanBurstSec/4, t0+3*leanBurstSec/4); !ok {
		return 0, 0, false
	}
	off = on
	for k := -3; k <= 3; k++ {
		t := t0 + float64(k)*iqPowerUs/1e6
		if p, ok := p.mean(t, t); ok {
			off = min32(off, p)
		}
	}
	for k := 1; k <= 3; k++ {
		t := t0 + leanBurstSec + float64(k)*iqPowerUs/1e6
		if p, ok := p.mean(t, t); ok {
			off = min32(off, p)
		}
	}
	return on, off, true
}
