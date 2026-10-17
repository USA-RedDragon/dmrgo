package rf

// Layout of the discriminator front end.
const (
	// discInputRMSHz is the RMS, in Hz, to which a DiscReceiver scales its
	// input, and discMeanGain the weight of each block in its mean.
	discInputRMSHz = 2000
	discMeanGain   = 0.1
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
	r.init(cfg.OnBurst, opt, 4*cfg.MaxWrite/(slotSymbols*discSPS)+16)
	r.f.gain = 1
	r.front, r.center = &r.f, -int64(discSpan*samplesPerSy)
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
	r.power(1)
	r.push(f.filter(x))
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
	gain    float32
	dc      float32
	sum     float32
	sq      float32
	n       int
	mean    float32
	rrcHist [rrcTaps]int8
	rrcHead int
}

// retuneFront moves the offset that f removes to hz and adds -d to the
// samples that its filter holds.
func (f *audioFront) retuneFront(hz, d float32) {
	f.dc = hz
	for i, v := range f.rrcHist {
		f.rrcHist[i] = sat8(float32(v) - d/rrcStepHz)
	}
}

// filter takes one input sample x in Hz and returns the matched filter
// sample, in Hz, of x less the DC offset.
func (f *audioFront) filter(x float32) float32 {
	f.rrcHist[f.rrcHead] = sat8((x - f.dc) / rrcStepHz)
	if f.rrcHead++; f.rrcHead == rrcTaps {
		f.rrcHead = 0
	}
	const mid = rrcTaps / 2
	k0 := f.rrcHead + mid
	if k0 >= rrcTaps {
		k0 -= rrcTaps
	}
	acc := int32(discRRCQ15[mid]) * int32(f.rrcHist[k0])
	i, j := f.rrcHead, f.rrcHead-1
	if j < 0 {
		j += rrcTaps
	}
	for _, t := range discRRCQ15[:mid] {
		acc += int32(t) * (int32(f.rrcHist[i]) + int32(f.rrcHist[j]))
		if i++; i == rrcTaps {
			i = 0
		}
		if j--; j < 0 {
			j = rrcTaps - 1
		}
	}
	return float32(acc) * (rrcStepHz / 32768.0)
}
