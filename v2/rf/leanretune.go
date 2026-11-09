package rf

import "github.com/USA-RedDragon/dmrgo/v2/enums"

type lockRetune struct {
	want     func(*leanCore) bool
	p        enums.SyncPattern
	src      int
	verified bool
	wideOK   bool
	wideZ    float32
	wideNs   int32
}

func (r *leanCore) dueLock() bool { return r.hasExt() && r.lockDue }

func (r *leanCore) wantRetune(p enums.SyncPattern, src int, verified bool) bool {
	return !r.locked() && r.hasExt() && r.ext.lockTune(r, p, src, verified)
}

func (r *leanCore) relock(p enums.SyncPattern, src int, verified bool, at *float32) {
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
	d, c2, _, ok := r.resample(p, &saved, false, discLock, at)
	if ok && c2 >= discLock*discLock && (r.fitLevels(r.ns-1, p, true, true) || r.fitLevels(r.ns-1, p, true, false)) {
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
	var at *float32
	if x.rt.wideOK {
		at = &x.rt.wideZ
	}
	x.rt.wideOK = false
	c.relock(x.rt.p, x.rt.src, x.rt.verified, at)
	x.rt.wideNs = -1
	if at != nil {
		x.rt.wideNs = c.fitNs
	}
}

// wideFit times the sync of a coming relock on the kept samples that the
// wide channel filter gave at the new tune. The narrow filter bandlimits the
// FM, and the MM zero of the sync on its output moved 0.2 us with the sync
// pattern; the relock then takes this zero on the narrow samples.
func (x *softExt) wideFit(c *leanCore) {
	x.rt.wideOK = false
	if x.rt.verified {
		return
	}
	var saved [syncSymbols]int8
	again := c.again
	if _, _, z, ok := c.resample(x.rt.p, &saved, false, discLock, nil); ok {
		c.restoreSync(&saved)
		x.rt.wideZ, x.rt.wideOK = z, true
	}
	c.again = again
}
