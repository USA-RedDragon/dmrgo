package rf

import "github.com/USA-RedDragon/dmrgo/v2/enums"

func (r *leanCore) fitLevels(end int64, p enums.SyncPattern, first, check bool) bool {
	g, dc, res := r.fitSync(end, p)
	r.stepHold = false
	if first && check && !tightSync(g, res) {
		return false
	}
	var step float32
	if !first && g > 0 {
		se := sqrtf(res/(syncSymbols-2)/9) / deviationHz
		z := (g - r.devScale) / max32(se, 1e-6)
		jump := abs32(z) > levelJumpZ
		twice := jump && r.jumpZ*z > 0
		r.jumpZ = 0
		if jump && !twice {
			r.jumpZ = z
		}
		if first = twice || r.heldOff(end, p, res); first {
			r.preDev, r.preOff, r.preStep = r.devScale, r.off-r.dcHz, !r.isReplaying()
			k := max32(g, leanMinScale) / r.devScale
			gk := max32(g, leanMinScale) * deviationHz
			r.tm.pow = 5*gk*gk + res
			r.tm.gain = r.trackGain
			step = k
		}
	}
	if first {
		g = max32(g, leanMinScale)
		r.devScale, r.dcHz, r.discVar, r.refit, r.looseLv = g, dc, res, false, !tightSync(g, res)
		r.levelN, r.levelEnd = levelWarm, uint16(end)
		r.syncFitRes = res
		if r.audio != nil {
			r.fitHz = r.audioHz(end, dc)
		}
		r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
		if step > 0 && r.hasExt() && !r.isReplaying() {
			base := end - leanSyncEnd
			off := func(dc float32) int32 { return int32(roundf((r.off - dc) / llrStepHz)) }
			dev := func(g float32) int32 { return int32(roundf(g * deviationHz / llrStepHz)) }
			r.ext.stepped(r, base+int64(r.splitAt(base, off(r.off-r.preOff), dev(r.preDev), off(dc), dev(g))))
		}
		return true
	}
	if check {
		g, dc = r.inlierLevels(end, p, g, dc)
	}
	f := float32(levelForget)
	if r.levelN > 0 && uint16(end) != r.levelEnd {
		f = 1 - 1/float32(levelWarm+2-r.levelN)
		r.levelN--
	}
	r.levelEnd = uint16(end)
	r.devScale = f*r.devScale + (1-f)*g
	r.dcHz = f*r.dcHz + (1-f)*dc
	r.discVar = f*r.discVar + (1-f)*res
	r.looseLv = r.looseLv && !tightSync(g, res)
	r.syncFitRes = res
	if r.audio != nil {
		r.fitHz = f*r.fitHz + (1-f)*r.audioHz(end, dc)
	}
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
	return true
}

// audioHz gives the carrier offset in Hz of an audio input whose SYNC that
// ends at end fits the DC dc: the tune of the front at that symbol and dc
// over the gain of the front.
func (r *leanCore) audioHz(end int64, dc float32) float32 {
	t, o := r.tuneHz, r.fcOff
	if int32(uint32(end)-r.tuneNs) < 0 {
		t, o = r.tunePrev, r.fcPrev
	}
	return (t+dc)/r.audio.gain + o
}

func (r *leanCore) syncRes(end int64, p enums.SyncPattern, g, dc float32) float32 {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	j := ringPos(end - syncSymbols + 1)
	var res float32
	for k := range syncSymbols {
		e := float32(r.ring[j])*discStepHz + r.off - dc - g*deviationHz*float32(syms[k])
		if j++; j == leanRing {
			j = 0
		}
		res += e * e
	}
	return res / syncSymbols
}

func (r *leanCore) fitSync(end int64, p enums.SyncPattern) (g, dc, res float32) {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	return r.fitSyms(end, &syms)
}

func (r *leanCore) fitSyms(end int64, syms *[syncSymbols]int8) (g, dc, res float32) {
	j0 := ringPos(end - syncSymbols + 1)
	var mp, mx float32
	j := j0
	for k := range syncSymbols {
		mx += float32(r.ring[j])*discStepHz + r.off
		if j++; j == leanRing {
			j = 0
		}
		mp += float32(syms[k])
	}
	mp /= syncSymbols
	mx /= syncSymbols
	var sxy, spp float32
	j = j0
	for k := range syncSymbols {
		sxy += (float32(syms[k]) - mp) * (float32(r.ring[j])*discStepHz + r.off - mx)
		if j++; j == leanRing {
			j = 0
		}
		spp += (float32(syms[k]) - mp) * (float32(syms[k]) - mp)
	}
	g = sxy / spp / deviationHz
	dc = mx - g*deviationHz*mp
	j = j0
	for k := range syncSymbols {
		e := float32(r.ring[j])*discStepHz + r.off - dc - g*deviationHz*float32(syms[k])
		if j++; j == leanRing {
			j = 0
		}
		res += e * e
	}
	return g, dc, res / syncSymbols
}

