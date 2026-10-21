package rf

import (
	"math"
	"unsafe"
)

const (
	discInputRMSHz = 2000
	discMeanGain   = 0.1
	audio96Span    = discSpan
	audio12Taps    = discSpan*discSPS + 1
	audio96Taps    = 4*audio96Span + 1
	discLevelUp    = 1.41
	audioMaxK      = 1<<15 - 1
	audioMaxOff    = 1<<30 - 1<<16
)

// DiscConfig sets up a DiscReceiver.
type DiscConfig struct {
	// Hz is the sample rate of the discriminator samples of Write: 48000,
	// as sound cards sample, 24000, as MMDVM modems sample, 12000 or 9600.
	// 0 means 24000. A lower rate costs less: the receiver halves 48 kHz
	// and then 24 kHz with a half-band filter before its matched filter,
	// which runs at 12 kHz or 9.6 kHz. For a sound card at 44100 Hz, set
	// it to 48000 Hz or resample to 48000 Hz first.
	Hz int
	// MaxWrite is the largest number of samples in one Write, for the
	// burst buffer that NewDisc allocates. 0 means 1 << 12. When OnBurst is
	// nil, a longer Write can return ErrWriteTooLong.
	MaxWrite int
	// OnBurst, when not nil, takes each burst as the receiver recovers it,
	// in place of the slices that Write, AppendBursts and Flush return,
	// which then return no bursts. The burst is valid only during the call;
	// copy it to keep it.
	OnBurst func(*Burst)
	// Tier is Tier0, Tier1, Tier2 or Tier3. The zero value TierDefault is
	// Tier1 for NewDisc, and Tier0 for Init, which does not allocate the
	// stages of the higher tiers, and under TinyGo, which has neither.
	// Tier2 detects the bursts again coherently, on the carrier phase that
	// it rebuilds from the samples, at about 5 times the CPU time of Tier1.
	// Tier3 runs Tier2 and, beside it, a receiver with more coherent CPU
	// time and four passes over each data burst. It keeps every burst of
	// Tier2 and adds the bursts that Tier2 misses, the data bursts that pass
	// a CRC where those of Tier2 fail, and the embedded LCs that pass where
	// those of Tier2 fail, at about twice the CPU time and memory of Tier2,
	// and 3 s of delay.
	Tier  Tier
	opt   *options
	lowIQ bool
}

// DiscReceiver recovers DMR bursts from the output of an FM discriminator,
// as the radio chips of hotspots and MMDVM modems give it: real samples in
// any units, with any gain and DC offset, which the receiver measures
// itself. With Tier0 it keeps about 1.4 KB of state, for microcontrollers;
// on a Cortex-M4F it needs about 3.8 MHz and 1.9 KB with its stack, built
// with TinyGo -opt z. A
// DiscReceiver that Init sets up in memory of the caller, such as a package
// variable, uses no heap with OnBurst or AppendBursts. A DiscReceiver is not
// safe for concurrent use.
type DiscReceiver struct {
	leanCore
	f   audioFront
	d48 disc48
}

var (
	errDiscHz       = &ConfigError{Type: "DiscConfig", Field: "Hz"}
	errDiscMaxWrite = &ConfigError{Type: "DiscConfig", Field: "MaxWrite"}
	errDiscTier     = &ConfigError{Type: "DiscConfig", Field: "Tier"}
)

// NewDisc returns a DiscReceiver of cfg. It allocates the receiver, and
// the burst buffer of Write when cfg.OnBurst is nil.
func NewDisc(cfg DiscConfig) (*DiscReceiver, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	if cfg.Tier < Tier0 || cfg.Tier > Tier3 {
		return nil, errDiscTier
	}
	r := &DiscReceiver{}
	soft := cfg.opt == nil
	opt := discTierOptions(cfg.Tier)
	if !soft {
		opt = *cfg.opt
	}
	cfg.opt = &opt
	if err := r.Init(cfg); err != nil {
		return nil, err
	}
	if soft && opt.SoftExt {
		if err := r.addSoft(&opt); err != nil {
			return nil, err
		}
	}
	if cfg.OnBurst == nil {
		if cfg.MaxWrite == 0 {
			cfg.MaxWrite = 1 << 12
		}
		r.out = make([]Burst, 0, max(r.maxBursts(cfg.MaxWrite), r.maxBursts(slotSymbols*discSPS)))
	}
	return r, nil
}

// Init sets up r for cfg, in place of any state it holds. It does not
// allocate, so that r can live in memory of the caller.
func (r *DiscReceiver) Init(cfg DiscConfig) error {
	dec := cfg.Hz == 2*discRate
	if cfg.Hz == 0 || dec {
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
	opt := disc0Options()
	switch {
	case cfg.opt != nil:
		opt = *cfg.opt
	case cfg.lowIQ:
		opt = iqLowOptions()
	}
	*r = DiscReceiver{d48: disc48{on: dec}}
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

func (r *DiscReceiver) hz() int {
	switch {
	case r.f.half:
		return discRate
	case r.f.slow:
		return 2 * symbolRate
	}
	return discRate / 2
}

func growBursts(dst []Burst, n int) []Burst {
	for cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], Burst{})[:len(dst)]
	}
	return dst
}

func (r *DiscReceiver) maxBursts(n int) int {
	return 4*r.inputSamples(n)*symbolRate/(slotSymbols*r.inputHz()) + 16
}

