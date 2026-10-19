package rf

import (
	"math"
	"math/bits"
	"unsafe"
)

const (
	// iqLowBuf is the number of discriminator samples that iqLowRx hands
	// the core at once.
	iqLowBuf = 32
	// iqLowPack is the number of complex64 samples that
	// iqLowRx.writeBaseband packs at once on its stack.
	iqLowPack = 16
	// iqLowScale is the value of 1 in the pairs of int16 that
	// iqLowRx.writeBaseband makes of complex64 samples.
	iqLowScale = 4096
	// iqLowShift is the right shift of the products that iqLowRx sums over
	// the block of the oscillator, so that a block at 12 kHz at full
	// scale does not overflow.
	iqLowShift = 10
	// iqLowISI is the part of the levels of the symbols before and after
	// that cancelISI takes from each symbol. The phase step over a sample
	// at 9.6 kHz averages the frequency over half a symbol, and a radio
	// chip that decimates to 9.6 kHz cuts the band of a signal off its
	// center, which spread each symbol into its neighbors.
	iqLowISI = 0.04
	// iqLowAtanNear takes the angle of halfRun from iqLowAtan64, without
	// interpolation.
	iqLowAtanNear = false
	// iqLowRecip takes the ratio of halfRun by a multiply with
	// iqLowRecip64 in place of a division, and its angle as
	// iqLowAtanNear does.
	iqLowRecip = false
)

// iqLowRecip64 holds, for i from 0 to 63, 2^21/(65+i) rounded down: 2^46
// over the top of the step i of 2^25 above 2^31.
var iqLowRecip64 = [64]uint16{
	32263, 31775, 31300, 30840, 30393, 29959, 29537, 29127, 28728, 28339, 27962, 27594, 27235, 26886, 26546, 26214,
	25890, 25575, 25266, 24966, 24672, 24385, 24105, 23831, 23563, 23301, 23045, 22795, 22550, 22310, 22075, 21845,
	21620, 21399, 21183, 20971, 20763, 20560, 20360, 20164, 19972, 19784, 19599, 19418, 19239, 19065, 18893, 18724,
	18558, 18396, 18236, 18078, 17924, 17772, 17623, 17476, 17331, 17189, 17050, 16912, 16777, 16644, 16513, 16384,
}

// iqLowAtan64 holds, for i from 0 to 63, the arctangent of (i+0.5)/64 in
// units of 2^-16 of a turn.
var iqLowAtan64 = [64]uint16{
	81, 244, 407, 570, 732, 894, 1056, 1217, 1377, 1537, 1696, 1854, 2012, 2168, 2324, 2478,
	2632, 2784, 2935, 3085, 3233, 3380, 3526, 3670, 3813, 3955, 4095, 4233, 4370, 4505, 4639, 4771,
	4901, 5030, 5157, 5282, 5406, 5528, 5649, 5768, 5885, 6000, 6114, 6227, 6337, 6446, 6554, 6660,
	6764, 6867, 6968, 7068, 7166, 7262, 7358, 7451, 7544, 7635, 7724, 7812, 7899, 7984, 8068, 8151,
}

// iqLowAtan holds, for i from 0 to 15, the arctangent of i/16 in units of
// 2^-16 of a turn in the low half, and the step to that of (i+1)/16 in
// the high half.
var iqLowAtan = [16]uint32{
	0x028B0000, 0x0286028B, 0x027C0511, 0x026E078D, 0x025C09FB, 0x02470C57, 0x02300E9E, 0x021610CE,
	0x01FC12E4, 0x01E214E0, 0x01C816C2, 0x01AE188A, 0x01951A38, 0x017D1BCD, 0x01661D4A, 0x01501EB0,
}

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
// complete to OnBurst. It takes 1 as 4096 of WriteIQ16 and clips each part
// at ±8.
func (r *LowIQReceiver) WriteBaseband(x []complex64) { r.r.writeBaseband(x) }

// WriteIQ16 processes the samples x, as a radio chip delivers them, and
// hands the bursts that they complete to OnBurst. Each word holds I in its
// low 16 bits and Q in its high 16 bits, two's complement: in the memory
// of a little-endian chip, the pairs of int16 I, Q. The scale does not
// matter, but I and Q must be above -32768.
func (r *LowIQReceiver) WriteIQ16(x []uint32) {
	r.r.d.out = r.r.d.out[:0]
	r.r.run(x)
}

// Flush finishes the stream at its end.
func (r *LowIQReceiver) Flush() { r.r.flush() }

// iqLowRx is the receiver of LowIQReceiver, and of New with options.IQLow:
// the phase step of each sample less the step of the oscillator, and the
// DiscReceiver core.
type iqLowRx struct {
	// d is the first field: iqLowOf finds the receiver from its core.
	d DiscReceiver
	// q is the last sample.
	q uint32
	// ar, ai and pow sum the products of each sample and the conjugate of
	// the one before, and the power of the samples, over the block of the
	// oscillator, shifted by iqLowShift.
	ar, ai, pow int32
	// afcAvg is the mean of the sums of the blocks since the oscillator
	// last moved, in its frame.
	afcAvg complex64
	// step is the phase step of the oscillator, hz, per sample, in units
	// of 2^-32 of a turn, a multiple of 2^16.
	step int32
	hz   float32
	afcN int16
	n    uint8
	buf  [iqLowBuf]int16
}