func (r *leanCore) inlierLevels(end int64, p enums.SyncPattern, g, dc float32) (float32, float32) {
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	ref := max(r.devScale, leanMinScale) * deviationHz
	lim := leanSyncClip * ref
	var n, sp, spp, sx, spx float32
	j := ringPos(end - syncSymbols + 1)
	out := 0
	for k := range syncSymbols {
		v := float32(r.ring[j])*discStepHz + r.off
		if j++; j == leanRing {
			j = 0
		}
		s := float32(syms[k])
		if abs32(v-r.dcHz-ref*s) > lim && abs32(v-dc-g*deviationHz*s) > lim {
			out++
			continue
		}
		n++
		sp += s
		spp += s * s
		sx += v
		spx += s * v
	}
	den := n*spp - sp*sp
	if out == 0 || out > leanSyncClipMax || den <= 0 {
		return g, dc
	}
	gd := (n*spx - sp*sx) / den
	return gd / deviationHz, (sx - gd*sp) / n
}

func (r *leanCore) relevel(g float32) {
	g = r.scaleAudio(g)
	r.devScale, r.dcHz, r.discVar = r.devScale*g, r.dcHz*g, r.discVar*g*g
	for i := range r.ms.lv {
		lv := &r.ms.lv[i]
		lv.devScale, lv.dcHz, lv.discVar = lv.devScale*g, lv.dcHz*g, lv.discVar*g*g
	}
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
}

func (r *leanCore) fitScale() {
	r.relevel(1 / r.devScale)
}

// blindLevels refits the levels of a flywheel burst from its own symbols after
// a level step of an audio input. The discriminator of an I/Q input measures
// Hz at any input level, so a level step there does not move the levels. The
// step scales the whole input, so the held levels of the other slot take the
// same step.
func (r *leanCore) blindLevels(slot int32, base int64, anchor enums.SyncPattern) {
	if a := r.audio; anchor != 0 || a == nil || a.iq {
		return
	}
	const n = slotSymbols - cachSymbols
	j0 := ringPos(base + cachSymbols)
	var sum, spread float32
	j := j0
	for range n {
		v := float32(r.ring[j])*discStepHz + r.off
		sum += v
		spread += abs32(v - r.dcHz)
		if j++; j == leanRing {
			j = 0
		}
	}
	dc := sum / n
	unit := r.devScale * deviationHz
	if q := spread / (n * 2 * unit); abs32(dc-r.dcHz) < unit && q > leanBlindStep && q < 1/leanBlindStep {
		return
	}
	var a float32
	j = j0
	for range n {
		a += abs32(float32(r.ring[j])*discStepHz + r.off - dc)
		if j++; j == leanRing {
			j = 0
		}
	}
	g := a / n / (2 * deviationHz)
	k := g / r.devScale
	if g < leanMinScale || k > leanBlindStep && k < 1/leanBlindStep || k < leanBlindStep*leanBlindStep || k > 1/(leanBlindStep*leanBlindStep) {
		return
	}
	g, dc = r.eyeFit(j0, g, dc)
	fresh, held := r.eyeNear(j0, g, dc), r.eyeNear(j0, r.devScale, r.dcHz)
	if float32(fresh) < leanBlindNear*n || float32(fresh-held) < leanBlindGain*n {
		return
	}
	k = g / r.devScale
	held0 := r.dcHz
	r.devScale, r.dcHz, r.discVar = g, dc, r.discVar*k*k
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
	r.stepOther(slot, k, held0)
	if r.hasExt() && !r.isReplaying() {
		r.ext.stepped(r, -1)
	}
}

// stepOther gives the held levels of the slot after slot the level step k
// that the levels of slot took from dc0 to r.dcHz. The step scales the whole
// input, so the offset between the two talkers scales with it.
func (r *leanCore) stepOther(slot int32, k, dc0 float32) {
	if r.bs {
		return
	}
	lv := &r.ms.lv[(slot+1)&1]
	lv.devScale, lv.dcHz, lv.discVar = lv.devScale*k, r.dcHz+k*(lv.dcHz-dc0), lv.discVar*k*k
}

// eyeFit refits the levels g and dc of the burst at ring position j by least
// squares on the symbols sliced with g and dc. The mean and the mean absolute
// deviation of a burst follow its symbols: a burst with more symbols on one
// side or more outer symbols moves them. With inner and outer symbols in the
// burst, the mean absolute deviation is off by at most a factor of 1.5 or 2;
// a refit beyond a factor of 2 comes from a slicing with one kind of symbol
// only, and the levels stay.
func (r *leanCore) eyeFit(j uint32, g, dc float32) (float32, float32) {
	const n = slotSymbols - cachSymbols
	var s1, s2, sv, svs float32
	for range n {
		v := float32(r.ring[j])*discStepHz + r.off
		if j++; j == leanRing {
			j = 0
		}
		y := (v - dc) / (g * deviationHz)
		s := float32(1)
		if abs32(y) >= 2 {
			s = 3
		}
		if y < 0 {
			s = -s
		}
		s1 += s
		s2 += s * s
		sv += v
		svs += v * s
	}
	d := n*s2 - s1*s1
	if d <= 0 {
		return g, dc
	}
	gd := (n*svs - s1*sv) / d / deviationHz
	if k := gd / g; gd < leanMinScale || k < leanBlindStep || k > 1/leanBlindStep {
		return g, dc
	}
	return gd, (sv - gd*deviationHz*s1) / n
}

