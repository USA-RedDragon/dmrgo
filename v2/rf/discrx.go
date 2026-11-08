package rf

import (
	"math"
	"unsafe"
)

const (
	discInputRMSHz = 2000
	discMeanGain   = 0.1
	audio96Span    = discSpan
	halfLag        = halfTaps/2 - 1
	audio12Taps    = discSpan*discSPS + 1
	audio96Taps    = 4*audio96Span + 1
	discLevelUp    = 1.41
	frontQuarter   = frontBlock / 4
	heldStep       = 8
	discEdgeRatio  = 16
	discIQMaxGain  = 1
	discEdgeHot    = 0.5
	audioMaxK      = 1<<16 - 1
	audioMaxGain   = audioMaxK * rrcStepHz / 65536.0
	discMeanJump   = 2 * discInputRMSHz
	discIQJump     = 0.75 * discInputRMSHz
	discIQJumpVar  = 4 * discInputRMSHz * discInputRMSHz
	audioMaxOff    = 1<<29 - 1<<15
	audioStartGain = math.MaxInt8 * rrcStepHz / -float32(math.MinInt16)
)

type discSetup struct {
	Hz      int
	OnBurst func(*Burst)
	Tier    Tier
	opt     *options
	lowIQ   bool
	iq      bool
	pol     bool
}

type discRx struct {
	leanCore
	f audioFront
}

func (r *discRx) setup(cfg discSetup, dec bool) {
	opt := disc0Options()
	switch {
	case cfg.opt != nil:
		opt = *cfg.opt
	case cfg.lowIQ:
		opt = iqLowOptions()
	}
	sink := r.sink
	*r = discRx{}
	r.sink = sink
	r.f.dec = dec
	r.f.half = cfg.Hz == discRate
	r.f.slow = cfg.Hz == 2*symbolRate
	sps := float32(discSPS) / 2
	if r.f.slow {
		sps = 2
	}
	r.init(cfg.OnBurst, opt, sps)
	r.front96 = opt.Front96 && r.f.slow && r.tm.cubic
	switch {
	case r.front96 && opt.RawSearch == 1:
		r.mid96 = mid96Raw
	case r.front96 && opt.RawSearch >= 2:
		r.mid96 = mid96RawLock
	case r.front96 && opt.LinearMid:
		r.mid96 = mid96Linear
	}
	r.f.gain, r.f.up = audioStartGain, opt.LevelUp
	r.f.idleHold = uint8((2*slotSymbols-burstSymbols)*r.hz()/(symbolRate*frontBlock) + 1)
	if cfg.iq || cfg.lowIQ {
		r.f.gain, r.f.maxGain, r.f.idleHold, r.f.leveled, r.f.iq = 1, discIQMaxGain, 1, true, true
	}
	if cfg.iq && opt.CohExt {
		r.f.gain = float32(discInputRMSHz / (iqUnitsPerHz * deviationHz * math.Sqrt(5)))
	}
	if cfg.pol {
		r.f.polBits = polOn
	}
	r.f.cold = !cfg.iq && !cfg.lowIQ
	r.f.refresh()
	r.audio, r.center = &r.f, -int64(discSpan*samplesPerSy)
	if r.f.slow {
		r.center = -int64(audio96Span * samplesPerSy)
	}
	if r.f.half {
		r.center -= halfTaps - 1
		r.lagTicks = (2*halfLag - (halfTaps - 1)) << 8
	}
	if dec {
		r.lagTicks += halfLag << 8
	}
}

func (r *discRx) hz() int {
	switch {
	case r.f.half:
		return discRate
	case r.f.slow:
		return 2 * symbolRate
	}
	return discRate / 2
}

func (r *discRx) maxBursts(n int) int {
	return 4*r.inputSamples(n)*symbolRate/(slotSymbols*r.inputHz()) + 16
}

func (r *discRx) flushBursts() int {
	return r.maxBursts(slotSymbols * discSPS)
}

