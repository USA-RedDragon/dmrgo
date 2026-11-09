package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// leanStage is the stage of the stream state machine of the lean core, the
// one place where the core decides to lock, hunt and end a stream. symbol
// runs it once per symbol and slot once per slot. The core starts in
// stageSearch, where each symbol tests the newest SYNC span for a SYNC
// (searchHit). A lock on a SYNC (lockStream) moves it to stageTrack, where
// the slots follow the grid of the lock. At the end of each slot setHunt
// moves it to stageHunt, where each symbol also tests for a SYNC off the
// grid (huntSym), while the lock is stale, the MS talker of the next slot
// has no anchor yet, the clock rate is not yet known, or a voice A SYNC is
// due; and back to stageTrack once none of these holds. A forced lock
// (forceLock) goes straight to stageHunt. The end of the stream (ended)
// moves it back to stageSearch (unlock). leanLock holds the state that the
// transitions read, leanSlots the state of the two talkers of an MS stream,
// and leanStep the state of the level fit across a level step.
type leanStage uint8

const (
	stageSearch leanStage = iota
	stageTrack
	stageHunt
)

// leanLock is the stream lock of the lean core: its stage, and the state
// that the transitions of the stage read.
//
// forced marks a lock that no level fit confirmed: forceLock, decidePend, a
// relock of the hunt on a SYNC off the grid (midSync, huntMatch). An anchor
// after the lock slot or a voice burst whose EMB carries the color code
// clears it; while it holds the lock is stale, and the stream ends 6 slots
// after the lock. unproven marks a lock that a soft stage verified on a weak
// SYNC (searchSym, the ext scan, relock): the color code takes no votes
// until an anchor that the soft stage proves clears it, and a new lock first
// ends the unproven stream of the soft stage. rcLone marks a lock on a
// standalone RC burst; it holds while every anchor is an RC SYNC, and the
// RC bursts must then pass their EMB proof. suspect marks an MS stream whose
// voice burst EMB failed the color code, or whose voice A SYNC or data
// anchor was missed; an anchor clears it, and suspectRun counts the slots it
// has held for the BS rule of stale. reEntry marks the hunt of a slot that
// runs because the clock rate of the stream is not settled. lockDue marks a
// lock that waits for the retune of the soft stage (relock). pendEnd, when
// not 0, is the end of a SYNC found in a held slot of the stream, whose
// decision decidePend takes pendWait symbols later, with its source pendSrc,
// data SYNC pendData and unexplained share pendQ. lockNs is the symbol of
// the lock, and streams counts the locks.
type leanLock struct {
	pendEnd    int64
	lockNs     int64
	pendQ      float32
	streams    int32
	stage      leanStage
	pendWait   uint8
	pendSrc    uint8
	suspectRun uint8
	pendData   bool
	unproven   bool
	rcLone     bool
	forced     bool
	suspect    bool
	reEntry    bool
	lockDue    bool
}

func (r *leanCore) locked() bool { return r.stage != stageSearch }

func (r *leanCore) hunting() bool { return r.stage == stageHunt }

// huntOn starts the hunt of a lock that was forced or that a soft stage
// verified, so that the next SYNC on the grid or off it confirms the lock.
func (r *leanCore) huntOn() {
	if r.stage != stageSearch {
		r.stage = stageHunt
	}
}

// leanReplay is the pass of the lookback replay of the soft stages: the dry
// pass decodes the kept slots before the lock to place the stream, and the
// live pass decodes them again and gives their bursts.
type leanReplay uint8

const (
	replayNone leanReplay = iota
	replayDry
	replayLive
)

