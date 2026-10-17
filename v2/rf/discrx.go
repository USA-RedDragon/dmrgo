package rf

// Layout of the discriminator front end.
const (
	// discInputRMSHz is the RMS, in Hz, to which a DiscReceiver scales its
	// input, and discMeanGain the weight of each block in its mean.
	discInputRMSHz = 2000
	discMeanGain   = 0.1
	// audio12Taps and audio96Taps are the lengths of the matched filters at
	// 12 kHz (2.5 samples per symbol) and 9.6 kHz (2 samples per symbol),
	// discSpan symbols on each side.
	audio12Taps = discSpan*discSPS + 1
	audio96Taps = 4*discSpan + 1
)

// DiscConfig sets up a DiscReceiver.
type DiscConfig struct {
	// Hz is the sample rate of the discriminator samples of Write: 24000,
	// as MMDVM modems sample, 12000 or 9600. 0 means 24000. A lower rate
	// costs less: the receiver halves 24 kHz with a half-band filter before
	// its matched filter, which runs at 12 kHz or 9.6 kHz.
	Hz int
	// MaxWrite is the largest number of samples in one Write. 0 means
	// 1 << 12.
	MaxWrite int
	// OnBurst, when not nil, takes each burst as the receiver recovers it,
	// in place of the slice that Write and Flush return, which are then
	// empty. The receiver then keeps no buffer of bursts.
	OnBurst func(Burst)
	// opt, when not nil, replaces the settings of TierDisc.
	opt *options
}

// DiscReceiver recovers DMR bursts from the output of an FM discriminator,
// as the radio chips of hotspots and MMDVM modems give it: real samples in
// any units, with any gain and DC offset, which the receiver measures
// itself. It detects as TierDisc does, without lookback, and keeps about
// 1 KiB of state, for microcontrollers. A DiscReceiver does not allocate
// after NewDisc and is not safe for concurrent use.
type DiscReceiver struct {
	leanCore
	f audioFront
}

// NewDisc returns a DiscReceiver of cfg. It allocates the receiver, and
// the burst buffer when cfg.OnBurst is nil.
func NewDisc(cfg DiscConfig) (*DiscReceiver, error) {
	if cfg.Hz == 0 {
		cfg.Hz = discRate
	}
	if cfg.Hz != discRate && cfg.Hz != discRate/2 && cfg.Hz != 2*symbolRate {
		return nil, &ConfigError{Field: "Hz"}
	}
	if cfg.MaxWrite == 0 {
		cfg.MaxWrite = 1 << 12
	}
	if cfg.MaxWrite < 0 {
		return nil, &ConfigError{Field: "MaxWrite"}
	}
	opt := tierOptions(TierDisc)
	if cfg.opt != nil {
		opt = *cfg.opt
	}
	r := &DiscReceiver{}
	r.f.half = cfg.Hz == discRate
	r.f.slow = cfg.Hz == 2*symbolRate
	sps := float32(discSPS) / 2
	if r.f.slow {
		sps = 2
	}
	r.init(cfg.OnBurst, opt, 4*cfg.MaxWrite*symbolRate/(slotSymbols*cfg.Hz)+16, sps)
	r.f.gain = 1
	r.f.refresh()
	r.front, r.center = &r.f, -int64(discSpan*samplesPerSy)
	if r.f.half {
		r.center -= halfTaps - 1
	}
	return r, nil
}

// Write processes the discriminator samples x and returns the bursts that
// are complete. The returned slice is valid until the next call.
func (r *DiscReceiver) Write(x []int16) []Burst {
	r.out = r.out[:0]
	f := &r.f
	for _, v := range x {
		// The body of sample, which the compiler does not inline.
		if !r.locked {
			f.sum += int32(v)
			f.sq += int64(v) * int64(v)
		}
		if f.n++; f.n == frontBlock {
			r.levels()
		}
		if s, ok := f.filter(v); ok {
			// A discriminator gives no carrier power, so the core takes
			// a burst as present from its anchors.
			r.n++
			r.push(s)
		}
	}
	return r.out
}