func (r *discRx) flush() {
	r.markEnd()
	var idle [16]int16
	r.idle(&idle)
	for range slotSymbols * discSPS &^ 63 / len(idle) {
		r.write(idle[:])
	}
	if r.locked {
		r.unlock()
	}
	r.endTicks = 0
}

func (r *discRx) flushIQ() {
	r.markEnd()
	var idle [16]int16
	r.idle(&idle)
	for range slotSymbols * discSPS &^ 63 / len(idle) {
		r.writeIQ(idle[:])
	}
	if r.locked {
		r.unlock()
	}
	r.endTicks = 0
}

func (r *discRx) idle(idle *[16]int16) {
	v := int16(max32(min32(r.f.dc/r.f.gain, 32767), -32768))
	for i := range idle {
		idle[i] = v
	}
}

func (r *discRx) run(x []int16) {
	for r.f.cold && len(x) > 0 {
		x = x[r.warm(x):]
	}
	if len(x) > 0 {
		r.runN(x)
	}
}

func (r *discRx) warm(x []int16) int {
	f := &r.f
	held := r.huntBuf[:frontQuarter]
	n := copy(held[f.nHeld:], x)
	f.nHeld += uint8(n)
	if int(f.nHeld) < len(held) {
		return n
	}
	const h = frontQuarter / 2
	m0, v0 := heldStats(held[:h])
	m1, v1 := heldStats(held[h:])
	switch {
	case v1 > discEdgeRatio*v0 && !f.edge:
		e := quietEnd(held[:h], v1)
		if e == 0 {
			break
		}
		f.ran, f.edge = true, true
		f.quiet, _ = heldStats(held[:e])
		r.runN(held[:e])
		copy(held, held[e:])
		f.nHeld = uint8(len(held) - e)
		return n
	case v1 == 0:
		f.ran, f.edge, f.quiet = true, false, m1
		r.runN(held)
		f.nHeld = 0
		return n
	}
	m, v := (m0+m1)/2, max32(v0, v1)
	if v0 > discEdgeRatio*v1 {
		m = m0
	}
	f.cold, f.nHeld, f.leveled = false, 0, true
	g := discInputRMSHz / (sqrtf(v) * f.gain)
	if u := g * f.gain; u >= 0.5 && u <= 2 && (!f.up || u <= discLevelUp) {
		g = 1 / f.gain
	}
	r.scaleAudio(g)
	f.mean = m * f.gain
	if d := f.mean - f.dc; abs32(d) > leanSearchHz {
		r.retune(d)
	}
	var q int8
	if f.ran {
		q = sat8((f.quiet*f.gain - f.dc) / rrcStepHz)
	} else {
		tm := &r.tm
		tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.last, tm.half = 0, 0, 0, 0, 0, 0, 0, 0
	}
	for i := range f.rrcHist {
		f.rrcHist[i] = q
	}
	for i := range f.hbHist {
		f.hbHist[i] = q
	}
	r.runN(held)
	return n
}

func quietEnd(x []int16, loud float32) int {
	for k := 0; k+heldStep <= len(x); k += heldStep {
		if _, v := heldStats(x[k : k+heldStep]); discEdgeRatio*v > loud {
			return k
		}
	}
	return len(x)
}

func heldStats(x []int16) (m, v float32) {
	var sum int32
	for _, u := range x {
		sum += int32(u)
	}
	m = float32(sum) / float32(len(x))
	for _, u := range x {
		d := float32(u) - m
		v += d * d
	}
	return m, v / float32(len(x))
}

//nolint:unparam // writeN reads the count outside TinyGo.
func (r *discRx) runN(x []int16) int {
	if !r.front96 {
		n := r.runAll(x)
		r.polarity()
		return n
	}
	n := len(x)
	for {
		i, _ := r.run96(x, 0, nil)
		x = x[i:]
		if !r.slotDue {
			r.polarity()
			return n
		}
		r.slotDue = false
		r.endSlot()
	}
}

