package rf

import (
	"math"
	"math/bits"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/quadraticresidue"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

// leanSlots is the state of the two talkers of an MS stream, one per slot
// parity: the TDMA slots of direct mode or the two MS of a repeater input
// . Each parity is in one of three states: no
// anchor yet (anch is leanNoSlot), fresh (an anchor in the last fresh slots,
// whose timing seekMid fits again on the EMB of the next burst), or
// anchored. anch holds the slot of the last anchor, off the grid offset of
// the parity in symbols, t, n and ns the strobe phase it left at its last
// slot, lv, lvN and looseLv its levels, and clk, clkAt and clkNs the clock
// of the other talker. At the end of each slot slotSwitch saves the state of
// the parity that ends and loads that of the next. seek marks a slot whose
// talker the hunt seeks (seeking), late that the seek moved the slot end by
// a symbol, moved that a retime moved its timing, and retry that the next
// SYNC span of the hunt tries the seek again.
type leanSlots struct {
	anch    [2]int32
	t       [2]float32
	n, ns   [2]uint16
	off     [2]int16
	lv      [2]leanLevels
	moved   bool
	seek    bool
	retry   bool
	late    bool
	fresh   [2]uint8
	lvN     [2]uint8
	clk     leanClock
	clkNs   uint16
	clkAt   uint8
	looseLv uint8
}

func (r *leanCore) slotSwitch(j int32) {
	if r.bs || r.lastAnch == leanNoSlot {
		return
	}
	m := &r.ms
	p, q := j&1, (j+1)&1
	live := !r.isReplaying()
	sps := 2 * r.tm.step
	if m.anch[p] != leanNoSlot {
		if live {
			r.savePhase(p)
		}
		r.saveLevels(p)
	}
	if m.fresh[p] > 0 {
		m.fresh[p]--
	}
	if m.anch[q] != leanNoSlot && j+1-m.anch[q] >= lostSlots {
		m.anch[q] = leanNoSlot
	}
	moved := m.moved
	m.moved = false
	if m.anch[q] == leanNoSlot {
		m.off[q] = m.off[p]
		r.tm.hold = true
		return
	}
	r.tm.hold = false
	if m.anch[p] == leanNoSlot && !moved {
		return
	}
	if m.anch[p] != leanNoSlot {
		r.setLevels(&m.lv[q])
		r.levelN, r.looseLv = m.lvN[q], m.looseLv>>q&1 != 0
	}
	if !live {
		return
	}
	if m.anch[p] != leanNoSlot {
		r.swapClock(p, q)
	}
	lo := float32(0)
	if !r.tm.mm && r.tm.mid {
		lo = r.tm.step
	}
	u := m.t[q] + float32(int16(m.n[q]-uint16(r.n))) + float32(int16(uint16(r.ns)-m.ns[q]))*(sps+r.tm.rate)
	cur := r.symWait()
	for k := 0; (u <= lo || u <= cur-sps/2) && k < 4; k++ {
		u += sps
		m.off[q]--
	}
	for k := 0; u > cur+sps/2 && u-sps > lo && k < 4; k++ {
		u -= sps
		m.off[q]++
	}
	r.tm.t = u - lo
	r.savePhase(q)
}

func (r *leanCore) saveClock(i int32) {
	m := &r.ms
	m.clk, m.clkAt, m.clkNs = r.tm.leanClock, uint8(i)+1, uint16(r.ns)
	r.tm.newTalker()
	r.tm.gain = r.trackGain
}

func (r *leanCore) swapClock(p, q int32) {
	m := &r.ms
	if m.clkAt != uint8(q)+1 {
		r.saveClock(p)
		return
	}
	gap := int32(int16(uint16(r.ns) - m.clkNs))
	r.tm.leanClock, m.clk = m.clk, r.tm.leanClock
	m.clkAt, m.clkNs = uint8(p)+1, uint16(r.ns)
	r.tm.nall += gap
}

func (r *leanCore) savePhase(i int32) {
	r.ms.t[i], r.ms.n[i], r.ms.ns[i] = r.symWait(), uint16(r.n), uint16(r.ns)
}

func (r *leanCore) symWait() float32 {
	if !r.tm.mm && r.tm.mid {
		return r.tm.t + r.tm.step
	}
	return r.tm.t
}

func (r *leanCore) slotFirst(j int32, base int64, anchor enums.SyncPattern) {
	o := (j + 1) & 1
	r.saveLevels(o)
	r.fitLevels(base+leanSyncEnd, anchor, true, false)
	r.amp = 0
	if !r.isReplaying() {
		r.ms.fresh[j&1] = 2
		r.saveClock(o)
	}
}

func (r *leanCore) holdNext(j int32, present bool) {
	r.tm.hold = !r.bs && !present
	if !r.bs && r.lastAnch != leanNoSlot {
		r.tm.hold = r.ms.anch[(j+1)&1] == leanNoSlot
	}
	r.tm.hold = r.tm.hold || r.stepHold
}

// holdGuard keeps the timing loop of an MS stream held over the guard at
// the start of the next slot, where the ramp of the transmitter and the
// carrier-off noise give no symbols, and frees it at the first burst symbol.
func (r *leanCore) holdGuard() {
	if !r.bs && !r.tm.hold && !r.isReplaying() {
		r.tm.hold, r.tm.wake = true, cachSymbols
	}
}

func (r *leanCore) wakeStep() {
	if r.tm.wake != 0 {
		if r.tm.wake--; r.tm.wake == 0 {
			r.tm.hold, r.tm.stale = false, true
		}
	}
}

// lockFresh counts the MS talker that a core with no soft stages locked on
// with a voice SYNC in slot j as fresh, so that seekMid fits its timing
// again on the EMB of its next burst.
func (r *leanCore) lockFresh(j int32, voice bool) {
	if j == 0 && voice && !r.bs && !r.hasExt() {
		r.ms.fresh[0] = 2
	}
}

// seekMid seeks, in a core with no soft stages, an MS talker that has no
// SYNC anchor yet by the EMB of the slot at the end of its SYNC field, while
// the hunt buffer still holds the burst up to there. A seek at the end of the
// slot, as the soft stages run it, needs the samples of the whole slot. In
// the next burst of a fresh talker, the one it took on its EMB or the one
// the stream locked on, it fits the timing again on the EMB: that talker's
// clock rate is not known yet, and the phase it carried over the other slot
// can be off at the start of the burst. Where
// the core measures the I/Q power, the head of the burst must stand 10 dB
// over the guard before it, as the slot power test at the end of the slot
// asks of the whole burst.
func (r *leanCore) seekMid() bool {
	j := r.next
	if r.hasExt() || !r.ms.seek || r.ns != r.slotEnd-slotSymbols+leanSyncEnd+1 || r.ms.anch[j&1] != leanNoSlot && r.ms.fresh[j&1] == 0 {
		return false
	}
	base := r.first + int64(j)*slotSymbols + int64(r.ms.off[j&1])
	if on, off, ok := r.slotPower(base, true); ok && on <= leanSlotContrast*off || !ok && r.eSyms > 0 && !r.powered(r.eAcc/float32(r.eSyms), 0, true, false) {
		return false
	}
	sh, ok := r.embAnchor(j, base)
	if !ok {
		return false
	}
	r.slotEnd += sh
	r.ms.seek, r.tm.hold = false, false
	return true
}

func (r *leanCore) embSeek(j int32, base int64, present, known, measured bool) (int64, bool) {
	if !r.hasExt() || !present && (known || measured) {
		return 0, false
	}
	return r.embAnchor(j, base)
}

func (r *leanCore) seekNoise(energy float32, known, measured bool) {
	if known || measured {
		r.noise = min32(r.noise, energy)
	}
}

// embAnchor takes the MS slot at base, in the slot parity of a talker that
// has no SYNC anchor yet, as that talker's voice burst B or E when its EMB,
// sliced at the grid of the locked talker, carries the stream's color code
// and the LCSS of burst B or E with at most one corrected bit (ETSI TS 102
// 361-1 clause 7.1.3). A core with no soft stages keeps no slot back to
// confirm the EMB by the next one, so before any color code vote it takes
// an EMB of burst B or E that reads with no bit corrected. The 24 sliced
// symbols of the SYNC field then stand in for a SYNC pattern: their timing
// fit on the kept samples resamples the slot and moves the talker's strobe
// phase, and their level fit gives the talker's levels, as a SYNC anchor's
// fit does.
func (r *leanCore) embAnchor(j int32, base int64) (int64, bool) {
	cc, known := r.colorCode()
	end := base + leanSyncEnd
	sym, at0, sps := r.ns-1, r.lastAt, 2*r.tm.step+r.tm.rate
	if r.isReplaying() {
		sym, at0, sps = r.ext.replayGrid(r)
	}
	back := sym - end
	at := at0 - float32(back)*sps
	if back < 0 || float32(int(back)+syncSymbols+4)*2*r.tm.step+4-at0 > float32(r.acqKept()) {
		return 0, false
	}
	syms, emb, shift, top := r.embFind(at, sps, known, cc)
	if top == embNone {
		return 0, false
	}
	at += shift
	strong := top == 0
	agrees := r.hasExt() && r.ext.seekAgrees(j, emb.ColorCode, emb.LCSS)
	if !strong && !agrees {
		return 0, false
	}
	if !known {
		cc = emb.ColorCode
	}
	var win syncWindow
	r.fillWindow(&win, max(int32(-at-sps/2)-2, 0))
	var cv [7]float32
	best := win.peak(&syms, &cv, at, sps, true)
	if best == 0 || best == len(cv)-1 {
		return 0, false
	}
	d := float32(best-3)*sps/4 + edgeVertex(&cv, best)*sps/4
	sv, svv, sxv, sp := win.sums(&syms, at+d, sps)
	d += at - (at0 - float32(back)*sps)
	sh := int64(roundf(d / sps))
	d -= float32(sh) * sps
	base += sh
	end += sh
	var spp float32
	for _, q := range syms {
		spp += float32(q) * float32(q)
	}
	const n = syncSymbols
	sxy, sxx, pp := sxv-sp*sv/n, svv-sv*sv/n, spp-sp*sp/n
	if sxy <= 0 || sxx <= 0 || pp <= 0 || sxy*sxy < discLock*discLock*sxx*pp {
		return 0, false
	}
	kept := sym + 1 - int64((float32(r.acqKept())-4+at0)/sps)
	lo, hi := max(base-1, kept), min(base+slotSymbols, r.ns-1)
	d = r.ringZero(lo, hi, base, sym, at0, d, sps)
	r.sliceSyms(end, &syms)
	g, dc, res := r.fitSyms(end, &syms)
	if emb = embOfSyms(&syms); !embWeak(&emb, true, cc) || g <= 0 {
		return 0, false
	}
	o := (j + 1) & 1
	refit := r.ms.anch[j&1] != leanNoSlot
	if !refit {
		r.embTalker(j, end, g, dc, res, &emb, known)
		r.ms.anch[j&1] = j
	}
	k := sh + int64(r.ms.off[j&1])
	r.first += k
	r.ms.off[(j+1)&1] -= int16(k)
	r.ms.off[j&1] = 0
	if r.isReplaying() {
		r.ext.replayPhase(j&1, d)
	} else {
		if !refit {
			r.ms.fresh[j&1] = 2
			r.saveClock(o)
		}
		r.fitNs, r.fitT, r.fitQ = int32(end), r.strobeTicks(r.lastAt+d-float32(r.ns-1-end)*sps), sxy*sxy/(sxx*pp)
		r.tm.t += d
		if r.hasExt() {
			r.ext.noteSpan(r, max(base-1, kept), base+slotSymbols, d, sps)
			if agrees {
				r.seekBackfill(j-2, base-2*slotSymbols, d, sps)
			}
		}
	}
	return sh, true
}

// embFind slices the SYNC field that ends at at, and at a quarter and a half
// symbol to each side, and returns the reading whose EMB ranks best: rank 0
// for an EMB that needs no corrected bit to carry the stream's color code,
// or, in a core with no soft stages before a color code vote, to place its
// burst; rank 1 plus its corrected bits for an EMB that fits the stream with
// at most one corrected bit; embNone when no reading holds such an EMB.
func (r *leanCore) embFind(at, sps float32, known bool, cc enums.ColorCode) (syms [syncSymbols]int8, emb pdu.EmbeddedSignalling, shift float32, top int) {
	var try [syncSymbols]int8
	top = embNone
	var eye float32
	for _, k := range [5]float32{0, -1, 1, -2, 2} {
		e, ok := r.sliceEMB(at+k*sps/4, sps, known, cc)
		if !ok {
			continue
		}
		var rank int
		switch {
		case known && embStrong(&e, cc), !known && !r.hasExt() && embPlaced(&e):
		case embWeak(&e, known, cc):
			rank = 1 + e.FEC.ErrorsCorrected
		default:
			continue
		}
		ek := r.sliceKept(at+k*sps/4, sps, &try)
		if rank < top || rank == top && ek < eye {
			top, shift, syms, emb, eye = rank, k*sps/4, try, e, ek
		}
		if rank == 0 {
			break
		}
	}
	if top != 0 && r.hasExt() {
		if vs, ve, vsh, rank, ek, ok := r.ext.vertexEMB(r, at, sps, known, cc); ok && (rank < top || rank == top && ek < eye) {
			syms, emb, shift, top = vs, ve, vsh, rank
		}
	}
	return syms, emb, shift, top
}

// eyeVertex returns the phase, in quarter symbols from at, where the 24
// symbols of the SYNC field that ends there slice with the least squared
// distance to their levels, from a parabola through the best of the five
// quarter symbol phases from -1/2 to +1/2 symbol and its neighbors. With the
// roll-off of 0.2 of the DMR pulse the eye closes fast off its center, so
// the nearest quarter symbol phase can miss outer symbols that the vertex
// phase reads. It reports false when the best phase is an outer one.
func (r *leanCore) eyeVertex(at, sps float32) (float32, bool) {
	var try [syncSymbols]int8
	var ek [5]float32
	for p := range ek {
		ek[p] = r.sliceKept(at+float32(p-2)*sps/4, sps, &try)
	}
	lo, a, b, c := 0, float32(0), float32(math.Inf(1)), float32(0)
	for p := 1; p+1 < len(ek); p++ {
		if ek[p] < b {
			lo, a, b, c = p, ek[p-1], ek[p], ek[p+1]
		}
	}
	den := a - 2*b + c
	if ek[0] <= b || ek[len(ek)-1] < b || den <= 0 {
		return 0, false
	}
	return float32(lo-2) + 0.5*(a-c)/den, true
}

// embTalker starts the talker of slot j on its EMB anchor: it keeps the
// levels of the other talker, takes the levels of the SYNC field fit, and
// before a color code vote places the burst in the superframe by the LCSS of
// its EMB.
func (r *leanCore) embTalker(j int32, end int64, g, dc, res float32, emb *pdu.EmbeddedSignalling, known bool) {
	r.saveLevels((j + 1) & 1)
	g = max32(g, leanMinScale)
	r.devScale, r.dcHz, r.discVar, r.amp, r.looseLv = g, dc, res, 0, true
	r.levelN, r.levelEnd = levelWarm, uint16(end)
	r.tm.dc, r.tm.dev = r.dcHz, r.devScale*deviationHz
	if known {
		return
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		r.ch[j&1].sf = j - 2*int32(enums.VoiceBurstB-enums.VoiceBurstA)
	case enums.LCSSLastFragment:
		r.ch[j&1].sf = j - 2*int32(enums.VoiceBurstE-enums.VoiceBurstA)
	case enums.LCSSSingleFragment, enums.LCSSContinuationFragment:
	}
}

// sliceEMB slices from the kept samples only the 8 EMB symbols of the SYNC
// field that ends at at, the first and last 4 of its 24 (ETSI TS 102 361-1
// clause 9.1.2), and reads the EMB from them. It reports no EMB unless the
// EMB needs at most one corrected bit, and with the color code known it
// stops after the first 2 symbols when they differ from it in 2 bits or
// more, since no such EMB can then carry it.
func (r *leanCore) sliceEMB(at, sps float32, known bool, cc enums.ColorCode) (pdu.EmbeddedSignalling, bool) {
	r.setLLR()
	var cw uint16
	for n, i := range [8]int{0, 1, 2, 3, syncSymbols - 4, syncSymbols - 3, syncSymbols - 2, syncSymbols - 1} {
		v := int32(roundf(r.keptAt(at - float32(syncSymbols-1-i)*sps)))
		cw = cw<<2 | dibitOf(r.sliceLevel(v))
		if n == 1 && known && bits.OnesCount16(cw^uint16(cc)) >= 2 {
			return pdu.EmbeddedSignalling{}, false
		}
	}
	if !embNear(cw) {
		return pdu.EmbeddedSignalling{}, false
	}
	return embOfCW(cw), true
}

// embNear reports whether cw lies at most one bit from a QR (16,7,6)
// codeword: its data bits encode to within one bit of it, or one flipped
// data bit makes it a codeword.
func embNear(cw uint16) bool {
	if bits.OnesCount16(quadraticresidue.Encode(uint8(cw>>9))^cw) <= 1 {
		return true
	}
	for k := 9; k < 16; k++ {
		if quadraticresidue.Check(cw ^ 1<<k) {
			return true
		}
	}
	return false
}

func (r *leanCore) sliceKept(at, sps float32, syms *[syncSymbols]int8) float32 {
	r.setLLR()
	var e float32
	for i := range syncSymbols {
		v := int32(roundf(r.keptAt(at - float32(syncSymbols-1-i)*sps)))
		syms[i] = r.sliceLevel(v)
		d := float32(v*(discStepHz/llrStepHz)+r.llrOff) - float32(syms[i])*float32(r.llrDev)
		e += d * d
	}
	return e
}

func (r *leanCore) sliceSyms(end int64, syms *[syncSymbols]int8) {
	r.setLLR()
	k0 := end - (syncSymbols - 1)
	for i := range syncSymbols {
		syms[i] = r.sliceLevel(int32(r.ring[ringPos(k0+int64(i))]))
	}
}

func (r *leanCore) sliceLevel(v int32) int8 {
	x := v*(discStepHz/llrStepHz) + r.llrOff
	q := int8(1)
	if x > 2*r.llrDev || -x > 2*r.llrDev {
		q = 3
	}
	if x < 0 {
		q = -q
	}
	return q
}

func (r *leanCore) ringZero(lo, hi, base, sym int64, at, d, sps float32) float32 {
	r.ringKept(lo, hi, sym, at+d, sps)
	for range leanZeroPasses {
		z, ok := r.burstZero(max(lo, base+cachSymbols+2), min(hi, base+slotSymbols-3), sym, at, d, sps)
		if !ok {
			break
		}
		moved := abs32(z - d)
		d = z
		r.ringKept(lo, hi, sym, at+d, sps)
		if moved < leanZeroStill {
			break
		}
	}
	return d
}

func (r *leanCore) ringKept(lo, hi, sym int64, at, sps float32) {
	for k := lo; k <= hi; k++ {
		v := r.keptAt(at - float32(sym-k)*sps)
		r.ring[ringPos(k)] = int8(max32(min32(roundf(v), math.MaxInt8), math.MinInt8))
	}
}

// burstZero moves the timing d of an EMB seek to the zero of the
// Mueller-Muller detector over the symbols lo to hi of the burst, sliced from
// the ring at d. The correlation peak of the 24 EMB symbols moves with the
// payload symbols next to them; the detector zero over the whole burst does
// not.
func (r *leanCore) burstZero(lo, hi, sym int64, at, d, sps float32) (float32, bool) {
	if hi-lo < slotSymbols/2 {
		return d, false
	}
	r.setLLR()
	x0, x1 := d-leanLockMMStep, d+leanLockMMStep
	f0, f1 := r.burstMM(lo, hi, sym, at+x0, sps), r.burstMM(lo, hi, sym, at+x1, sps)
	for range leanMMSteps {
		if f1 == f0 {
			break
		}
		x0, x1 = x1, x1-f1*(x1-x0)/(f1-f0)
		if abs32(x1-d) > sps/4 {
			return d, false
		}
		f0, f1 = f1, r.burstMM(lo, hi, sym, at+x1, sps)
	}
	return x1, true
}

func (r *leanCore) burstMM(lo, hi, sym int64, at, sps float32) float32 {
	prev := r.keptAt(at - float32(sym-lo)*sps)
	a0 := r.sliceLevel(int32(r.ring[ringPos(lo)]))
	first, e, m := a0, float32(0), prev
	for k := lo + 1; k <= hi; k++ {
		y := r.keptAt(at - float32(sym-k)*sps)
		a1 := r.sliceLevel(int32(r.ring[ringPos(k)]))
		e += float32(a0)*y - float32(a1)*prev
		m += y
		prev, a0 = y, a1
	}
	return e - m/float32(hi-lo+1)*float32(first-a0)
}

// seekBackfill decodes the talker's slot two slots before the one where
// its EMB seek closed, from the kept samples at the talker's fitted timing,
// for the slot whose EMB the seek noted first. The seek kept the samples of
// that slot, as the hunt keeps them for every seeked slot, so it reads them
// back from the whole kept buffer, past the start of the current hunt. The burst goes to the late
// queue until a later EMB or SYNC of the talker gives its position.
func (r *leanCore) seekBackfill(j int32, base int64, d, sps float32) {
	_, an := r.acqBuf()
	room := float32(min(int64(an), r.n-r.ext.keptSince())) + r.lastAt - 4
	lo := base - 1
	for float32(r.ns-lo)*sps > room && lo < base+cachSymbols-1 {
		lo++
	}
	if j < 0 || float32(r.ns-lo)*sps > room {
		return
	}
	saved := r.ring
	r.ringKept(lo, base+slotSymbols, r.ns-1, r.lastAt+d, sps)
	r.ext.backfillAt(d)
	fitNs, fitT := r.fitNs, r.fitT
	end := base + leanSyncEnd
	r.fitNs, r.fitT = int32(end), r.strobeTicks(r.lastAt+d-float32(r.ns-1-end)*sps)
	r.backfill = true
	r.burst(j, base, 0, false, false)
	r.ring, r.backfill = saved, false
	r.fitNs, r.fitT = fitNs, fitT
}

func (r *leanCore) seekMatch(d int32, mid bool) {
	rho2, _, _ := r.syncAt(r.ns - 1)
	if !mid && r.rcSource() {
		rho2 = max32(rho2, r.rcAt(r.ns-1))
	}
	if !mid && rho2 < leanSeekRho*leanSeekRho {
		return
	}
	if !r.ms.late && r.ms.anch[r.next&1] == leanNoSlot {
		r.ms.late = true
		r.slotEnd++
	}
	moved, retry := r.ms.moved, r.ms.retry
	r.ms.moved, r.ms.retry = false, false
	r.retime(mid)
	r.ms.retry = !r.ms.moved && !retry
	if r.ms.moved && d != 0 {
		r.ms.off[r.next&1] += int16(d)
		r.slotEnd += int64(d)
	}
	r.ms.moved = r.ms.moved || moved
}

func (r *leanCore) gridMiss() int32 {
	m := int32(mod64(r.ns-1-leanSyncEnd-r.first-int64(r.ms.off[r.next&1]), slotSymbols))
	if m > slotSymbols/2 {
		m -= slotSymbols
	}
	return m
}

func (r *leanCore) offGrid() bool {
	d := r.gridMiss()
	return d < -1 || d > 1
}

func embOfSyms(syms *[syncSymbols]int8) pdu.EmbeddedSignalling {
	var cw uint16
	for _, i := range [8]int{0, 1, 2, 3, syncSymbols - 4, syncSymbols - 3, syncSymbols - 2, syncSymbols - 1} {
		cw = cw<<2 | dibitOf(syms[i])
	}
	return embOfCW(cw)
}

func embStrong(emb *pdu.EmbeddedSignalling, cc enums.ColorCode) bool {
	return !emb.FEC.Uncorrectable && emb.FEC.ErrorsCorrected == 0 && emb.ColorCode == cc && emb.LCSS != enums.LCSSSingleFragment
}

func embWeak(emb *pdu.EmbeddedSignalling, known bool, cc enums.ColorCode) bool {
	return !emb.FEC.Uncorrectable && emb.FEC.ErrorsCorrected <= 1 && (!known || emb.ColorCode == cc) && emb.LCSS != enums.LCSSSingleFragment
}

// embPlaced reports whether emb reads with no bit corrected as the EMB of
// voice burst B or E, the bursts whose LCSS gives their place in the
// superframe (ETSI TS 102 361-1 clause 9.1.2 and annex B.2.1).
func embPlaced(emb *pdu.EmbeddedSignalling) bool {
	return !emb.FEC.Uncorrectable && emb.FEC.ErrorsCorrected == 0 && (emb.LCSS == enums.LCSSFirstFragment || emb.LCSS == enums.LCSSLastFragment)
}

func (r *leanCore) embPosition(b *Burst) (enums.VoiceBurstType, bool) {
	cc, ok := r.colorCode()
	if !ok {
		return 0, false
	}
	emb := embOf(&b.Data)
	if emb.FEC.Uncorrectable || emb.FEC.ErrorsCorrected > 1 || emb.ColorCode != cc {
		return 0, false
	}
	switch emb.LCSS {
	case enums.LCSSFirstFragment:
		return enums.VoiceBurstB, true
	case enums.LCSSLastFragment:
		return enums.VoiceBurstE, true
	default:
		return 0, false
	}
}