func (r *leanCore) lockStream(src int, d, q float32, back int64) {
	if r.dueLock() {
		return
	}
	if r.unproven && r.hasExt() {
		r.ext.end(r)
	}
	if !r.locked() && (!r.hasExt() || r.ext.kept() == nil) {
		r.acqFrom, r.acqHead, r.acqLow = r.n, 0, 0
	}
	r.stage, r.forced, r.suspect, r.src, r.bs = stageTrack, false, false, uint8(src), sourceAt(src) == layer2.SyncSourceBS
	r.suspectRun = 0
	r.pairSrc = 0
	r.lockNs = r.ns
	r.unproven, r.rcLone = false, false
	r.lastGood, r.termAt, r.pendEnd = 0, leanNoSlot, 0
	r.streams++
	r.slotJ = leanNoSlot
	r.first = r.ns - 1 - back - leanSyncEnd
	r.next, r.lastAnch, r.lost = 0, leanNoSlot, 0
	r.ms.anch, r.ms.off, r.ms.moved, r.ms.seek = [2]int32{leanNoSlot, leanNoSlot}, [2]int16{}, false, false
	r.ms.fresh, r.ms.clkAt = [2]uint8{}, 0
	r.slotEnd = r.first + slotSymbols
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.owed, r.tm.fresh = false, r.trackGain, 0, leanClockGain, 0, 1
	r.tm.wake = 0
	r.tm.learned = false
	r.tm.lag0 = float32(back) + (syncSymbols-1)/2
	r.tm.baseN, r.tm.baseC, r.tm.mOK, r.tm.jit = 0, 0, false, 0
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.estS, r.tm.estW, r.tm.estN = 0, 0, 0
	gk := max32(r.devScale, leanMinScale) * deviationHz
	r.tm.pow = 5*gk*gk + r.discVar
	if q > 0 && !r.bs {
		sps := 2 * r.tm.step
		r.tm.measure(0, leanRefitVar*(1-q)/q*sps*sps)
	}
	if r.mmOK && !r.tm.mm {
		r.tm.mm, r.tm.mid = true, false
		r.tm.t += r.tm.step
		r.tm.half = 0
	}
	r.resetChans()
	r.eAcc, r.eSyms = 0, 0
	r.fitNs, r.fitT, r.fitQ = int32(r.ns-1-back), r.strobeTicks(r.lastAt+d-float32(back)*2*r.tm.step), q
	if a := r.audio; a != nil && a.iq && !r.hasExt() && abs32(r.fitHz) > iqHeldHz*iqUnitsPerHz {
		r.heldNs = r.ns
	}
	if r.hasExt() {
		r.ext.locked(r, d)
		r.ext.lockFit(r, r.ns-1-back, r.fitT, q)
	}
}

// forceLock locks, forced, on the SYNC of src that ended back symbols ago,
// at the levels of a fit to it and, with the soft stages, at the timing
// shift that they fit, and starts the hunt that confirms the lock.
func (r *leanCore) forceLock(src int, data bool, back int64) {
	p := syncOf(src, data)
	r.fitLevels(r.ns-1-back, p, true, false)
	d := float32(0)
	if r.hasExt() {
		if t, ok := r.ext.lockShift(r, p, back); ok {
			d = t * 2 * r.tm.step
			r.shiftStrobe(d)
		}
	}
	r.lockStream(src, d, 0, back)
	r.forced = true
	r.huntOn()
}

// decidePend takes the decision on the SYNC found in a held slot, once the
// slot has its whole SYNC field: the slot holds the stream's own burst, or
// the stream locks, forced, on that SYNC.
//
//go:noinline
func (r *leanCore) decidePend() {
	end := r.pendEnd
	r.pendEnd = 0
	if !r.slotHolds(end+int64(r.pendWait)-leanSyncEnd-1, r.pendQ) {
		r.forceLock(int(r.pendSrc), r.pendData, r.ns-1-end)
	}
}

//go:noinline
func (r *leanCore) unlock() {
	if r.hasExt() {
		r.ext.end(r)
	}
	if !r.hunting() || !r.hasExt() || r.ext.kept() == nil {
		r.acqFrom = r.n
		if r.hasExt() && r.ext.kept() != nil {
			r.ext.keptGap(r.n)
		}
	}
	r.stage, r.forced, r.stepHold, r.preStep = stageSearch, false, false, false
	r.snap().ok = false
	if a := r.audio; a != nil {
		a.sum, a.sq, a.n, a.qsum, a.qsq = 0, 0, 0, 0, 0
	}
	r.rebuildSigns()
	r.tm.hold, r.tm.gain, r.tm.rate, r.tm.rateK, r.tm.owed = false, r.acqGain, 0, 0, 0
	r.tm.wake = 0
	r.tm.corr, r.tm.corr2, r.tm.nc, r.tm.nall = 0, 0, 0, 0
	r.tm.estS, r.tm.estW, r.tm.estN = 0, 0, 0
	if r.tm.mm {
		r.tm.mm, r.tm.mid = false, true
		r.tm.t -= r.tm.step
	}
	r.offsetHz = r.tuneHz + r.dcHz
	r.devHz = sat16(3 * deviationHz * r.devScale)
	r.devAtHz = sat16(r.fitHz)
}