//go:noinline
func (r *discRx) runAll(x []int16) int {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	var dn int32
	kh, kq := r.acqHead, r.acqLow
	buf, an := r.acqBuf()
	cubic, half, slow := tm.cubic, f.half, f.slow
	search, keep := !r.locked, !r.locked || r.hunt
	fit := r.hasExt()
	r.startRaw(0)
	for i, v := range x {
		if search {
			f.sum += int32(v)
			f.sq += int64(v) * int64(v)
			if f.n++; f.n&(frontQuarter-1) == 0 {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.acqHead, r.acqLow = kh, kq
				r.keepRaw(x, i)
				r.levels()
				sa, sb, s0, s1, s2, s3, t = tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
				kq = r.acqLow
			}
		}
		var acc int32
		switch u := f.quantize(v); {
		case slow:
			acc = f.mf96(u)
		case half:
			w, ok := f.halfBand(u)
			if !ok {
				continue
			}
			acc = f.mf12(w)
		default:
			acc = f.mf12(u)
		}
		y := float32(acc) * (rrcStepHz / 32768.0)
		dn++
		sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
		if keep {
			q := int16(keptQ(acc))
			if kh&1 == 0 {
				kq = q
			} else if i := kh >> 1; i < int32(len(buf)) {
				buf[i] = int16(uint16(uint8(kq)) | uint16(uint8(q))<<8)
			}
			if kh++; kh == an {
				kh = 0
			}
		}
		if fit {
			r.ext.keepFit(keptQ(acc))
		}
		t--
		for t <= 0 {
			at := t
			var xv float32
			if cubic {
				xv = cubicAt(tm.six, sa, sb, s0, s1, s2, s3, 1+at)
				at--
			} else {
				xv = s3 + at*(s3-s2)
			}
			tm.t = t
			sym := tm.strobe(xv, r.off)
			t = tm.t
			if sym {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3 = sa, sb, s0, s1, s2, s3
				r.n += int64(dn)
				dn = 0
				r.acqHead, r.acqLow = kh, kq
				r.symbol(tm.last, at)
				sa, sb, s0, s1, s2, s3, t, kq = tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.acqLow
				kh = r.acqHead
				buf, an = r.acqBuf()
				search, keep = !r.locked, !r.locked || r.hunt
				if r.dueLock() {
					r.keepRaw(x, i+1)
					return i + 1
				}
			}
		}
	}
	tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
	r.n += int64(dn)
	r.acqHead, r.acqLow = kh, kq
	r.keepRaw(x, len(x))
	return len(x)
}

const (
	run96Strobe = 0
)

const (
	st96Search = 1 << iota
	st96Keep
	st96Raw
	st96Lock
	st96Quiet
	st96LinMid
)

func (r *discRx) state96() uint8 {
	var st uint8
	mode := r.mid96
	if mode == mid96Linear {
		st |= st96LinMid
	}
	if !r.locked {
		st |= st96Search | st96Keep
		if mode >= mid96Raw {
			st |= st96Raw
		}
		return st
	}
	if r.hunt {
		st |= st96Keep
	}
	if mode == mid96RawLock {
		st |= st96Lock
		if !r.hunt {
			st |= st96Quiet
		}
	}
	return st
}