func (r *leanCore) eyeNear(j uint32, g, dc float32) int {
	c := 0
	for range slotSymbols - cachSymbols {
		y := abs32((float32(r.ring[j])*discStepHz + r.off - dc) / (g * deviationHz))
		if j++; j == leanRing {
			j = 0
		}
		if y < 2 {
			y--
		} else {
			y -= 3
		}
		if abs32(y) < 0.5 {
			c++
		}
	}
	return c
}

func (r *leanCore) stepSplit(base int64) (int, int32, int32) {
	if !r.preStep {
		return 0, 0, 0
	}
	r.preStep = false
	off := int32(roundf(r.preOff / llrStepHz))
	dev := int32(roundf(r.preDev * deviationHz / llrStepHz))
	return r.splitAt(base, off, dev, r.llrOff, r.llrDev), off, 2 * dev
}

// splitAt gives the number of symbols of the burst at base before a level
// step: the head that the levels off and dev fit better than the levels
// offNew and devNew.
func (r *leanCore) splitAt(base int64, off, dev, offNew, devNew int32) int {
	var c, best int32
	split := 0
	h := ringPos(base)
	for k := range syncFirstSym {
		v := int32(r.ring[h]) * (discStepHz / llrStepHz)
		if h++; h == leanRing {
			h = 0
		}
		if c += symRes(v+off, dev) - symRes(v+offNew, devNew); c < best {
			best, split = c, k+1
		}
	}
	return split
}

//go:noinline
func (r *leanCore) sliceHead(b *Burst, base int64, split int, off, thr int32) {
	h := ringPos(base)
	for k := range split {
		x := int32(r.ring[h])*(discStepHz/llrStepHz) + off
		if h++; h == leanRing {
			h = 0
		}
		var d byte
		if x < 0 {
			d = 2
			x = -x
		}
		if x > thr {
			d |= 1
		}
		sh := uint(6 - 2*(k&3))
		if k < cachSymbols {
			b.CACH[k/4] = b.CACH[k/4]&^(3<<sh) | d<<sh
		} else {
			j := (k - cachSymbols) / 4
			b.Data[j] = b.Data[j]&^(3<<sh) | d<<sh
		}
	}
}

func symRes(x, d int32) int32 {
	if x < 0 {
		x = -x
	}
	return min(abs32i(x-d), abs32i(x-3*d))
}

func (r *leanCore) heldOff(end int64, p enums.SyncPattern, res float32) bool {
	return r.syncRes(end, p, r.devScale, r.dcHz) > levelRefit*max32(res, levelRefitFloor)
}

func (r *leanCore) stepped(end int64, p enums.SyncPattern) bool {
	g, dc, res := r.fitSync(end, p)
	return tightSync(g, res) && r.heldOff(end, p, res) && r.syncErrors(end, p, g, dc) <= discAnchorBits
}

func tightSync(g, res float32) bool {
	return g > 0 && sqrtf(res) <= discFirstSpread*2*g*deviationHz
}

func (r *leanCore) levels() leanLevels {
	return leanLevels{r.devScale, r.dcHz, r.discVar, r.amp, r.noise, r.fitHz}
}

func (r *leanCore) saveLevels(i int32) {
	m := &r.ms
	m.lv[i], m.lvN[i] = r.levels(), r.levelN
	m.looseLv &^= 1 << i
	if r.looseLv {
		m.looseLv |= 1 << i
	}
}

func (r *leanCore) setLevels(l *leanLevels) {
	r.devScale, r.dcHz, r.discVar, r.amp, r.noise, r.fitHz = l.devScale, l.dcHz, l.discVar, l.amp, l.noise, l.fitHz
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
}

func (r *leanCore) sliceLevels() sliceLevels {
	return sliceLevels{r.off, r.dcHz, r.dev, r.discVar, r.ll, r.lfloor, r.llrOff, r.llrDev, r.llrFloor, r.llrK}
}

func (r *leanCore) setSliceLevels(l *sliceLevels) {
	r.off, r.dcHz, r.dev, r.discVar, r.ll, r.lfloor = l.off, l.dcHz, l.dev, l.discVar, l.ll, l.lfloor
	r.llrOff, r.llrDev, r.llrFloor, r.llrK = l.llrOff, l.llrDev, l.llrFloor, l.llrK
}