func (r *leanCore) setHunt() {
	switch {
	case !r.suspect:
		r.suspectRun = 0
	case r.suspectRun < math.MaxUint8:
		r.suspectRun++
	}
	r.ms.seek, r.ms.retry, r.ms.late = r.seeking(), false, false
	r.reEntry = false
	if r.isReplaying() {
		return
	}
	was := r.hunting()
	if !r.stale() {
		r.stage = stageTrack
		if !r.ms.seek {
			settled := r.bs || r.tm.learned && (!r.hasExt() || r.ext.rateSettled(r))
			if settled && !r.syncDue() {
				return
			}
			r.reEntry = !settled
		}
	}
	if !r.hunting() {
		r.acqFrom = r.n
		if !r.hasExt() || r.ext.kept() == nil {
			r.acqHead, r.acqLow = 0, 0
		} else if !was {
			r.ext.keptGap(r.n)
		}
	}
	r.stage = stageHunt
}

func (r *leanCore) stale() bool {
	if r.forced || r.suspect && (!r.bs || r.suspectRun >= r.bsSuspectRun) {
		return true
	}
	if r.lastAnch == leanNoSlot {
		return r.next > 0
	}
	return r.bs && r.next-r.lastAnch > staleSlots
}

// syncDue reports whether the next slot of an MS stream should carry the
// voice A SYNC of a superframe after the last one. The hunt runs in that slot
// for a SYNC off the grid, so a voice A SYNC that a timing jump moved locks
// again in the slot that carries it.
func (r *leanCore) syncDue() bool {
	j := r.next
	c := &r.ch[j&1]
	return !r.bs && !r.isReplaying() && c.sf != leanNoSlot && j > c.sf && (j-c.sf)%(2*layer2.VoiceBurstsPerSuperframe) == 0
}

func (r *leanCore) seeking() bool {
	p := r.next & 1
	return !r.bs && r.lastAnch != leanNoSlot && (r.ms.anch[p] == leanNoSlot || r.ms.fresh[p] != 0) && !r.isReplaying()
}

func (r *leanCore) ended(j int32, base int64) bool {
	a := r.lastLive()
	return r.lost >= lostSlots || a == leanNoSlot && j >= 6 || j-a >= lostSlots ||
		r.forced && base-r.lockNs > 6*slotSymbols
}

func (r *leanCore) holding() bool {
	return !r.bs && r.lastGood != r.termAt && r.next-1-r.lastGood <= 2*leanFadeBursts+1
}

//go:noinline
func (r *leanCore) symbol(x, at float32) {
	r.lastAt = at
	r.pushSym(x)
	if r.hasExt() {
		r.ext.noteStrobe(r)
	}
	if !r.locked() {
		r.searchHit(r.searchSigns())
		return
	}
	if r.ns&7 == 0 {
		r.clipHot()
	}
	if r.pendEnd != 0 && r.ns >= r.pendEnd+int64(r.pendWait) {
		r.decidePend()
	}
	if r.hunting() && (uint32(r.ns)+leanSeekLead != uint32(r.slotEnd) || !r.seekMid()) {
		r.huntSym(at)
	}
	if r.ns == r.slotEnd {
		if r.front96 {
			r.slotDue = true
			return
		}
		r.endSlot()
	}
}

func (r *leanCore) searchHit(sym, mid bool) {
	switch {
	case r.ns >= leanAcqSymbols && sym:
		r.searchSym(false)
	case r.ns >= leanAcqSymbols && r.rcSigns(r.signs) && r.searchRC():
	case r.ns >= leanRing && r.hasExt():
		if src, data, back, ok := r.ext.scan(r); ok && !r.clipped(r.ns-1-back) {
			r.forceLock(src, data, back)
			r.unproven = true
			r.ext.weakAt(r.ns - 1 - back)
		}
	}
	if mid && !r.locked() && !r.dueLock() && r.ns > leanAcqSymbols {
		r.searchMid()
	}
	if !r.locked() && !r.dueLock() && r.ns > leanAcqSymbols && r.ns >= r.acqNext && r.rcSigns(r.midSigns) && r.lockMid(1, enums.SyncMSSourcedRC) {
		r.rcLone = true
	}
}