//go:noinline
//go:section .text.run96
func (r *discRx) run96(x []int16, i0 int, in []uint32) (int, int) {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	fed := 0
	n := len(x)
	kh, kq := r.acqHead, r.acqLow
	buf, an := r.acqBuf()
	st := r.state96()
	fit := r.hasExt()
	r.startRaw(i0)
	k, off := f.k, f.off
	hist := &f.rrcHist
	h := f.rrcHead
	if h >= audio96Taps {
		h = 0
	}
	for i := i0; ; {
		if t > 0 {
			for i != n {
				v := *(*int16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(x)), 2*i))
				if st&st96Search != 0 {
					f.sum += int32(v)
					f.sq += int64(v) * int64(v)
					if f.n++; f.n&(frontQuarter-1) == 0 {
						tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
						r.acqHead, r.acqLow = kh, kq
						f.rrcHead = h
						r.keepRaw(x, i)
						r.levels()
						sb, s0, s1, s2, s3, t = tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
						kq = r.acqLow
						k, off = f.k, f.off
					}
				}
				u := int8(max(min((int32(v)*k>>1-off)>>15, 127), -128))
				p := unsafe.Add(unsafe.Pointer(hist), h)
				*(*int8)(p), *(*int8)(unsafe.Add(p, audio96Taps)) = u, u
				acc := fir96[site0]((*[audio96Taps]int8)(unsafe.Add(p, 1)))
				if h++; h >= audio96Taps {
					h = 0
				}
				y := float32(acc) * (rrcStepHz / 32768.0)
				sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
				if fit {
					r.ext.keepFit(keptQ(acc))
				}
				if st&st96Keep != 0 {
					q := int16(keptQ(acc))
					if kh&1 == 0 {
						kq = q
					} else if uint32(kh) < uint32(an) {
						*(*int16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(buf)), kh&^1)) = int16(uint16(uint8(kq)) | uint16(uint8(q))<<8)
					}
					if kh++; kh == an {
						kh = 0
					}
				}
				t--
				i++
				if t <= 0 {
					goto strobe
				}
			}
			if !r.iqLow {
				break
			}
			r.n += int64(i - i0)
			i0 = i
			m := iqLowOf(r).fill(in[fed:])
			if m == 0 {
				break
			}
			fed += m
			n, i, i0 = m, 0, 0
			continue
		}
	strobe:
		for {
			if st&st96Raw != 0 {
				t = tm.step
				if tm.mid {
					tm.half, tm.mid = s2, false
					break
				}
				tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*s2*s2
				tm.last, tm.mid = s2, true
				hit, sym, mid := r.rawSym(s2)
				if !hit {
					break
				}
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.n += int64(i - i0)
				i0 = i
				r.acqHead, r.acqLow = kh, kq
				f.rrcHead = h
				r.lastAt = -1
				r.searchHit(sym, mid)
				sa, sb, s0, s1, s2, s3, t, kq = tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.acqLow
				kh = r.acqHead
				buf, an = r.acqBuf()
				st = r.state96()
				k, off = f.k, f.off
				if t > 0 {
					break
				}
				continue
			}
			at := t
			mu := 1 + at
			var xv float32
			switch {
			case tm.mid && st&st96LinMid != 0:
				xv = s1 + mu*(s2-s1)
			case run96Strobe == 1:
				xv = s1 + 0.5*mu*(s2-s0+mu*(2*s0-5*s1+4*s2-s3+mu*(3*(s1-s2)+s3-s0)))
			case run96Strobe == 2:
				xv = s1 + mu*(s2-s1)
			case run96Strobe == 3:
				c1 := s2 - (1.0/3)*s0 - 0.5*s1 - (1.0/6)*s3
				c2 := 0.5*(s0+s2) - s1
				c3 := (1.0/6)*(s3-s0) + 0.5*(s1-s2)
				xv = s1 + mu*(c1+mu*(c2+mu*c3))
			default:
				xv = lagrange6[site0](sa, sb, s0, s1, s2, s3, mu)
			}
			at--
			sym := true
			switch {
			case tm.mm:
				yv := xv - tm.dc
				d := tm.dev
				if abs32(yv) > 2*tm.dev {
					d = 3 * tm.dev
				}
				if yv < 0 {
					d = -d
				}
				e := leanMMSign * (tm.half*yv - d*(tm.last-tm.dc))
				out := abs32(xv-r.off) > leanRingRail
				if tm.stale || out {
					tm.stale, e = out, 0
				}
				if !tm.hold && !out {
					tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*yv*yv
				}
				tm.last, tm.half = xv, d
				t += 2*tm.step + tm.rate
				tm.nall++
				tm.lastC = 0
				if !tm.hold && tm.pow > 0 {
					c := max32(min32(leanMMGain*tm.gain*e/tm.pow, 0.5), -0.5)
					t += c
					tm.lastC = c
					tm.corr, tm.corr2, tm.nc = tm.corr+c, tm.corr2+c*c, tm.nc+1
				}
			case tm.mid:
				tm.half, tm.mid = xv, false
				t += tm.step
				sym = false
			default:
				if !tm.hold {
					tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*xv*xv
				}
				e := tm.half * (tm.last - xv)
				if tm.stale {
					tm.stale, e = false, 0
				}
				tm.last, tm.mid = xv, true
				t += tm.step
				tm.lastC = 0
				if !tm.hold && tm.pow > 0 {
					tm.lastC = max32(min32(tm.gain*e/tm.pow, 0.5), -0.5)
					t += tm.lastC
				}
			}
			if sym && st&st96Quiet != 0 {
				if r.lockPush(tm.last, at) {
					if t > 0 {
						break
					}
					continue
				}
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.n += int64(i - i0)
				r.acqHead, r.acqLow = kh, kq
				f.rrcHead = h
				r.slotDue = true
				r.keepRaw(x, i)
				return i, fed
			}
			if sym {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.n += int64(i - i0)
				i0 = i
				r.acqHead, r.acqLow = kh, kq
				if st&st96Lock != 0 {
					again, mid, match := r.lockSym(tm.last, at)
					if !match && !r.ms.retry && uint32(r.ns) != uint32(r.slotEnd) && (uint32(r.ns)+leanSeekLead != uint32(r.slotEnd) || !r.hunt || !r.seekMid()) {
						if t > 0 {
							break
						}
						continue
					}
					f.rrcHead = h
					if match || r.ms.retry {
						r.huntMatch(again, mid)
					}
					if uint32(r.ns) == uint32(r.slotEnd) {
						r.slotDue = true
					}
				} else {
					f.rrcHead = h
					r.symbol(tm.last, at)
				}
				if r.slotDue {
					r.keepRaw(x, i)
					return i, fed
				}
				sa, sb, s0, s1, s2, s3, t, kq = tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.acqLow
				kh = r.acqHead
				buf, an = r.acqBuf()
				st = r.state96()
				k, off = f.k, f.off
			}
			if t > 0 {
				break
			}
		}
	}
	f.rrcHead = h
	tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
	r.n += int64(n - i0)
	r.acqHead, r.acqLow = kh, kq
	r.keepRaw(x, n)
	return n, fed
}

