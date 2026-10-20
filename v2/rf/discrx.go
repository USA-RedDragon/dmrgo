package rf

import (
	"math"
	"unsafe"
)

// Layout of the discriminator front end.
const (
	// discInputRMSHz is the RMS, in Hz, to which a DiscReceiver scales its
	// input, and discMeanGain the weight of each block in its mean.
	discInputRMSHz = 2000
	discMeanGain   = 0.1
	// audio12Taps and audio96Taps are the lengths of the matched filters at
	// 12 kHz (2.5 samples per symbol), discSpan symbols on each side, and
	// 9.6 kHz (2 samples per symbol), audio96Span symbols on each side.
	audio96Span = discSpan
	audio12Taps = discSpan*discSPS + 1
	audio96Taps = 4*audio96Span + 1
	// discLevelUp is the least ratio of discInputRMSHz to the input RMS
	// at which the level control scales the input up with
	// options.LevelUp.
	discLevelUp = 1.41
	// audioMaxK and audioMaxOff limit audioFront.k and off, to gains of
	// 64 Hz per input unit and offsets of 2 MHz.
	audioMaxK   = 1<<15 - 1
	audioMaxOff = 1<<30 - 1<<16
)

// DiscConfig sets up a DiscReceiver.
type DiscConfig struct {
	// Hz is the sample rate of the discriminator samples of Write: 24000,
	// as MMDVM modems sample, 12000 or 9600. 0 means 24000. A lower rate
	// costs less: the receiver halves 24 kHz with a half-band filter before
	// its matched filter, which runs at 12 kHz or 9.6 kHz.
	Hz int
	// MaxWrite is the largest number of samples in one Write, for the
	// burst buffer that NewDisc allocates. 0 means 1 << 12.
	MaxWrite int
	// OnBurst, when not nil, takes each burst as the receiver recovers it,
	// in place of the slices that Write, AppendBursts and Flush return,
	// which then return no bursts. The burst is valid only during the call;
	// copy it to keep it.
	OnBurst func(*Burst)
	// Tier is Tier0, Tier1 or Tier2. The zero value TierDefault is Tier1
	// for NewDisc, and Tier0 for Init, which does not allocate the stages
	// of the higher tiers, and under TinyGo, which has neither. Tier2
	// detects each burst again coherently, on the carrier phase that it
	// rebuilds from the samples, in place of the detection of Tier1, at
	// about 15 times its CPU time.
	Tier Tier
	// opt, when not nil, replaces the settings of the tier.
	opt *options
}

// DiscReceiver recovers DMR bursts from the output of an FM discriminator,
// as the radio chips of hotspots and MMDVM modems give it: real samples in
// any units, with any gain and DC offset, which the receiver measures
// itself. With Tier0 it keeps about 1 KiB of state, for microcontrollers. A DiscReceiver that Init sets up in
// memory of the caller, such as a package variable, uses no heap with
// OnBurst or AppendBursts. A DiscReceiver is not safe for concurrent use.
type DiscReceiver struct {
	leanCore
	f audioFront
}

// The errors of Init. Each is a static value, so that Init does not
// allocate.
var (
	errDiscHz       = &ConfigError{Field: "Hz"}
	errDiscMaxWrite = &ConfigError{Field: "MaxWrite"}
	errDiscTier     = &ConfigError{Field: "Tier"}
)

// NewDisc returns a DiscReceiver of cfg. It allocates the receiver, and
// the burst buffer of Write when cfg.OnBurst is nil.
func NewDisc(cfg DiscConfig) (*DiscReceiver, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	r := &DiscReceiver{}
	tier := cfg.Tier
	soft := (tier == Tier1 || tier == Tier2) && cfg.opt == nil
	if soft {
		cfg.Tier = Tier0
	}
	if err := r.Init(cfg); err != nil {
		return nil, err
	}
	if soft {
		if err := r.addSoft(tier); err != nil {
			return nil, err
		}
	}
	if cfg.OnBurst == nil {
		if cfg.MaxWrite == 0 {
			cfg.MaxWrite = 1 << 12
		}
		r.out = make([]Burst, 0, r.maxBursts(cfg.MaxWrite))
	}
	return r, nil
}