//go:noinline
func (r *leanCore) huntSym(at float32) {
	if again, mid, ok := r.huntSigns(at); ok || r.ms.retry {
		r.huntMatch(again, mid)
	}
}

//go:noinline
func (r *leanCore) huntMatch(again, mid bool) {
	if !r.ms.seek && !r.reEntry && !r.offGrid() && !r.stale() {
		return
	}
	if d := r.gridMiss(); r.ms.seek && d >= -leanSeekSyms && d <= leanSeekSyms {
		r.seekMatch(d, again && mid)
		return
	}
	switch {
	case again && mid && r.offGrid():
		r.midSync()
	case again && mid:
		r.retime(true)
	case !r.offGrid():
		r.retime(false)
	default:
		if r.searchSym(true); r.lockNs == r.ns {
			r.forced = true
		}
	}
}

func (r *leanCore) endSlot() {
	lagN := float32(leanClockLag)
	switch {
	case r.tm.mm && r.tm.gain > 0:
		lagN = 2 * r.tm.step / (leanMMGain * r.trackGain * leanMMSlope)
	case r.bs:
		lagN = leanClockLagBS
	}
	r.gearShift(r.tm.settle(lagN))
	first := r.next == 0
	if r.hasExt() && first {
		r.ext.firstSlot(r)
	}
	r.slot()
	if first && r.locked() && r.audio != nil && r.devScale > leanOverScale {
		r.fitScale()
	}
	r.slotJ, r.slotC = leanNoSlot, r.tm.baseC+r.tm.corr
	if r.locked() && (!r.tm.hold || r.tm.wake != 0) {
		r.slotJ = r.next
	}
}

//go:noinline
func (r *leanCore) slot() {
	j := r.next
	r.next++
	base := r.first + int64(j)*slotSymbols + int64(r.ms.off[j&1])
	energy, floor, known, measured := r.slotEnergy(base)
	vp, dp, _ := sourceAt(int(r.src)).SyncPatterns()
	seek := !r.bs && r.lastAnch != leanNoSlot && r.ms.anch[j&1] == leanNoSlot
	r.replaySeek(j, base, seek)
	isAnchor, shift, anchor := r.slotAnchor(base, seek)
	c := &r.ch[j&1]
	sf := c.sf
	if isAnchor {
		base += shift
		if seek {
			r.slotFirst(j, base, anchor)
		}
		r.anchorAt(j, base, shift, anchor, energy)
		r.lockFresh(j, anchor == vp)
	}
	present := isAnchor || r.powered(energy, floor, known, measured)
	noisy := present && !isAnchor && r.eyeNoise(base)
	missedA := !isAnchor && sf != leanNoSlot && j-sf >= 2*layer2.VoiceBurstsPerSuperframe
	if seek && !isAnchor {
		if sh, ok := r.embSeek(j, base, present, known, measured); ok {
			base, present = base+sh, true
		} else {
			present = false
			r.seekNoise(energy, known, measured)
		}
	}
	r.holdNext(j, present)
	lost := r.lost
	switch {
	case present && (isAnchor || r.lastLive() != leanNoSlot && j-r.lastLive() < lostSlots):
		r.lost = 0
	default:
		r.lost++
	}
	heard := present
	if present {
		if !isAnchor {
			anchor = 0
		}
		if heard = r.burst(j, base, anchor, noisy, missedA); !heard {
			r.lost = lost + 1
		} else if !isAnchor && r.syncLive(j) {
			r.lost = 0
		}
	}
	r.undoSlot(j, heard)
	if !r.isReplaying() {
		switch {
		case isAnchor:
			r.suspect = false
		case r.dataAnch[j&1], sf != leanNoSlot && j-sf == 2*layer2.VoiceBurstsPerSuperframe && c.sf != j:
			r.suspect = true
		}
		r.dataAnch[j&1] = isAnchor && anchor == dp
	}
	r.slotSwitch(j)
	r.holdGuard()
	r.slotEnd = r.first + int64(j+2)*slotSymbols + int64(r.ms.off[(j+1)&1])
	r.setHunt()
	if !r.isReplaying() && r.ended(j, base) {
		r.unlock()
	}
}