// AppendBursts processes the discriminator samples x, appends the bursts
// that are complete to dst and returns the extended slice. It allocates
// only when dst has no room for len(x)/72+16 bursts, the most that x can
// complete at the lowest input rate, 9600 Hz.
func (r *DiscReceiver) AppendBursts(dst []Burst, x []int16) []Burst {
	dst = growBursts(dst, r.maxBursts(len(x)))
	keep, n := r.out, len(dst)
	r.out = dst[n:n]
	r.input(x)
	m := len(r.out)
	r.out = keep[:0]
	return dst[:n+m]
}

// Write processes the discriminator samples x and returns the bursts that
// are complete, in the buffer that NewDisc allocates. It returns at most
// len(x)/72+16 bursts, as AppendBursts does. The returned slice
// is valid until the next call. Without OnBurst, it returns
// ErrWriteTooLong and processes nothing when its buffer has no room for
// the bursts that x can complete. The buffer of NewDisc takes a Write of up
// to DiscConfig.MaxWrite samples; a DiscReceiver that Init set up has none,
// so use AppendBursts.
func (r *DiscReceiver) Write(x []int16) ([]Burst, error) {
	if r.onBurst == nil && r.maxBursts(len(x)) > cap(r.out) {
		return nil, ErrWriteTooLong
	}
	r.out = r.out[:0]
	r.input(x)
	return r.out, nil
}

// Flush finishes the stream at its end and returns the remaining bursts,
// as Write does. It returns at most 26 bursts, as AppendFlush does. A
// DiscReceiver that Init set up without OnBurst has no buffer for them, so
// there Flush changes nothing and returns nil; use AppendFlush.
func (r *DiscReceiver) Flush() []Burst {
	if r.onBurst == nil && cap(r.out) == 0 {
		return nil
	}
	r.out = r.out[:0]
	r.flush()
	return r.out
}

// AppendFlush finishes the stream at its end, appends the remaining bursts
// to dst and returns the extended slice. It allocates only when dst has no
// room for 26 bursts, the most that one slot of samples can complete.
func (r *DiscReceiver) AppendFlush(dst []Burst) []Burst {
	dst = growBursts(dst, r.maxBursts(slotSymbols*discSPS))
	keep, n := r.out, len(dst)
	r.out = dst[n:n]
	r.flush()
	m := len(r.out)
	r.out = keep[:0]
	return dst[:n+m]
}

func (r *DiscReceiver) flush() {
	if !r.unionFlush() {
		r.flushSolo()
	}
}

func (r *DiscReceiver) flushSolo() {
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

func (r *DiscReceiver) run(x []int16) {
	if !r.front96 {
		r.runAll(x)
		return
	}
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

//go:noinline
func (r *DiscReceiver) runAll(x []int16) {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	var dn int32
	kh, kq := r.acqHead, r.acqLow
	buf, an := r.acqBuf()
	cubic, half, slow := tm.cubic, f.half, f.slow
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
		dn++
		sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
		if keep {
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
	run96Strobe = 0
)

const (
	st96Search = 1 << iota
	st96Keep
	st96Raw
	st96Lock
	st96Quiet
	st96LinMid
	st96Tab
)

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

//go:noinline
//go:section .text.run96
func (r *DiscReceiver) run96(x []int16, i0 int, in []uint32) (int, int) {
	f, tm := &r.f, &r.tm
	sa, sb, s0, s1, s2, s3, t := tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t
	fed := 0
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
	for i := i0; ; {
		if t > 0 {
			for i != n {
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
				p := unsafe.Add(unsafe.Pointer(hist), h)
				*(*int8)(p), *(*int8)(unsafe.Add(p, audio96Taps)) = u, u
				acc := fir96[site0]((*[audio96Taps]int8)(unsafe.Add(p, 1)))
				if h++; h >= audio96Taps {
					h = 0
				}
				y := float32(acc) * (rrcStepHz / 32768.0)
				sa, sb, s0, s1, s2, s3 = sb, s0, s1, s2, s3, y
				if st&st96Keep != 0 {
					q := int16(int8((acc + 1<<13) >> 14))
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
			case st&st96Tab != 0:
				xv = lagrangeAt(sa, sb, s0, s1, s2, s3, mu)
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
				return i, fed
			}
			if sym {
				tm.sa, tm.sb, tm.s0, tm.s1, tm.s2, tm.s3, tm.t = sa, sb, s0, s1, s2, s3, t
				r.n += int64(i - i0)
				i0 = i
				r.acqHead, r.acqLow = kh, kq
				if st&st96Lock != 0 {
					again, mid, match := r.lockSym(tm.last, at)
					if !match && uint32(r.ns) != uint32(r.slotEnd) {
						if t > 0 {
							break
						}
						continue
					}
					f.rrcHead = h
					if match {
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

//go:noinline
func (r *DiscReceiver) levels() {
	f := &r.f
	m := float32(f.sum) / frontBlock
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

type audioFront struct {
	gain    float32
	dc      float32
	k       int32
	off     int32
	sum     int32
	sq      int64
	n       int
	mean    float32
	half    bool
	slow    bool
	rrcHist [2 * audio12Taps]int8
	rrcHead int
	hbHist  [halfTaps]int8
	hbHead  int
	odd     bool
	up      bool
}

func (f *audioFront) refresh() {
	f.k = int32(max(min(f.gain/rrcStepHz*65536+0.5, audioMaxK), 0))
	f.off = int32(max(min(f.dc/rrcStepHz*65536, audioMaxOff), -audioMaxOff)) - 32768
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

func (f *audioFront) quantize(u int16) int8 {
	return int8(max(min((int32(u)*f.k-f.off)>>16, 127), -128))
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
