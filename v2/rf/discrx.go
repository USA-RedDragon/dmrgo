package rf

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
	// opt, when not nil, replaces the settings of TierDisc.
	opt *options
}

// DiscReceiver recovers DMR bursts from the output of an FM discriminator,
// as the radio chips of hotspots and MMDVM modems give it: real samples in
// any units, with any gain and DC offset, which the receiver measures
// itself. It detects as TierDisc does, without lookback, and keeps about
// 1 KiB of state, for microcontrollers. A DiscReceiver that Init sets up in
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
)

// NewDisc returns a DiscReceiver of cfg. It allocates the receiver, and
// the burst buffer of Write when cfg.OnBurst is nil.
func NewDisc(cfg DiscConfig) (*DiscReceiver, error) {
	r := &DiscReceiver{}
	if err := r.Init(cfg); err != nil {
		return nil, err
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
	opt := tierOptions(TierDisc)
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
	r.f.gain = 1
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

// growBursts returns dst with room for n more bursts, as slices.Grow.
func growBursts(dst []Burst, n int) []Burst {
	if cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], make([]Burst, n)...)[:len(dst)]
	}
	return dst
}

// maxBursts returns the most bursts that n input samples complete.
func (r *DiscReceiver) maxBursts(n int) int {
	return 4*n*symbolRate/(slotSymbols*r.hz()) + 16
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

// flush writes a slot of idle input and ends the stream.
func (r *DiscReceiver) flush() {
	var idle [64]int16
	v := int16(max32(min32(r.f.dc/r.f.gain, 32767), -32768))
	for i := range idle {
		idle[i] = v
	}
	for range slotSymbols * discSPS / len(idle) {
		r.write(idle[:])
	}
	if r.locked {
		r.unlock()
	}
}

// write runs the samples x through the front end, the symbol timing and
// the core. It is the leanTiming.push of the matched filter samples, fused
// into one loop that keeps the last four samples and the time of the next
// strobe in locals: the core sees them only around levels and symbol,
// which can retune or lock. The samples are kept as cubic takes them, s3
// the last, and the linear interpolation runs between s2 and s3.
func (r *DiscReceiver) write(x []int16) {
	f, tm := &r.f, &r.tm
	s0, s1, s2, s3, t, n := tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.n
	for _, v := range x {
		if !r.locked {
			f.sum += int32(v)
			f.sq += int64(v) * int64(v)
		}
		if f.n++; f.n == frontBlock {
			tm.s0, tm.s1, tm.s2, tm.s3, tm.t = s0, s1, s2, s3, t
			r.levels()
			s0, s1, s2, s3, t = tm.s0, tm.s1, tm.s2, tm.s3, tm.t
		}
		y, ok := f.filter(v)
		if !ok {
			continue
		}
		// A discriminator gives no carrier power, so the core takes a
		// burst as present from its anchors.
		n++
		s0, s1, s2, s3 = s1, s2, s3, y
		t--
		for t <= 0 {
			at := t
			var xv float32
			if tm.cubic {
				mu := 1 + at
				xv = s1 + 0.5*mu*(s2-s0+mu*(2*s0-5*s1+4*s2-s3+mu*(3*(s1-s2)+s3-s0)))
				// The cubic strobes are one sample late.
				at--
			} else {
				xv = s3 + at*(s3-s2)
			}
			tm.t = t
			sym := tm.strobe(xv)
			t = tm.t
			if sym {
				tm.s0, tm.s1, tm.s2, tm.s3, r.n = s0, s1, s2, s3, n
				r.symbol(tm.last, at)
				s0, s1, s2, s3, t = tm.s0, tm.s1, tm.s2, tm.s3, tm.t
			}
		}
	}
	tm.s0, tm.s1, tm.s2, tm.s3, tm.t, r.n = s0, s1, s2, s3, t, n
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

// levels ends a block of frontBlock samples. While the receiver searches,
// it scales the input to discInputRMSHz when it is off by more than a
// factor of 2, and moves the DC offset that the front end removes to the
// mean input when they differ by more than leanSearchHz. A locked stream
// keeps both, and the SYNC fits of the core follow it. It is not inlined,
// so that the code of every sample stays small.
//
//go:noinline
func (r *DiscReceiver) levels() {
	f := &r.f
	m := float32(f.sum) / frontBlock
	// sq is not negative; the halves convert without a 64-bit helper.
	sq := float32(uint32(f.sq>>32))*(1<<32) + float32(uint32(f.sq))
	v := sq/frontBlock - m*m
	f.sum, f.sq, f.n = 0, 0, 0
	if r.locked {
		return
	}
	m *= f.gain
	v *= f.gain * f.gain
	f.mean += discMeanGain * (m - f.mean)
	if rms := sqrtf(max(v, 0)); rms > 0 {
		if g := discInputRMSHz / rms; g > 2 || g < 0.5 {
			f.gain *= g
			f.mean *= g
			f.dc *= g
			r.tuneHz *= g
			r.off *= g
			f.scale(g)
			f.refresh()
		}
	}
	if d := f.mean - f.dc; abs32(d) > leanSearchHz {
		r.retune(d)
	}
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
}

// refresh sets k and off from gain and dc.
func (f *audioFront) refresh() {
	f.k = int32(max(min(f.gain/rrcStepHz*65536+0.5, audioMaxK), 0))
	f.off = int32(max(min(f.dc/rrcStepHz*65536, audioMaxOff), -audioMaxOff)) - 32768
}

// scale multiplies the samples that the filters hold by g.
func (f *audioFront) scale(g float32) {
	for i, h := range &f.rrcHist {
		f.rrcHist[i] = sat8(float32(h) * g)
	}
	for i, h := range &f.hbHist {
		f.hbHist[i] = sat8(float32(h) * g)
	}
}

// retuneFront moves the offset that f removes to hz and adds -d to the
// samples that its filters hold.
func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	f.refresh()
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

// filter takes one input sample u and returns the matched filter sample,
// in Hz, of u in Hz less the DC offset, when the sample completes one.
func (f *audioFront) filter(u int16) (float32, bool) {
	v := f.quantize(u)
	if f.half {
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
		v = int8(max(min((acc+1<<14)>>15, 127), -128))
	}
	var acc int32
	if f.slow {
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
		acc = fir96((*[audio96Taps]int8)(f.rrcHist[h : h+audio96Taps]))
	} else {
		h := f.rrcHead
		if h >= audio12Taps {
			// Never; as above.
			h = 0
		}
		f.rrcHist[h], f.rrcHist[h+audio12Taps] = v, v
		if h++; h == audio12Taps {
			h = 0
		}
		f.rrcHead = h
		acc = fir12((*[audio12Taps]int8)(f.rrcHist[h : h+audio12Taps]))
	}
	return float32(acc) * (rrcStepHz / 32768.0), true
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

// fir96 returns the matched filter output in Q15 of the samples w at
// 9.6 kHz, oldest first. The taps are written out, so that TinyGo folds
// them into the code at every -opt.
func fir96(w *[audio96Taps]int8) int32 {
	return int32(audio96RRCQ15[8])*int32(w[8]) +
		int32(audio96RRCQ15[0])*(int32(w[0])+int32(w[16])) +
		int32(audio96RRCQ15[1])*(int32(w[1])+int32(w[15])) +
		int32(audio96RRCQ15[2])*(int32(w[2])+int32(w[14])) +
		int32(audio96RRCQ15[3])*(int32(w[3])+int32(w[13])) +
		int32(audio96RRCQ15[4])*(int32(w[4])+int32(w[12])) +
		int32(audio96RRCQ15[5])*(int32(w[5])+int32(w[11])) +
		int32(audio96RRCQ15[6])*(int32(w[6])+int32(w[10])) +
		int32(audio96RRCQ15[7])*(int32(w[7])+int32(w[9]))
}