// Init sets up r for cfg, in place of any state it holds. It does not
// allocate, so that r can live in memory of the caller.
func (r *DiscReceiver) Init(cfg DiscConfig) error {
	if cfg.Hz == 0 {
		cfg.Hz = discRate
	}
	if cfg.Hz != discRate && cfg.Hz != discRate/2 && cfg.Hz != 2*symbolRate {
		return errDiscHz
	}
	if cfg.MaxWrite < 0 {
		return errDiscMaxWrite
	}
	if cfg.Tier != TierDefault && cfg.Tier != Tier0 && cfg.opt == nil {
		return errDiscTier
	}
	opt := discOptions()
	if cfg.opt != nil {
		opt = *cfg.opt
	}
	*r = DiscReceiver{}
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
	if r.front96 && opt.StrobeTab {
		r.mid96 |= mid96Tab
	}
	r.f.gain, r.f.up = 1, opt.LevelUp
	r.f.refresh()
	r.audio, r.center = &r.f, -int64(discSpan*samplesPerSy)
	if r.f.slow {
		r.center = -int64(audio96Span * samplesPerSy)
	}
	if r.f.half {
		r.center -= halfTaps - 1
	}
	return nil
}

// hz returns the input rate of r.
func (r *DiscReceiver) hz() int {
	switch {
	case r.f.half:
		return discRate
	case r.f.slow:
		return 2 * symbolRate
	}
	return discRate / 2
}

// growBursts returns dst with room for n more bursts. It grows dst by
// append alone, which the allocation gate follows as growth of dst; each
// append at least doubles the capacity.
func growBursts(dst []Burst, n int) []Burst {
	for cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], Burst{})[:len(dst)]
	}
	return dst
}

// maxBursts returns the most bursts that n input samples complete.
func (r *DiscReceiver) maxBursts(n int) int {
	return 4*n*symbolRate/(slotSymbols*r.inputHz()) + 16
}

// AppendBursts processes the discriminator samples x, appends the bursts
// that are complete to dst and returns the extended slice. It allocates
// only when dst has no room for the bursts that x could complete.
func (r *DiscReceiver) AppendBursts(dst []Burst, x []int16) []Burst {
	dst = growBursts(dst, r.maxBursts(len(x)))
	keep, n := r.out, len(dst)
	// The core fills the room after dst.
	r.out = dst[n:n]
	r.write(x)
	m := len(r.out)
	r.out = keep[:0]
	return dst[:n+m]
}

// Write processes the discriminator samples x and returns the bursts that
// are complete, in the buffer that NewDisc allocates. The returned slice
// is valid until the next call. On a DiscReceiver that Init set up without
// OnBurst, Write allocates its buffer; use AppendBursts.
func (r *DiscReceiver) Write(x []int16) []Burst {
	r.out = r.out[:0]
	r.write(x)
	return r.out
}

// Flush finishes the stream at its end and returns the remaining bursts,
// as Write does.
func (r *DiscReceiver) Flush() []Burst {
	r.out = r.out[:0]
	r.flush()
	return r.out
}

// AppendFlush finishes the stream at its end, appends the remaining bursts
// to dst and returns the extended slice. It allocates only when dst has no
// room for the bursts of one slot.
func (r *DiscReceiver) AppendFlush(dst []Burst) []Burst {
	dst = growBursts(dst, r.maxBursts(slotSymbols*discSPS))
	keep, n := r.out, len(dst)
	// The core fills the room after dst.
	r.out = dst[n:n]
	r.flush()
	m := len(r.out)
	r.out = keep[:0]
	return dst[:n+m]
}

// flush writes a slot of idle input, in whole parts of 64 samples, and
// ends the stream. It writes them 16 at a time, which keeps the stack of
// a microcontroller small.
func (r *DiscReceiver) flush() {
	var idle [16]int16
	v := int16(max32(min32(r.f.dc/r.f.gain, 32767), -32768))
	for i := range idle {
		idle[i] = v
	}
	for range slotSymbols * discSPS &^ 63 / len(idle) {
		r.write(idle[:])
	}
	if r.locked {
		r.unlock()
	}
}