const coreLagSeconds = 0.01

func (r *leanCore) lagSeconds() float64 {
	if r.hasExt() {
		return coreLagSeconds + extQueue*slotSeconds
	}
	return coreLagSeconds
}

func (r *discRx) lagSamples(rate int) int {
	return int(math.Ceil((r.lagSeconds() + r.fixLag()) * float64(rate)))
}

func (r *discRx) offset() float64 {
	return float64(r.offsetHz / r.f.gain)
}

func (r *discRx) deviation() float64 {
	return float64(float32(r.devHz) / r.f.gain)
}

// jumpDC moves the DC of the front to the mean qm of the last quarter block
// when it is off by more than discIQJump and the variance qv is that of a
// signal, not of noise. Before the lock the mean of the blocks follows a
// carrier step at the start of a burst too slowly, and at I/Q tier 0, which
// times the first burst before the AFC tunes, the DC left in the kept samples
// clipped them at the sync 2 kHz or more off. A quarter that a converter tune
// split is skipped.
//
//go:noinline
func (r *discRx) jumpDC(qm, qv float32) {
	f := &r.f
	m := qm * f.gain
	if r.tuneNs != 0 && uint32(r.ns)-r.tuneNs <= frontQuarter/2 || qv*f.gain*f.gain >= discIQJumpVar || abs32(m-f.dc) <= discIQJump {
		return
	}
	f.mean = m
	r.moveDC(m - f.dc)
}

