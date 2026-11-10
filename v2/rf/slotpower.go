package rf

func (r *leanCore) slotPower(base int64, head bool) (on, off float32, ok bool) {
	p := r.pow
	if !powerOn() || p == nil {
		return 0, 0, false
	}
	ticks, _, _ := r.burstTicks(base)
	t0 := tickSeconds(ticks)
	end := t0 + 3*leanBurstSec/4
	if head {
		end = t0 + leanBurstSec/2
	}
	if on, ok = p.mean(t0+leanBurstSec/4, end); !ok {
		return 0, 0, false
	}
	off = on
	for k := -3; k <= 3; k++ {
		t := t0 + float64(k)*iqPowerUs/1e6
		if p, ok := p.mean(t, t); ok {
			off = min32(off, p)
		}
	}
	for k := 1; k <= 3 && !head; k++ {
		t := t0 + leanBurstSec + float64(k)*iqPowerUs/1e6
		if p, ok := p.mean(t, t); ok {
			off = min32(off, p)
		}
	}
	return on, off, true
}
