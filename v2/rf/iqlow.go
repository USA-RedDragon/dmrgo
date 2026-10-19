package rf

import "math"

const (
	// iqLowBuf is the number of discriminator samples that iqLowRx hands
	// the core at once.
	iqLowBuf = 32
)

// LowIQConfig configures a LowIQReceiver.
type LowIQConfig struct {
	// Hz is the rate of the complex baseband: 9600 or 12000. 0 means
	// 9600.
	Hz int
	// OnBurst receives each burst; the receiver needs it. The burst is
	// valid only during the call.
	OnBurst func(*Burst)
	opt     *options
}

// LowIQReceiver is the Tier 0 receiver for complex baseband at the rate of
// its core, 9.6 or 12 kHz, that a radio chip, or a converter with
// hardware decimation, filtered to the channel and decimated. It takes the
// phase step of each sample in the frame of its oscillator and runs the
// DiscReceiver core on it, with the matched filter of the core as the
// only filter. It does not allocate, so that it can live in memory of the
// caller.
type LowIQReceiver struct {
	r iqLowRx
}

// errLowIQ is the error of LowIQReceiver.Init for a Config out of range.
var (
	errLowIQHz      = &ConfigError{Field: "Hz"}
	errLowIQOnBurst = &ConfigError{Field: "OnBurst"}
)

// Init sets up r for cfg, in place of any state it holds.
func (r *LowIQReceiver) Init(cfg LowIQConfig) error {
	if cfg.Hz == 0 {
		cfg.Hz = iq0LowHz
	}
	if cfg.Hz != iq0LowHz && cfg.Hz != discRate/2 {
		return errLowIQHz
	}
	if cfg.OnBurst == nil {
		return errLowIQOnBurst
	}
	return r.r.init(cfg.Hz, cfg.OnBurst, cfg.opt)
}

// WriteBaseband processes the samples x and hands the bursts that they
// complete to OnBurst.
func (r *LowIQReceiver) WriteBaseband(x []complex64) { r.r.writeBaseband(x) }

// Flush finishes the stream at its end.
func (r *LowIQReceiver) Flush() { r.r.flush() }

// iqLowRx is the receiver of LowIQReceiver, and of New with options.IQLow:
// the phase step of each sample, in the frame of the oscillator, which
// turns the conjugate of the sample before, and the DiscReceiver core.
type iqLowRx struct {
	d DiscReceiver
	// q is the conjugate of the last sample turned by step.
	q complex64
	// acc and pow sum the phase steps and the power of the samples over
	// the block of the oscillator, in its frame.
	acc complex64
	pow float32
	// afcAvg is the mean of the sums of the blocks since the oscillator
	// last moved, in its frame.
	afcAvg complex64
	// step turns a sample by the oscillator, hz, per sample.
	step complex64
	hz   float32
	afcN int16
	n    uint8
	buf  [iqLowBuf]int16
}

// init sets up r for I/Q at hz with the settings opt, nil for those of
// discOptions.
func (r *iqLowRx) init(hz int, on func(*Burst), opt *options) error {
	*r = iqLowRx{step: 1}
	return r.d.Init(DiscConfig{Hz: hz, OnBurst: on, opt: opt})
}

// newIQLowRx returns the receiver of New for cfg at a BasebandHz of the
// core. Without OnBurst the core keeps the bursts of a call in d.out.
func newIQLowRx(cfg Config, opt options) *iqLowRx {
	r := &iqLowRx{}
	if err := r.init(cfg.BasebandHz, cfg.OnBurst, &opt); err != nil {
		panic(err)
	}
	if cfg.OnBurst == nil {
		r.d.out = make([]Burst, 0, r.d.maxBursts(cfg.MaxWrite))
	}
	return r
}

// block returns the number of samples of the block of the oscillator,
// iqAFCBlock at basebandRate.
func (r *iqLowRx) block() int { return iqAFCBlock * r.d.hz() / basebandRate }

// writeBaseband processes the samples of Receiver.WriteBaseband.
func (r *iqLowRx) writeBaseband(x []complex64) []Burst {
	r.d.out = r.d.out[:0]
	block := r.block()
	for len(x) > 0 {
		m := min(len(x), iqLowBuf-int(r.n), block-int(r.afcN))
		r.halfRun(x[:m])
		x = x[m:]
		r.n += uint8(m)
		r.afcN += int16(m)
		if r.n == iqLowBuf || int(r.afcN) == block {
			r.d.write(r.buf[:r.n])
			r.n = 0
		}
		if int(r.afcN) == block {
			r.afcN = 0
			r.follow()
		}
	}
	return r.d.out
}