func (r *discRx) levels() {
	f := &r.f
	qm := float32(f.sum-f.qsum) / frontQuarter
	qv := sq32(f.sq-f.qsq)/frontQuarter - qm*qm
	f.qsum, f.qsq = f.sum, f.sq
	r.quarterHot(qv)
	if f.iq && !r.hasExt() {
		r.jumpDC(qm, qv)
	}
	if f.n == frontQuarter {
		f.qmin, f.qmax = qv, qv
	} else {
		f.qmin, f.qmax = min32(f.qmin, qv), max32(f.qmax, qv)
	}
	if f.n != frontBlock {
		return
	}
	m := float32(f.sum) / frontBlock
	v := sq32(f.sq)/frontBlock - m*m
	edge := f.qmax > discEdgeRatio*f.qmin
	f.sum, f.sq, f.n, f.qsum, f.qsq = 0, 0, 0, 0, 0
	m *= f.gain
	v *= f.gain * f.gain
	if f.jumps(m) {
		f.mean = m
	} else {
		f.mean += discMeanGain * (m - f.mean)
	}
	f.idle = min(f.idle+1, f.idleHold)
	if rms := sqrtf(max32(v, 0)); rms > 0 {
		g := discInputRMSHz / rms
		quiet := !edge && g*g > discEdgeRatio
		if !quiet {
			f.idle = 0
		}
		switch {
		case quiet && f.leveled && f.idle < f.idleHold:
		case !edge && !f.leveled:
			f.idle, f.leveled = 0, true
			if u := g * f.gain; u >= 0.5 && u <= 2 && (!f.up || u <= discLevelUp) {
				g = 1 / f.gain
			}
			r.scaleAudio(g)
		case !edge && (g > 2 || g < 0.5 || f.up && g > discLevelUp):
			f.idle = 0
			if f.maxGain > 0 {
				g = min32(g, f.maxGain/f.gain)
			}
			if g != 1 {
				g = r.scaleAudio(g)
				if mg := m * g; f.jumps(mg) {
					f.mean = mg
				}
			}
		case edge:
			if hot := discInputRMSHz / (sqrtf(max32(f.qmax, 0)) * abs32(f.gain)); hot < discEdgeHot {
				r.scaleAudio(hot)
			}
		}
	}
	if d := f.mean - f.dc; abs32(d) > leanSearchHz {
		r.retune(d)
	}
}

const (
	snapHistAt = 2 * frontQuarter
	snapAt     = (snapHistAt + 2*audio12Taps + halfTaps + 3) &^ 3
	_          = uint(leanHuntSamples - snapAt - unsafe.Sizeof(frontSnap{}))
)

type frontSnap struct {
	kh      int32
	low     int16
	rrc, hb uint8
	seg     uint16
	rn      uint8
	odd, ok bool
}

func (r *leanCore) snap() *frontSnap {
	return (*frontSnap)(unsafe.Add(unsafe.Pointer(&r.huntBuf), snapAt))
}

func (r *leanCore) startRaw(i int) {
	if f := r.audio; audioOn() && f.maxGain == 0 && !f.cold && !r.locked {
		r.snap().seg = uint16(i)
	}
}

func (r *leanCore) keepRaw(x []int16, i int) {
	f := r.audio
	if !audioOn() || f.maxGain != 0 || f.cold || r.locked {
		return
	}
	sn := r.snap()
	if j := int(sn.seg); sn.ok && sn.rn <= frontQuarter && j <= i {
		sn.rn += uint8(copy(r.rawRing()[sn.rn:], x[j:i]))
	}
	sn.seg = uint16(i)
}

func (r *leanCore) rawRing() *[frontQuarter]int16 {
	return (*[frontQuarter]int16)(unsafe.Pointer(&r.huntBuf))
}

func (r *leanCore) snapHist() (*[2 * audio12Taps]int8, *[halfTaps]int8) {
	p := unsafe.Add(unsafe.Pointer(&r.huntBuf), snapHistAt)
	return (*[2 * audio12Taps]int8)(p), (*[halfTaps]int8)(unsafe.Add(p, 2*audio12Taps))
}

