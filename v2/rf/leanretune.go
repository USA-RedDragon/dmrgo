//go:build !tinygo

package rf

import "github.com/USA-RedDragon/dmrgo/v2/enums"

type lockRetune struct {
	want     func(*leanCore) bool
	p        enums.SyncPattern
	src      int
	verified bool
}

func (r *leanCore) dueLock() bool { return r.lockDue }

func (r *leanCore) wantRetune(p enums.SyncPattern, src int, verified bool) bool {
	return !r.locked && r.hasExt() && r.ext.lockTune(r, p, src, verified)
}

func (r *leanCore) relock(p enums.SyncPattern, src int, verified bool) {
	r.lockDue = false
	if verified {
		r.fitLevels(r.ns-1, p, true, false)
		var d float32
		if t, ok := r.ext.lockShift(r, p, 0); ok {
			d = t * 2 * r.tm.step
			r.shiftStrobe(d)
		}
		r.lockStream(src, d, 0, 0)
		r.unproven = true
		r.ext.weakAt(r.ns - 1)
		return
	}
	var saved [syncSymbols]int8
	d, c2, _, ok := r.resample(p, &saved, false, discLock)
	if ok && c2 >= discLock*discLock && (r.fitLevels(r.ns-1, p, true, true) || r.pairLv && r.fitLevels(r.ns-1, p, true, false)) {
		r.resampleStart(d)
		r.lockStream(src, d, c2, 0)
		return
	}
	if ok {
		r.restoreSync(&saved)
	}
	if r.fitLevels(r.ns-1, p, true, true) {
		r.lockStream(src, 0, 0, 0)
	}
}

func (x *softExt) lockTune(c *leanCore, p enums.SyncPattern, src int, verified bool) bool {
	if x.rt.want == nil || !x.rt.want(c) {
		return false
	}
	x.rt.p, x.rt.src, x.rt.verified = p, src, verified
	return true
}

func (x *softExt) relock(c *leanCore) {
	c.relock(x.rt.p, x.rt.src, x.rt.verified)
}
