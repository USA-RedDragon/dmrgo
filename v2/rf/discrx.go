package rf

// Layout of the discriminator front end.
const (
	// discInputRMSHz is the RMS, in Hz, to which a DiscReceiver scales its
	// input, and discMeanGain the weight of each block in its mean.
	discInputRMSHz = 2000
	discMeanGain   = 0.1
	// audioBox, when above 0, replaces the matched filter of the front end
	// with the mean of the last audioBox samples, and audioBoxStepHz is the
	// resolution of the samples that it holds.
	audioBox       = 0
	audioBoxStepHz = 8
	// audioDecimate is 2 to run the matched filter and the symbol timing
	// at half the input rate, after the half-band filter of discHalfQ15,
	// and 1 to run them at the input rate.
	audioDecimate = 2
	// audioTaps is the length of the matched filter at the rate of
	// audioDecimate, and audioRRCTaps its length at half the input rate.
	audioTaps    = 2*discSpan*discSPS/audioDecimate + 1
	audioRRCTaps = discSpan*discSPS + 1
)

// DiscConfig sets up a DiscReceiver.
type DiscConfig struct {
	// Hz is the sample rate of the discriminator samples of Write. 0 means
	// 24000, the only rate that DiscReceiver takes for now.
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
	if cfg.Hz != discRate {
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
	r.init(cfg.OnBurst, opt, 4*cfg.MaxWrite/(slotSymbols*discSPS)+16, float32(discSPS)/audioDecimate)
	r.f.gain = 1
	r.front, r.center = &r.f, -int64(discSpan*samplesPerSy)
	if audioDecimate == 2 {
		r.center -= halfTaps - 1
	}
	if audioBox > 0 {
		r.center = -int64(audioBox - 1)
	}
	return r, nil
}

// Write processes the discriminator samples x and returns the bursts that
// are complete. The returned slice is valid until the next call.
func (r *DiscReceiver) Write(x []int16) []Burst {
	r.out = r.out[:0]
	for _, v := range x {
		r.sample(float32(v))
	}
	return r.out
}

// Flush finishes the stream at its end and returns the remaining bursts.
func (r *DiscReceiver) Flush() []Burst {
	r.out = r.out[:0]
	for range slotSymbols * discSPS {
		r.sample(r.f.dc / r.f.gain)
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
	return float64(r.offsetHz / r.f.gain)
}

// DeviationHz returns the outer deviation of the last locked stream, in
// units of the input samples.
func (r *DiscReceiver) DeviationHz() float64 {
	return float64(r.devHz / r.f.gain)
}

// sample runs one input sample through the front end and the core.
func (r *DiscReceiver) sample(v float32) {
	f := &r.f
	x := v * f.gain
	f.sum += x
	f.sq += x * x
	if f.n++; f.n == frontBlock {
		r.levels()
	}
	if s, ok := f.filter(x); ok {
		r.power(1)
		r.push(s)
	}
}

// levels ends a block of frontBlock samples. While the receiver searches,
// it scales the input to discInputRMSHz when it is off by more than a
// factor of 2, and moves the DC offset that the front end removes to the
// mean input when they differ by more than leanSearchHz. A locked stream
// keeps both, and the SYNC fits of the core follow it.
func (r *DiscReceiver) levels() {
	f := &r.f
	m := f.sum / frontBlock
	v := f.sq/frontBlock - m*m
	f.sum, f.sq, f.n = 0, 0, 0
	if r.locked {
		return
	}
	f.mean += discMeanGain * (m - f.mean)
	if rms := sqrtf(max(v, 0)); rms > 0 {
		if g := discInputRMSHz / rms; g > 2 || g < 0.5 {
			f.gain *= g
			f.mean *= g
			f.dc *= g
			r.tuneHz *= g
			r.off *= g
			for i, h := range f.rrcHist {
				f.rrcHist[i] = sat8(float32(h) * g)
			}
			for i, h := range f.hbHist {
				f.hbHist[i] = sat8(float32(h) * g)
			}
			f.boxSum = 0
			for i, h := range f.box {
				f.box[i] = sat16(float32(h) * g)
				f.boxSum += int32(f.box[i])
			}
		}
	}
	if d := f.mean - f.dc; abs32(d) > leanSearchHz {
		r.retune(d)
	}
}

// audioFront is the front end of a DiscReceiver: the gain that scales the
// input to Hz, the removal of the DC offset dc, and the matched filter of
// leanFront.
type audioFront struct {
	// gain is Hz per input unit and dc the offset in Hz that the front end
	// removes. sum, sq and n accumulate the block of levels, and mean is
	// the mean input.
	gain float32
	dc   float32
	sum  float32
	sq   float32
	n    int
	mean float32
	// rrcHist holds the matched filter history twice, so that the last
	// audioTaps samples are rrcHist[rrcHead:rrcHead+audioTaps], oldest
	// first, and hbHist the history of the half-band filter of
	// audioDecimate, with odd set between its outputs.
	rrcHist [2 * audioTaps]int8
	rrcHead int
	hbHist  [2 * halfTaps]int8
	hbHead  int
	odd     bool
	// box and boxSum hold the samples of the audioBox mean and their sum,
	// in units of audioBoxStepHz.
	box    [max(audioBox, 1)]int16
	boxSum int32
}

// retuneFront moves the offset that f removes to hz and adds -d to the
// samples that its filter holds.
func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	for i, v := range f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
	for i, v := range f.hbHist {
		f.hbHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
	f.boxSum = 0
	for i, v := range f.box {
		f.box[i] = sat16(float32(v) - d/audioBoxStepHz)
		f.boxSum += int32(f.box[i])
	}
}

// sat16 rounds v to the nearest int16, saturated.
func sat16(v float32) int16 {
	return int16(max32(min32(v+0.5*sign32(v), 32767), -32768))
}

// sign32 returns 1 for v >= 0 and -1 otherwise.
func sign32(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}

// filter takes one input sample x in Hz and returns the matched filter
// sample, in Hz, of x less the DC offset, when the sample completes one.
func (f *audioFront) filter(x float32) (float32, bool) {
	if audioBox > 0 {
		v := sat16((x - f.dc) / audioBoxStepHz)
		h := f.rrcHead
		f.boxSum += int32(v) - int32(f.box[h])
		f.box[h] = v
		if h++; h == audioBox {
			h = 0
		}
		f.rrcHead = h
		return float32(f.boxSum) * (audioBoxStepHz / float32(max(audioBox, 1))), true
	}
	v := sat8((x - f.dc) / rrcStepHz)
	if audioDecimate == 2 {
		h := f.hbHead
		f.hbHist[h], f.hbHist[h+halfTaps] = v, v
		if h++; h == halfTaps {
			h = 0
		}
		f.hbHead = h
		if f.odd = !f.odd; f.odd {
			return 0, false
		}
		w := (*[halfTaps]int8)(f.hbHist[h : h+halfTaps])
		const c = halfTaps / 2
		acc := int32(discHalfQ15[0]) * int32(w[c])
		for k, t := range discHalfQ15[1:] {
			acc += int32(t) * (int32(w[c-2*k-1]) + int32(w[c+2*k+1]))
		}
		v = int8(max(min((acc+1<<14)>>15, 127), -128))
	}
	h := f.rrcHead
	f.rrcHist[h], f.rrcHist[h+audioTaps] = v, v
	if h++; h == audioTaps {
		h = 0
	}
	f.rrcHead = h
	taps := discRRCQ15[:]
	if audioDecimate == 2 {
		taps = audioRRCQ15[:]
	}
	return float32(symFIR(taps, f.rrcHist[h:h+audioTaps])) * (rrcStepHz / 32768.0), true
}

// symFIR returns the output of the symmetric filter whose taps up to its
// center tap are taps over the samples w, oldest first, in Q15.
func symFIR(taps []int16, w []int8) int32 {
	n := len(w)
	mid := n / 2
	acc := int32(taps[mid]) * int32(w[mid])
	for k := range mid {
		acc += int32(taps[k]) * (int32(w[k]) + int32(w[n-1-k]))
	}
	return acc
}