func (r *leanCore) quarterHot(qv float32) {
	f := r.audio
	if !audioOn() || f.maxGain != 0 {
		return
	}
	if f.cold {
		r.snap().ok = false
		return
	}
	if g := discInputRMSHz / (sqrtf(max(qv, 0)) * abs32(f.gain)); r.snap().ok && f.leveled && qv > 0 && g < discEdgeHot {
		r.scaleAudio(g)
	}
	rrc, hb := r.snapHist()
	*rrc, *hb = f.rrcHist, f.hbHist
	sn := r.snap()
	*sn = frontSnap{kh: r.acqHead, low: r.acqLow, rrc: uint8(f.rrcHead), hb: uint8(f.hbHead), seg: sn.seg, odd: f.odd, ok: true}
}

func (r *leanCore) refilter(g float32) bool {
	f := r.audio
	if sn := r.snap(); r.locked || !sn.ok || sn.rn > frontQuarter {
		return false
	}
	rrc, hb := r.snapHist()
	for i, h := range rrc {
		rrc[i] = scaleSat8(h, g)
	}
	for i, h := range hb {
		hb[i] = scaleSat8(h, g)
	}
	sn := r.snap()
	sn.low = int16(scale8(int32(sn.low), int32(g*256+0.5), 0))
	f.rrcHist, f.hbHist = *rrc, *hb
	f.rrcHead, f.hbHead, f.odd = int(sn.rrc), int(sn.hb), sn.odd
	buf, an := r.acqBuf()
	kh, kq := sn.kh, sn.low
	for _, v := range r.rawRing()[:sn.rn] {
		var acc int32
		switch u := f.quantize(v); {
		case f.slow:
			acc = f.mf96(u)
		case f.half:
			w, ok := f.halfBand(u)
			if !ok {
				continue
			}
			acc = f.mf12(w)
		default:
			acc = f.mf12(u)
		}
		q := int16(keptQ(acc))
		if kh&1 == 0 {
			kq = q
		} else if i := kh >> 1; i < int32(len(buf)) {
			buf[i] = int16(uint16(uint8(kq)) | uint16(uint8(q))<<8)
		}
		if kh++; kh == an {
			kh = 0
		}
	}
	r.acqLow = kq
	return true
}

func sq32(x int64) float32 {
	return float32(uint32(x>>32))*(1<<32) + float32(uint32(x))
}

func (r *leanCore) scaleAudio(g float32) float32 {
	f := r.audio
	if g > 0 {
		if g = min32(g, audioMaxGain/abs32(f.gain)); abs32(g-1) < 1e-6 {
			return 1
		}
	}
	f.gain *= g
	f.mean *= g
	f.dc *= g
	r.tuneHz *= g
	r.off *= g
	r.offsetHz *= g
	r.devHz = sat16(float32(r.devHz) * g)
	f.refresh()
	r.rescale(g)
	if !audioOn() || f.maxGain != 0 || !r.refilter(g) {
		f.scale(g)
	}
	return g
}

type audioFront struct {
	sq       int64
	qsq      int64
	qsum     int32
	qmin     float32
	qmax     float32
	gain     float32
	maxGain  float32
	dc       float32
	k        int32
	off      int32
	sum      int32
	n        int
	mean     float32
	rrcHead  int
	hbHead   int
	rrcHist  [2 * audio12Taps]int8
	hbHist   [halfTaps]int8
	half     bool
	slow     bool
	odd      bool
	up       bool
	dec      bool
	leveled  bool
	iq       bool
	idle     uint8
	idleHold uint8
	polSum   int8
	polBits  uint8
	pol      discPol
	cold     bool
	ran      bool
	edge     bool
	nHeld    uint8
	quiet    float32
}

func (f *audioFront) jumps(m float32) bool {
	return audioOn() && f.maxGain == 0 && abs32(m-f.mean) > discMeanJump
}