// Flush finishes the stream at its end and returns the remaining bursts.
func (r *DiscReceiver) Flush() []Burst {
	r.out = r.out[:0]
	idle := int16(max32(min32(r.f.dc/r.f.gain, 32767), -32768))
	for range slotSymbols * discSPS {
		r.sample(idle)
	}
	if r.locked {
		r.unlock()
	}
	return r.out
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

// sample runs one input sample through the front end and the core. While
// the receiver searches, it sums the samples for levels.
func (r *DiscReceiver) sample(v int16) {
	f := &r.f
	if !r.locked {
		f.sum += int32(v)
		f.sq += int64(v) * int64(v)
	}
	if f.n++; f.n == frontBlock {
		r.levels()
	}
	if s, ok := f.filter(v); ok {
		r.n++
		r.push(s)
	}
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
	v := float32(f.sq)/frontBlock - m*m
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
	// removes, and k and off them in units of rrcStepHz, in Q16. sum, sq
	// and n accumulate the input samples of the block of levels, and mean
	// is the mean input in Hz.
	gain float32
	dc   float32
	k    int64
	off  int64
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
	f.k = int64(f.gain/rrcStepHz*65536 + 0.5)
	f.off = int64(f.dc/rrcStepHz*65536) - 32768
}

// scale multiplies the samples that the filters hold by g.
func (f *audioFront) scale(g float32) {
	for i, h := range f.rrcHist {
		f.rrcHist[i] = sat8(float32(h) * g)
	}
	for i, h := range f.hbHist {
		f.hbHist[i] = sat8(float32(h) * g)
	}
}

// retuneFront moves the offset that f removes to hz and adds -d to the
// samples that its filters hold.
func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	f.refresh()
	for i, v := range f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
	for i, v := range f.hbHist {
		f.hbHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
}

// quantize returns the input sample u in Hz less the DC offset, in units
// of rrcStepHz.
func (f *audioFront) quantize(u int16) int8 {
	return int8(max(min(int32((int64(u)*f.k-f.off)>>16), 127), -128))
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
		// at returns sample k of the ring, oldest first.
		at := func(k int) int32 {
			if k += h; k >= halfTaps {
				k -= halfTaps
			}
			return int32(f.hbHist[k])
		}
		const c = halfTaps / 2
		acc := int32(discHalfQ15[0]) * at(c)
		for k, t := range discHalfQ15[1:] {
			acc += int32(t) * (at(c-2*k-1) + at(c+2*k+1))
		}
		v = int8(max(min((acc+1<<14)>>15, 127), -128))
	}
	var acc int32
	if f.slow {
		h := f.rrcHead
		f.rrcHist[h], f.rrcHist[h+audio96Taps] = v, v
		if h++; h == audio96Taps {
			h = 0
		}
		f.rrcHead = h
		acc = fir96((*[audio96Taps]int8)(f.rrcHist[h : h+audio96Taps]))
	} else {
		h := f.rrcHead
		f.rrcHist[h], f.rrcHist[h+audio12Taps] = v, v
		if h++; h == audio12Taps {
			h = 0
		}
		f.rrcHead = h
		acc = fir12((*[audio12Taps]int8)(f.rrcHist[h : h+audio12Taps]))
	}
	return float32(acc) * (rrcStepHz / 32768.0), true
}

// fir12 returns the matched filter output in Q15 of the samples w at
// 12 kHz, oldest first.
func fir12(w *[audio12Taps]int8) int32 {
	const mid = audio12Taps / 2
	acc := int32(audioRRCQ15[mid]) * int32(w[mid])
	for k := range mid {
		acc += int32(audioRRCQ15[k]) * (int32(w[k]) + int32(w[audio12Taps-1-k]))
	}
	return acc
}

// fir96 returns the matched filter output in Q15 of the samples w at
// 9.6 kHz, oldest first.
func fir96(w *[audio96Taps]int8) int32 {
	const mid = audio96Taps / 2
	acc := int32(audio96RRCQ15[mid]) * int32(w[mid])
	for k := range mid {
		acc += int32(audio96RRCQ15[k]) * (int32(w[k]) + int32(w[audio96Taps-1-k]))
	}
	return acc
}