// recip returns about 1/e for e above 0: the bit estimate and two Newton
// steps, within 0.02%.
func recip(e float32) float32 {
	v := math.Float32frombits(0x7EF311C3 - math.Float32bits(e))
	v *= 2 - e*v
	return v * (2 - e*v)
}

// halfRun appends the phase step of each sample of x to buf, which has
// room for them: twice the arctangent of the half angle N/D, with N the
// cross product and D the magnitude plus the dot product of the sample
// and the one before, as N D / (D² + 0.28 N²), which needs one
// reciprocal and holds for every N and D.
//
//go:noinline
func (r *iqLowRx) halfRun(x []complex64) {
	out := r.buf[r.n : int(r.n)+len(x)]
	qr, qi := real(r.q), imag(r.q)
	sr, si := real(r.step), imag(r.step)
	ar, ai, pow := real(r.acc), imag(r.acc), r.pow
	k := 2 * float32(r.d.hz()) / (2 * math.Pi) * iqUnitsPerHz
	for i, v := range x {
		yr, yi := real(v), imag(v)
		d := yr*qr - yi*qi
		c := yr*qi + yi*qr
		dd := sqrtf(c*c+d*d) + d
		e := dd*dd + 0.28*c*c + 1e-30
		out[i] = int16(roundBits(c * dd * recip(e) * k))
		ar += d
		ai += c
		pow += yr*yr + yi*yi
		qr, qi = yr*sr+yi*si, yr*si-yi*sr
	}
	r.q = complex(qr, qi)
	r.acc, r.pow = complex(ar, ai), pow
}

// follow is iq0Rx.follow for iqLowRx, whose sums are in the frame of the
// oscillator. The core has taken the samples of the block before it runs.
// It is not inlined, so that its frame is not on the stack while the core
// runs.
//
//go:noinline
func (r *iqLowRx) follow() {
	acc := r.acc
	p := r.pow
	r.acc, r.pow = 0, 0
	rate := float32(r.d.hz())
	est := atan2f(imag(acc), real(acc)) * (rate / (2 * math.Pi))
	quiet := real(acc)*real(acc)+imag(acc)*imag(acc) < iq0Coherence*iq0Coherence*p*p
	switch {
	case quiet:
	case r.afcAvg == 0:
		r.afcAvg = acc
	default:
		r.afcAvg += complex(iqAFCAvgGain, 0) * (acc - r.afcAvg)
	}
	avg := atan2f(imag(r.afcAvg), real(r.afcAvg)) * (rate / (2 * math.Pi))
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	scale := f.gain * iqUnitsPerHz
	if r.d.locked {
		hz := (f.dc + r.d.dcHz) / scale
		switch {
		case abs32(avg) >= iqAFCPullHz && abs32(hz) >= iqAFCLockHz && hz*avg > 0:
			r.afcAvg = 0
			r.tune(avg, f.dc-avg*scale)
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < iqAFCPullHz:
			r.tune(hz, f.dc-hz*scale)
		}
		return
	}
	switch {
	case quiet:
	case abs32(est) >= iqAFCPullHz:
		r.afcAvg = 0
		r.tune(est, 0)
	case abs32(avg) >= iqAFCFineHz:
		r.tune(avg, 0)
	}
}

// tune is iq0Rx.tune for iqLowRx.
func (r *iqLowRx) tune(hz, dc float32) {
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	r.hz += hz
	rate := float32(r.d.hz())
	s, c := sincosf(-2 * math.Pi * r.hz / rate)
	r.step = complex(c, s)
	s, c = sincosf(-2 * math.Pi * hz / rate)
	r.afcAvg *= complex(c, s)
	d := hz*scale + dc - f.dc
	if r.d.locked {
		f.mean += dc - f.dc
	} else {
		f.mean = dc
	}
	f.dc, r.d.tuneHz = dc, dc
	f.refresh()
	if !r.d.locked {
		f.shiftHist(d)
		r.d.shift(d)
	}
}

// flush finishes the stream at its end.
func (r *iqLowRx) flush() []Burst {
	r.d.out = r.d.out[:0]
	r.d.write(r.buf[:r.n])
	r.n = 0
	r.d.flush()
	return r.d.out
}