// init sets up r for I/Q at hz with the settings opt, nil for those of
// discOptions.
func (r *iqLowRx) init(hz int, on func(*Burst), opt *options) error {
	*r = iqLowRx{}
	err := r.d.Init(DiscConfig{Hz: hz, OnBurst: on, opt: opt})
	r.d.iqLow = hz == iq0LowHz
	return err
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

// writeBaseband processes the samples of Receiver.WriteBaseband as the
// pairs of WriteIQ16, with 1 as iqLowScale.
func (r *iqLowRx) writeBaseband(x []complex64) []Burst {
	r.d.out = r.d.out[:0]
	var b [iqLowPack]uint32
	for len(x) > 0 {
		m := min(len(x), len(b))
		for i, v := range x[:m] {
			b[i] = uint32(uint16(q15(real(v)))) | uint32(q15(imag(v)))<<16
		}
		r.run(b[:m])
		x = x[m:]
	}
	return r.d.out
}

// q15 returns v times iqLowScale, rounded and clipped to ±32767.
func q15(v float32) int32 {
	return roundBits(min(max(v*iqLowScale, -32767), 32767))
}

// run processes the samples x and adds the bursts they complete to d.out.
func (r *iqLowRx) run(x []uint32) {
	block := r.block()
	if r.d.iqLow && r.d.front96 {
		for len(x) > 0 {
			m := min(len(x), block-int(r.afcN))
			r.feed(x[:m])
			x = x[m:]
			r.afcN += int16(m)
			if int(r.afcN) == block {
				r.afcN = 0
				r.follow()
			}
		}
		return
	}
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
}

// feed runs the core on x, which fill hands it through buf, and decodes
// the slots that end, as DiscReceiver.run does.
func (r *iqLowRx) feed(x []uint32) {
	r.n = 0
	j := 0
	for {
		i, m := r.d.run96(r.buf[:r.n], j, x)
		if !r.d.slotDue {
			r.n = 0
			return
		}
		r.d.slotDue = false
		r.d.endSlot()
		x, j = x[m:], i
	}
}

// iqLowOf returns the iqLowRx whose core is r, for a core with iqLow.
func iqLowOf(r *DiscReceiver) *iqLowRx { return (*iqLowRx)(unsafe.Pointer(r)) }

// fill puts the phase steps of the first samples of x into buf for
// run96 and returns their number, 0 for no x. It is not inlined, so that
// run96 keeps a small frame.
//
//go:noinline
func (r *iqLowRx) fill(x []uint32) int {
	m := min(len(x), iqLowBuf)
	r.n = 0
	r.halfRun(x[:m])
	r.n = uint8(m)
	return m
}

// halfRun appends the phase step of each sample of x, less the step of
// the oscillator, to buf, which has room for them. The dot product d and
// the cross product c of the sample and the one before are DSP dual
// multiplies. The angle is that of the octant of (|d|, |c|), from the
// ratio of the smaller to the larger, both shifted to the top bit, below
// 1, by a table of 16 steps with linear interpolation, within 0.0004 rad,
// and the fold to the full turn. Less the oscillator, it wraps in the top
// 16 bits of a word, and buf holds it as the frequency in iqUnitsPerHz.
//
//go:noinline
func (r *iqLowRx) halfRun(x []uint32) {
	out := r.buf[r.n : int(r.n)+len(x)]
	q, step := r.q, r.step
	ar, ai, pow := r.ar, r.ai, r.pow
	k := 2 * int32(r.d.hz())
	for i, y := range x {
		d := smuad(y, q)
		c := smusdx(q, y)
		q = y
		ax, ay := uint32(d), uint32(c)
		if d < 0 {
			ax = -ax
		}
		if c < 0 {
			ay = -ay
		}
		sw := ay > ax
		if sw {
			ax, ay = ay, ax
		}
		z := uint(bits.LeadingZeros32(ax | 1))
		var a int32
		if iqLowRecip {
			// ax<<z is at least 2^31: its 6 bits below the top bit pick
			// the reciprocal of the top of its step, so that the ratio
			// stays below 1.
			x := ax << z
			t := ((ay << z) >> 16 * uint32(iqLowRecip64[x>>25&63])) >> 24
			a = int32(iqLowAtan64[t&63])
		} else if t := (ay << z) / ((ax<<z)>>16 + 1); iqLowAtanNear {
			a = int32(iqLowAtan64[t>>10&63])
		} else {
			w := iqLowAtan[t>>12&15]
			a = int32(w&0xFFFF) + int32(w>>16)*int32(t&0xFFF)>>12
		}
		if sw {
			a = 1<<14 - a
		}
		if d < 0 {
			a = 1<<15 - a
		}
		if c < 0 {
			a = -a
		}
		out[i] = int16(smmulr(a<<16-step, k))
		ar += d >> iqLowShift
		ai += c >> iqLowShift
		pow += smuad(y, y) >> iqLowShift
	}
	r.q = q
	r.ar, r.ai, r.pow = ar, ai, pow
}

// follow is iq0Rx.follow for iqLowRx, with its sums turned to the frame of
// the oscillator. The core has taken the samples of the block before it runs.
// It is not inlined, so that its frame is not on the stack while the core
// runs.
//
//go:noinline
func (r *iqLowRx) follow() {
	s, c := sincosf(float32(r.step) * (-2 * math.Pi / (1 << 32)))
	acc := complex(float32(r.ar), float32(r.ai)) * complex(c, s)
	p := float32(r.pow)
	r.ar, r.ai, r.pow = 0, 0, 0
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
	r.step = roundBits(r.hz/rate*(1<<16)) << 16
	s, c := sincosf(-2 * math.Pi * hz / rate)
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
	if !r.d.iqLow || !r.d.front96 {
		r.d.write(r.buf[:r.n])
	}
	r.n = 0
	r.d.flush()
	return r.d.out
}