func (f *audioFront) refresh() {
	if k := f.gain / rrcStepHz * 65536; k < 0 {
		f.k = -int32(min32(0.5-k, audioMaxK))
	} else {
		f.k = int32(max32(min32(k+0.5, audioMaxK), 0))
	}
	f.off = int32(max32(min32(f.dc/rrcStepHz*32768, audioMaxOff), -audioMaxOff)) - 16384
}

func (f *audioFront) scale(g float32) {
	for i, h := range &f.rrcHist {
		f.rrcHist[i] = scaleSat8(h, g)
	}
	for i, h := range &f.hbHist {
		f.hbHist[i] = scaleSat8(h, g)
	}
}

func scaleSat8(h int8, g float32) int8 {
	if h == math.MaxInt8 || h == math.MinInt8 {
		return h
	}
	return sat8(float32(h) * g)
}

func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	f.refresh()
	f.shiftHist(d)
}

func (f *audioFront) shiftHist(d float32) {
	for i, v := range &f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
	for i, v := range &f.hbHist {
		f.hbHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
}

func keptQ(acc int32) int8 {
	return int8(max(min((acc+1<<13)>>14, math.MaxInt8), math.MinInt8))
}

func (f *audioFront) quantize(u int16) int8 {
	return int8(max(min((int32(u)*f.k>>1-f.off)>>15, 127), -128))
}

func (f *audioFront) halfBand(v int8) (int8, bool) {
	h := f.hbHead
	f.hbHist[h] = v
	if h++; h == halfTaps {
		h = 0
	}
	f.hbHead = h
	if f.odd = !f.odd; f.odd {
		return 0, false
	}
	const c = halfTaps / 2
	acc := int32(discHalfQ15[0]) * int32(f.hbHist[ringAt(h, c)])
	for k, t := range discHalfQ15[1:] {
		acc += int32(t) * (int32(f.hbHist[ringAt(h, c-2*k-1)]) + int32(f.hbHist[ringAt(h, c+2*k+1)]))
	}
	return int8(max(min((acc+1<<14)>>15, 127), -128)), true
}

func (f *audioFront) mf96(v int8) int32 {
	h := f.rrcHead
	if h >= audio96Taps {
		h = 0
	}
	f.rrcHist[h], f.rrcHist[h+audio96Taps] = v, v
	if h++; h == audio96Taps {
		h = 0
	}
	f.rrcHead = h
	return fir96[site1]((*[audio96Taps]int8)(f.rrcHist[h : h+audio96Taps]))
}

func (f *audioFront) mf12(v int8) int32 {
	h := f.rrcHead
	if h >= audio12Taps {
		h = 0
	}
	f.rrcHist[h], f.rrcHist[h+audio12Taps] = v, v
	if h++; h == audio12Taps {
		h = 0
	}
	f.rrcHead = h
	return fir12((*[audio12Taps]int8)(f.rrcHist[h : h+audio12Taps]))
}

func ringAt(h, k int) int {
	if k += h; k >= halfTaps {
		k -= halfTaps
	}
	return k
}

func fir12(w *[audio12Taps]int8) int32 {
	return int32(audioRRCQ15[10])*int32(w[10]) +
		int32(audioRRCQ15[0])*(int32(w[0])+int32(w[20])) +
		int32(audioRRCQ15[1])*(int32(w[1])+int32(w[19])) +
		int32(audioRRCQ15[2])*(int32(w[2])+int32(w[18])) +
		int32(audioRRCQ15[3])*(int32(w[3])+int32(w[17])) +
		int32(audioRRCQ15[4])*(int32(w[4])+int32(w[16])) +
		int32(audioRRCQ15[5])*(int32(w[5])+int32(w[15])) +
		int32(audioRRCQ15[6])*(int32(w[6])+int32(w[14])) +
		int32(audioRRCQ15[7])*(int32(w[7])+int32(w[13])) +
		int32(audioRRCQ15[8])*(int32(w[8])+int32(w[12])) +
		int32(audioRRCQ15[9])*(int32(w[9])+int32(w[11]))
}