// run runs the samples x through the front end, the symbol timing and
// the core. It is the leanTiming.push of the matched filter samples, fused
// into one loop that keeps the last six samples, the time of the next
// strobe and the state of the lock in locals: the core sees them only
// around levels and symbol, which can retune or lock. The samples are kept
// as cubic takes them, s3 the last, and the linear interpolation runs
// between s2 and s3.
func (r *DiscReceiver) run(x []int16) {
	if !r.front96 {
		r.runAll(x)
		return
	}
	// The slots decode here, so that the frame of run96 is not on the
	// stack while they do.
	for {
		i, _ := r.run96(x, 0, nil)
		x = x[i:]
		if !r.slotDue {
			return
		}
		r.slotDue = false
		r.endSlot()
	}
}

// runAll is run at every rate. It is not inlined, so that run keeps a
// small frame.
//
//go:noinline
func (r *DiscReceiver) runAll(x []int16) {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	// dn counts the matched filter samples after r.n.
	var dn int32
	// kh and kq are the head of the ring of kept samples and the sample
	// whose pair is not stored yet: the loop stores two samples at once.
	kh, kq := r.acqHead, r.acqLow
	buf, an := r.acqBuf()
	cubic, half, slow := tm.cubic, f.half, f.slow
	// search is set while the receiver searches, and keep while it keeps
	// the matched filter samples.
	search, keep := !r.locked, !r.locked || r.hunt
	for _, v := range x {
		if search {
			f.sum += int32(v)
			f.sq += int64(v) * int64(v)
			if f.n++; f.n == frontBlock {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.acqHead, r.acqLow = kh, kq
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
		// A discriminator gives no carrier power, so the core takes a
		// burst as present from its anchors.
		dn++
		sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
		if keep {
			// acc is in Q15 of rrcStepHz, twice discStepHz, so this is y
			// in units of discStepHz, rounded.
			// Samples beyond int8 wrap: they are noise without a carrier,
			// which no SYNC follows.
			q := int16(int8((acc + 1<<13) >> 14))
			if kh&1 == 0 {
				kq = q
			} else if i := kh >> 1; i < int32(len(buf)) {
				buf[i] = int16(uint16(uint8(kq)) | uint16(uint8(q))<<8)
			}
			if kh++; kh == an {
				kh = 0
			}
		}
		t--
		for t <= 0 {
			at := t
			var xv float32
			if cubic {
				xv = cubicAt(tm.six, sa, sb, s0, s1, s2, s3, 1+at)
				// The cubic strobes are one sample late.
				at--
			} else {
				xv = s3 + at*(s3-s2)
			}
			tm.t = t
			sym := tm.strobe(xv)
			t = tm.t
			if sym {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3 = sa, sb, s0, s1, s2, s3
				r.n += int64(dn)
				dn = 0
				r.acqHead, r.acqLow = kh, kq
				r.symbol(tm.last, at)
				sa, sb, s0, s1, s2, s3, t, kq = tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.acqLow
				// A slot can start or end the search of a lock, which
				// keeps the samples in another ring.
				kh = r.acqHead
				buf, an = r.acqBuf()
				search, keep = !r.locked, !r.locked || r.hunt
			}
		}
	}
	tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
	r.n += int64(dn)
	r.acqHead, r.acqLow = kh, kq
}

const (
	// run96Strobe picks the interpolator of the strobes of run96 that
	// are not linear midpoints: 0 by the options, 1 Catmull-Rom and 3 the
	// cubic Lagrange polynomial through s0 to s3, 2 linear between s1 and
	// s2.
	run96Strobe = 0
)

// The bits of the state of run96, which state96 returns.
const (
	// st96Search is set before a lock, and st96Keep while the receiver
	// keeps the samples: before a lock and in a hunt.
	st96Search = 1 << iota
	st96Keep
	// st96Raw is st96Search with RawSearch.
	st96Raw
	// st96Lock is a lock with RawSearch 2, and st96Quiet one that does not
	// hunt.
	st96Lock
	st96Quiet
	// st96LinMid is LinearMid, and st96Tab StrobeTab.
	st96LinMid
	st96Tab
)

// state96 returns the state of the receiver for run96, which keeps it in
// one local in place of a local for each flag.
func (r *DiscReceiver) state96() uint8 {
	var st uint8
	if r.mid96&mid96Tab != 0 {
		st |= st96Tab
	}
	mode := r.mid96 &^ mid96Tab
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

// run96 is run for the matched filter at 9.6 kHz with cubic strobes, from
// sample i0 of x. It keeps the gain, the offset and the head of the
// filter in locals, and loads them again after the calls that can change
// them. It returns the end of the samples of x that it took: all of x, or
// those up to the symbol that ends a slot, which it leaves to run. The
// core of an iqLowRx has x in iqLowRx.buf, and run96 refills it from the
// I/Q samples in when it ends, with fill: then the end is in the last
// fill, and run96 also returns the samples of in that it filled. It is not
// inlined; see run.
//
//go:noinline
func (r *DiscReceiver) run96(x []int16, i0 int, in []uint32) (int, int) {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	// i0 is the sample after r.n, and fed the samples of in that fill
	// put into x.
	fed := 0
	// n is the end of the samples in x, which fill moves up to cap(x): x
	// itself stays one value, so that the compiler sees that run96 keeps
	// no pointer into it.
	n := len(x)
	kh, kq := r.acqHead, r.acqLow
	buf, an := r.acqBuf()
	st := r.state96()
	k, off := f.k, f.off
	hist := &f.rrcHist
	h := f.rrcHead
	if h >= audio96Taps {
		h = 0
	}
	// Pass i runs the strobes of sample i-1, then takes sample i. The
	// strobes of the first pass are those that the last sample of the
	// call before left, when a slot ended there.
	for i := i0; ; i++ {
		if st&st96Raw != 0 && t <= 0 {
			// The search takes the samples as they come, the symbols and
			// the midpoints in turn, without the timing loop: a lock samples
			// its SYNC again. It runs before the loop below, so that the
			// compiler does not compute the cubic of the loop for it.
			t = tm.step
			if tm.mid {
				tm.half, tm.mid = s2, false
			} else {
				tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*s2*s2
				tm.last, tm.mid = s2, true
				if hit, sym, mid := r.rawSym(s2); hit {
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
				}
			}
		}
		if t <= 0 {
			// The strobes, in a loop that the compiler does not enter for
			// the samples without one.
			for {
				if st&st96Raw != 0 {
					// The search takes the samples as they come, the
					// symbols and the midpoints in turn, without the
					// timing loop: a lock samples its SYNC again.
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
				case st&st96Tab != 0:
					xv = lagrangeAt(sa, sb, s0, s1, s2, s3, mu)
				default:
					xv = lagrange6(sa, sb, s0, s1, s2, s3, mu)
				}
				at--
				// tm.strobe, written out: TinyGo does not inline it in two
				// loops.
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
					tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*yv*yv
					tm.last, tm.half = xv, d
					t += 2 * tm.step
					if !tm.hold && tm.pow > 0 {
						t += max32(min32(leanMMGain*tm.gain*e/tm.pow, 0.5), -0.5)
					}
				case tm.mid:
					tm.half, tm.mid = xv, false
					t += tm.step
					sym = false
				default:
					tm.pow = leanPowerForget*tm.pow + (1-leanPowerForget)*xv*xv
					e := tm.half * (tm.last - xv)
					tm.last, tm.mid = xv, true
					t += tm.step
					if !tm.hold && tm.pow > 0 {
						t += max32(min32(tm.gain*e/tm.pow, 0.5), -0.5)
					}
				}
				if sym && st&st96Quiet != 0 {
					// A lock that does not hunt: run96 stores its locals
					// only at the end of the slot.
					r.lockPush(tm.last, at)
					if r.ns != r.slotEnd {
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
					return i, fed
				}
				if sym {
					tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
					r.n += int64(i - i0)
					i0 = i
					r.acqHead, r.acqLow = kh, kq
					if st&st96Lock != 0 {
						again, mid, match := r.lockSym(tm.last, at)
						if !match && r.ns != r.slotEnd {
							if t > 0 {
								break
							}
							continue
						}
						f.rrcHead = h
						if match {
							r.huntMatch(again, mid)
						}
						if r.ns == r.slotEnd {
							r.slotDue = true
						}
					} else {
						f.rrcHead = h
						r.symbol(tm.last, at)
					}
					if r.slotDue {
						// Every local is in r: run decodes the slot and
						// calls run96 again.
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
		if i == n {
			if !r.iqLow {
				break
			}
			r.n += int64(i - i0)
			i0 = i
			m := iqLowOf(r).fill(in)
			if m == 0 {
				break
			}
			in, fed = in[m:], fed+m
			n, i, i0 = m, -1, 0
			continue
		}
		// i is below n, which is at most cap(x).
		v := *(*int16)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(x)), 2*i))
		if st&st96Search != 0 {
			f.sum += int32(v)
			f.sq += int64(v) * int64(v)
			if f.n++; f.n == frontBlock {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.acqHead, r.acqLow = kh, kq
				f.rrcHead = h
				r.levels()
				sb, s0, s1, s2, s3, t = tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
				kq = r.acqLow
				k, off = f.k, f.off
			}
		}
		u := int8(max(min((int32(v)*k-off)>>16, 127), -128))
		// h is below audio96Taps: the ring stores and the window need no
		// index checks.
		p := unsafe.Add(unsafe.Pointer(hist), h)
		*(*int8)(p), *(*int8)(unsafe.Add(p, audio96Taps)) = u, u
		if h++; h >= audio96Taps {
			h = 0
		}
		acc := fir96((*[audio96Taps]int8)(unsafe.Add(unsafe.Pointer(hist), h)))
		y := float32(acc) * (rrcStepHz / 32768.0)
		sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
		if st&st96Keep != 0 {
			q := int16(int8((acc + 1<<13) >> 14))
			if kh&1 == 0 {
				kq = q
			} else if j := kh >> 1; j < int32(len(buf)) {
				buf[j] = int16(uint16(uint8(kq)) | uint16(uint8(q))<<8)
			}
			if kh++; kh == an {
				kh = 0
			}
		}
		t--
	}
	f.rrcHead = h
	tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
	r.n += int64(n - i0)
	r.acqHead, r.acqLow = kh, kq
	return n, fed
}

// OffsetHz returns the DC offset of the last locked stream, in units of
// the input samples. It is the carrier offset times the gain of the
// discriminator, plus the offset of its converter.
func (r *DiscReceiver) OffsetHz() float64 {
	return float64(float32(r.offsetHz) / r.f.gain)
}

// DeviationHz returns the outer deviation of the last locked stream, in
// units of the input samples.
func (r *DiscReceiver) DeviationHz() float64 {
	return float64(float32(r.devHz) / r.f.gain)
}

// levels ends a block of frontBlock samples that came while the receiver
// searched. It scales the input to discInputRMSHz when it is off by more
// than a factor of 2, and moves the DC offset that the front end removes
// to the mean input when they differ by more than leanSearchHz. A locked
// stream keeps both, and the SYNC fits of the core follow it. It is not
// inlined, so that the code of every sample stays small.
//
//go:noinline
func (r *DiscReceiver) levels() {
	f := &r.f
	m := float32(f.sum) / frontBlock
	// sq is not negative; the halves convert without a 64-bit helper.
	sq := float32(uint32(f.sq>>32))*(1<<32) + float32(uint32(f.sq))
	v := sq/frontBlock - m*m
	f.sum, f.sq, f.n = 0, 0, 0
	m *= f.gain
	v *= f.gain * f.gain
	f.mean += discMeanGain * (m - f.mean)
	if rms := sqrtf(max(v, 0)); rms > 0 {
		if g := discInputRMSHz / rms; g > 2 || g < 0.5 || f.up && g > discLevelUp {
			r.scaleAudio(g)
		}
	}
	if d := f.mean - f.dc; abs32(d) > leanSearchHz {
		r.retune(d)
	}
}

// scaleAudio multiplies the gain of the audio front end by g, with the
// offsets in Hz and the samples that the front end and the core hold.
func (r *leanCore) scaleAudio(g float32) {
	f := r.audio
	f.gain *= g
	f.mean *= g
	f.dc *= g
	r.tuneHz *= g
	r.off *= g
	f.scale(g)
	f.refresh()
	r.rescale(g)
}

// audioFront is the front end of a DiscReceiver: the gain that scales the
// input to Hz, the removal of the DC offset dc, the half-band filter of
// discHalfQ15 for 24 kHz input, and the matched filter.
type audioFront struct {
	// gain is Hz per input unit and dc the offset in Hz that the front end
	// removes, and k and off them in units of rrcStepHz, in Q16, limited
	// so that int32 holds u*k-off for every input u. sum, sq and n
	// accumulate the input samples of the block of levels, and mean is the
	// mean input in Hz.
	gain float32
	dc   float32
	k    int32
	off  int32
	sum  int32
	sq   int64
	n    int
	mean float32
	// half selects the half-band filter, for 24 kHz input, and slow the
	// matched filter at 9.6 kHz.
	half bool
	slow bool
	// rrcHist holds the matched filter history twice, so that the last
	// taps are rrcHist[rrcHead:], oldest first, and hbHist the ring of the
	// half-band filter, with odd set between its outputs.
	rrcHist [2 * audio12Taps]int8
	rrcHead int
	hbHist  [halfTaps]int8
	hbHead  int
	odd     bool
	// up is options.LevelUp.
	up bool
}

// refresh sets k and off from gain and dc.
func (f *audioFront) refresh() {
	f.k = int32(max(min(f.gain/rrcStepHz*65536+0.5, audioMaxK), 0))
	f.off = int32(max(min(f.dc/rrcStepHz*65536, audioMaxOff), -audioMaxOff)) - 32768
}

// scale multiplies the samples that the filters hold by g. A sample that
// the quantizer clipped stays clipped.
func (f *audioFront) scale(g float32) {
	for i, h := range &f.rrcHist {
		f.rrcHist[i] = scaleSat8(h, g)
	}
	for i, h := range &f.hbHist {
		f.hbHist[i] = scaleSat8(h, g)
	}
}

// scaleSat8 returns h*g as sat8 does, and h at a limit of int8 as it is.
func scaleSat8(h int8, g float32) int8 {
	if h == math.MaxInt8 || h == math.MinInt8 {
		return h
	}
	return sat8(float32(h) * g)
}

// retuneFront moves the offset that f removes to hz and adds -d to the
// samples that its filters hold.
func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	f.refresh()
	f.shiftHist(d)
}

// shiftHist adds -d to the samples that the filters of f hold.
func (f *audioFront) shiftHist(d float32) {
	for i, v := range &f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
	for i, v := range &f.hbHist {
		f.hbHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
}

// quantize returns the input sample u in Hz less the DC offset, in units
// of rrcStepHz.
func (f *audioFront) quantize(u int16) int8 {
	return int8(max(min((int32(u)*f.k-f.off)>>16, 127), -128))
}

// halfBand takes one quantized sample v at 24 kHz and returns the
// half-band filter sample at 12 kHz when v completes one.
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

// mf96 takes one sample v at 9.6 kHz and returns the matched filter
// sample in Q15.
func (f *audioFront) mf96(v int8) int32 {
	h := f.rrcHead
	if h >= audio96Taps {
		// Never; it bounds h, so that the compiler drops the index
		// checks.
		h = 0
	}
	f.rrcHist[h], f.rrcHist[h+audio96Taps] = v, v
	if h++; h == audio96Taps {
		h = 0
	}
	f.rrcHead = h
	return fir96((*[audio96Taps]int8)(f.rrcHist[h : h+audio96Taps]))
}

// mf12 takes one sample v at 12 kHz and returns the matched filter sample
// in Q15.
func (f *audioFront) mf12(v int8) int32 {
	h := f.rrcHead
	if h >= audio12Taps {
		// Never; as in mf96.
		h = 0
	}
	f.rrcHist[h], f.rrcHist[h+audio12Taps] = v, v
	if h++; h == audio12Taps {
		h = 0
	}
	f.rrcHead = h
	return fir12((*[audio12Taps]int8)(f.rrcHist[h : h+audio12Taps]))
}

// ringAt returns the index of sample k, oldest first, of the half-band ring
// whose oldest sample is at h. It is not a closure, which TinyGo would
// allocate on every call when it does not inline it.
func ringAt(h, k int) int {
	if k += h; k >= halfTaps {
		k -= halfTaps
	}
	return k
}

// fir12 returns the matched filter output in Q15 of the samples w at
// 12 kHz, oldest first. The taps are written out, so that TinyGo folds
// them into the code at every -opt.
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
