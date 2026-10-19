package fsktx

import (
	"errors"
	"math"
)

// Tier selects a modulator and its cost. The zero value TierDefault is
// Tier1.
type Tier int

// The tiers, from the Pareto front of the transmit sweep.
const (
	TierDefault Tier = iota
	// Tier0 shapes by a fixed-point multiply and accumulate over taps in
	// read-only memory, the modulators MCUAudio and MCUIQ, for a
	// microcontroller.
	Tier0
	// Tier1 shapes by multiply and accumulate in float32 over eight
	// symbols on each side.
	Tier1
	// Tier2 shapes in float64 over sixteen symbols on each side.
	Tier2
)

var (
	errTier  = errors.New("fsktx: unknown tier")
	errPred  = errors.New("fsktx: ShapePred needs Exact and the pulse of its kernels")
	errHz    = errors.New("fsktx: sample rate out of range")
	errDelay = errors.New("fsktx: Delay must be from 0 to 64 symbols")
	errNoise = errors.New("fsktx: NoisePoleR must be from 0 to below 1")
)

const polyMaxPhases = 1024

type poly struct {
	taps32 []float32
	taps64 []float64
	ring32 []float32
	ring64 []float64
	n      int
	pos    int
	l, m   int
	acc    int
	peak   float64
}

func newPoly(pu pulse, hz int, f32 bool) (*poly, error) {
	l, m := ratio(hz)
	if l > polyMaxPhases || hz < 8000 {
		return nil, errHz
	}
	k := &poly{n: 2 * pu.span, l: l, m: m}
	h := pu.taps(l, m)
	g := pu.gain
	if g == 0 {
		g = 1
	}
	for i := range h {
		h[i] *= DeviationHz * g
	}
	if f32 {
		k.taps32 = make([]float32, len(h))
		for i, v := range h {
			k.taps32[i] = float32(v)
			h[i] = float64(k.taps32[i])
		}
		k.ring32 = make([]float32, 2*k.n)
	} else {
		k.taps64 = h
		k.ring64 = make([]float64, 2*k.n)
	}
	for p := range l {
		var sum float64
		for _, v := range h[p*k.n : p*k.n+k.n] {
			sum += math.Abs(v)
		}
		k.peak = max(k.peak, 3*sum)
	}
	return k, nil
}

func (k *poly) push(s int8) {
	switch s {
	case -3, -1, 1, 3:
	default:
		s = 0
	}
	k.pos--
	if k.pos < 0 {
		k.pos = k.n - 1
	}
	if k.taps32 != nil {
		k.ring32[k.pos] = float32(s)
		k.ring32[k.pos+k.n] = float32(s)
	} else {
		k.ring64[k.pos] = float64(s)
		k.ring64[k.pos+k.n] = float64(s)
	}
}

func (k *poly) symbol(dst []float64, s int8) int {
	k.push(s)
	n := 0
	if k.taps32 != nil {
		w := k.ring32[k.pos : k.pos+k.n]
		for ; k.acc < k.l; k.acc += k.m {
			t := k.taps32[k.acc*k.n : k.acc*k.n+k.n]
			var y float32
			for i, c := range t {
				y += c * w[i]
			}
			dst[n] = float64(y)
			n++
		}
	} else {
		w := k.ring64[k.pos : k.pos+k.n]
		for ; k.acc < k.l; k.acc += k.m {
			t := k.taps64[k.acc*k.n : k.acc*k.n+k.n]
			var y float64
			for i, c := range t {
				y += c * w[i]
			}
			dst[n] = y
			n++
		}
	}
	k.acc -= k.l
	return n
}

// AudioConfig sets up an Audio.
type AudioConfig struct {
	// Hz is the sample rate of the output. Tier0 takes 24000 or 48000;
	// Tier1 and Tier2 take any rate from 8000 whose ratio to 4800 has a
	// numerator of at most 1024 in lowest terms, such as 8000 or 44100.
	// 0 means 48000.
	Hz int
	// Level is the output for a run of symbol +3, 1944 Hz of deviation,
	// from 1 to 16000; 0 means 15000. Offset is added to each sample, as an
	// unsigned DAC needs. The worst run of symbols reaches up to 1.96 times
	// Level from Offset, which must fit int16. A 12-bit DAC takes Level
	// 1040 and Offset 2048 at any tier.
	Level, Offset int
	// Tier selects the modulator. The zero value TierDefault is Tier1.
	Tier Tier
	// Exact makes the pulse of Tier1 and Tier2 a Nyquist pulse after a
	// receive root-raised-cosine filter, with the gain of a run of one
	// symbol at the symbol times, through the zero-order hold of a DAC at
	// Hz, which the pulse inverts.
	Exact bool
	// Pulse sets the pulse of Tier1 and Tier2; its zero value is the
	// pulse of the tier.
	Pulse PulseConfig
	// Shape is the noise shaping of the rounding to codes: 0 rounds each
	// sample, 1 to 4 feed the rounding error back so that its spectrum has
	// that many pairs of zeros, at the frequencies of NoiseZerosHz.
	Shape int
	// NoiseZerosHz are the frequencies of the zeros of the noise transfer
	// function of Shape, in Hz.
	NoiseZerosHz [4]float64
	// Opt is the number of iterations per block of the out-of-band
	// optimizer of Tier1 and Tier2, which models the radio as a DAC hold,
	// a two-pole Butterworth low pass at 8 kHz and FM at 96 kHz. It works
	// where 96000 is Hz times a power of two, and is off at other rates
	// and for 0.
	Opt int
	// Corr is the length in symbols, 5 or 6, of the symbol context of the
	// generated table of deviation corrections of Tier1 and Tier2, fitted
	// to the output of Opt at 30 iterations for the pulse corrPulse of
	// corrgen_test.go at 24000 and 48000 Hz. Other rates interpolate the
	// table of 48000 Hz linearly. 0 is off.
	Corr int
	// CorrHiHz is the top of the band pass of Corr; 0 means none.
	CorrHiHz float64
	// CorrTaps is the length of the band pass of Corr, odd, under 512; 0
	// means 161 at 48000 Hz, scaled by Hz.
	CorrTaps int
}

// PulseConfig sets the transmit pulse of Tier1 and Tier2.
type PulseConfig struct {
	// Span is the half-length of the pulse in symbols; 0 means that of the
	// tier.
	Span int
	// Window is the taper: 0 none, 1 Hamming, 2 Blackman, 3 Kaiser of
	// shape Beta.
	Window int
	Beta   float64
	// Comp is the weight of the three-tap inverse-sinc for the zero-order
	// hold of the DAC; 1/24 matches it at low frequency.
	Comp float64
	// Gain scales the pulse; 0 means 1.
	Gain float64
	// Shape, when set, is the pulse itself, sampled at PerSymbol points per
	// symbol from -Span to Span symbols, in place of the tapered
	// root-raised-cosine.
	Shape     []float64
	PerSymbol int
	// Flat, with Exact, makes the gain of the pulse and what follows it at
	// 1200 Hz, the tone of the deviation test pattern, that at 0 Hz, so the
	// pattern has the peak deviation of the specification.
	Flat bool
}

// pulseOf returns the pulse of the tier t with c applied.
func pulseOf(t Tier, c PulseConfig) pulse {
	pu := pulse{span: 8}
	if t == Tier2 {
		pu.span = 16
	}
	if c.Span > 0 {
		pu.span = c.Span
	}
	pu.win, pu.beta, pu.comp, pu.gain = window(c.Window), c.Beta, c.Comp, c.Gain
	pu.flat = c.Flat
	if len(c.Shape) == 2*pu.span*c.PerSymbol+1 && c.PerSymbol > 0 {
		pu.tab, pu.per = c.Shape, c.PerSymbol
	}
	return pu
}

// Audio modulates the deviation signal that drives the modulator input of
// an FM radio, as int16 samples. NewAudio allocates its state, and its
// methods allocate nothing but the growth of dst. An Audio is not safe
// for concurrent use.
type Audio struct {
	mcu    MCUAudio
	k      *poly
	buf    []float64
	tmp    [maxSamples]int16
	scale  float64
	offset float64
	fb     [8]float64
	err    [8]float64
	shape  int
	opt    *aoptimizer
	corr   []int16
	corrC  int
	corrK  int
	spp    int
	step   float64
	pos    float64
	hist   [32]int8
	cbp    []float64
	cf, cc [512]float64
	cn     int
}

// NewAudio returns an Audio for cfg. It allocates the Audio, once.
func NewAudio(cfg AudioConfig) (*Audio, error) {
	if cfg.Hz == 0 {
		cfg.Hz = 48000
	}
	if cfg.Level == 0 {
		cfg.Level = tableUnits
	}
	if cfg.Level < 1 || cfg.Level > 16000 {
		return nil, errLevel
	}
	a := &Audio{scale: float64(cfg.Level) / (3 * DeviationHz), offset: float64(cfg.Offset)}
	switch cfg.Tier {
	case Tier0:
		t := Table24k()
		switch cfg.Hz {
		case 24000:
		case 48000:
			t = Table48k()
		default:
			return nil, errHz
		}
		if err := a.mcu.Init(t, cfg.Level, cfg.Offset); err != nil {
			return nil, err
		}
		return a, nil
	case TierDefault, Tier1, Tier2:
		pu := pulseOf(cfg.Tier, cfg.Pulse)
		if cfg.Exact {
			pu.nyquist()
			pu.invert(float64(cfg.Hz), holdChain(float64(cfg.Hz)))
		}
		k, err := newPoly(pu, cfg.Hz, cfg.Tier != Tier2)
		if err != nil {
			return nil, err
		}
		if x := k.peak * a.scale; a.offset+x > math.MaxInt16 || a.offset-x < math.MinInt16 {
			return nil, errOffset
		}
		a.k = k
		a.buf = make([]float64, k.l/k.m+1)
		a.shape = min(max(cfg.Shape, 0), 4)
		a.fb = ntf(a.shape, cfg.NoiseZerosHz, float64(cfg.Hz))
		if cfg.Corr > 0 {
			t, skip := corrTable(cfg.Hz, cfg.Corr)
			a.spp, a.step = cfg.Hz/SymbolRate, 1
			if t == nil {
				t, skip = corrTable(48000, cfg.Corr)
				a.spp, a.step = 48000/SymbolRate, 48000/float64(cfg.Hz)
			}
			if t != nil {
				taps := cfg.CorrTaps
				if taps <= 0 {
					taps = (optLPN*cfg.Hz/48000)/2*2 + 1
				}
				if taps%2 == 0 || taps >= len(a.cc) {
					taps = optLPN
				}
				a.corr, a.corrC, a.corrK = t, cfg.Corr, skip
				a.cbp = bandPass(optLPHz, cfg.CorrHiHz, float64(cfg.Hz), taps)
			}
		}
		if cfg.Opt > 0 && aoptRate(cfg.Hz) {
			a.opt = newAudioOpt(cfg.Opt, cfg.Hz)
		}
		return a, nil
	default:
		return nil, errTier
	}
}

// AppendSymbol shapes the symbol s, -3, -1, +1, +3 or 0 for no
// modulation, and appends the samples of the symbol period that it
// completes to dst. A symbol other than these counts as 0. The output lags
// the symbols by the half-length of the pulse: 8 symbol periods at 24 kHz
// and 6 at 48 kHz for Tier0, 8 for Tier1 and 16 for Tier2. Twice that many
// symbols 0 after the last bring its tail to rest. It allocates only to
// grow dst.
func (a *Audio) AppendSymbol(dst []int16, s int8) []int16 {
	if a.k == nil {
		n := a.mcu.Symbol(a.tmp[:], s)
		return append(dst, a.tmp[:n]...)
	}
	n := a.k.symbol(a.buf, s)
	if a.corr != nil {
		a.addCorr(s, n)
	}
	if a.opt != nil {
		a.opt.process(a.buf[:n])
	}
	if a.shape == 0 {
		for _, v := range a.buf[:n] {
			dst = append(dst, sat16(v*a.scale+a.offset))
		}
		return dst
	}
	for _, v := range a.buf[:n] {
		x := v*a.scale + a.offset
		for i, f := range a.fb[:2*a.shape] {
			x += f * a.err[i]
		}
		c := sat16(x)
		e := float64(c) - x
		if e > 1 || e < -1 {
			e = 0
		}
		copy(a.err[1:], a.err[:7])
		a.err[0] = e
		dst = append(dst, c)
	}
	return dst
}

// addCorr adds the correction of the table Corr, band-passed from
// optInHz to CorrHiHz, to the n samples in buf after the symbol s, and
// delays the samples by the half-length of the band pass.
func (a *Audio) addCorr(s int8, n int) {
	copy(a.hist[1:], a.hist[:len(a.hist)-1])
	a.hist[0] = s
	idx := 0
	for i := a.corrK + a.corrC - 1; i >= a.corrK; i-- {
		var d int
		switch a.hist[i] {
		case -3:
		case -1:
			d = 1
		case 1:
			d = 2
		case 3:
			d = 3
		default:
			idx = -1
		}
		if idx < 0 {
			break
		}
		idx = idx<<2 | d
	}
	h := len(a.cbp) / 2
	m := len(a.cc) - 1
	for i := range n {
		var c float64
		if p := a.pos; idx >= 0 && p < float64(a.spp) {
			j := int(p)
			v := float64(a.corr[idx*a.spp+j])
			if f := p - float64(j); f > 0 {
				v += f * (float64(a.corr[idx*a.spp+min(j+1, a.spp-1)]) - v)
			}
			c = v * (1.0 / corrUnits)
		}
		a.pos += a.step
		a.cf[a.cn&m] = a.buf[i]
		a.cc[a.cn&m] = c
		acc := a.cbp[h] * a.cc[(a.cn-h)&m]
		for k, v := range a.cbp[:h] {
			acc += v * (a.cc[(a.cn-k)&m] + a.cc[(a.cn-2*h+k)&m])
		}
		a.buf[i] = a.cf[(a.cn-h)&m] + acc
		a.cn++
	}
	a.pos = max(0, a.pos-float64(a.spp))
}

// corrUnits is the scale of the correction tables, in steps per Hz.
const corrUnits = 128

// corrTable returns the correction table of Corr for hz and syms context
// symbols, and the symbols from its newest to the newest symbol; nil when
// there is none.
func corrTable(hz, syms int) ([]int16, int) {
	switch {
	case hz == 24000 && syms == 5:
		return corr24k5[:], corr24k5Skip
	case hz == 24000 && syms == 6:
		return corr24k6[:], corr24k6Skip
	case hz == 48000 && syms == 5:
		return corr48k5[:], corr48k5Skip
	case hz == 48000 && syms == 6:
		return corr48k6[:], corr48k6Skip
	}
	return nil, 0
}

// bandPass returns the taps of a Kaiser-windowed band pass from lo to hi
// Hz at hz, over n taps, n odd; hi 0 means up to hz/2.
func bandPass(lo, hi, hz float64, n int) []float64 {
	lp := func(fc float64) []float64 {
		h := make([]float64, n)
		var sum float64
		c := n / 2
		for i := range h {
			t := float64(i - c)
			v := 2 * fc
			if t != 0 {
				v = math.Sin(2*math.Pi*fc*t) / (math.Pi * t)
			}
			u := t / float64(c+1)
			v *= besselI0(optLPBeta*math.Sqrt(max(0, 1-u*u))) / besselI0(optLPBeta)
			h[i] = v
			sum += v
		}
		for i := range h {
			h[i] /= sum
		}
		return h
	}
	b := lp(lo / hz)
	for i := range b {
		b[i] = -b[i]
	}
	if hi > 0 {
		for i, v := range lp(hi / hz) {
			b[i] += v
		}
	} else {
		b[n/2]++
	}
	return b
}

// ntf returns the feedback weights of the noise transfer function with n
// pairs of zeros on the unit circle at the frequencies zs at rate hz.
func ntf(n int, zs [4]float64, hz float64) [8]float64 {
	p := [9]float64{1}
	for k := range n {
		a := -2 * math.Cos(2*math.Pi*zs[k]/hz)
		for i := 2*k + 2; i >= 1; i-- {
			v := p[i] + a*p[i-1]
			if i >= 2 {
				v += p[i-2]
			}
			p[i] = v
		}
	}
	var fb [8]float64
	copy(fb[:], p[1:])
	return fb
}

// ntfMaxGain is the most gain of the noise transfer function of
// IQConfig.NoiseShape: NewIQ drops the last zeros until the gain is at most
// this, as more makes the rounding to 8 bits overload. Two pairs of zeros
// have at most 16.
const ntfMaxGain = 16

// ntfPeak returns the most gain over frequency of the noise transfer
// function 1 + sum of fb[i] z^-(i+1).
func ntfPeak(fb []float64) float64 {
	var top float64
	for k := range 513 {
		w := math.Pi * float64(k) / 512
		re, im := 1.0, 0.0
		for i, f := range fb {
			s, c := math.Sincos(-w * float64(i+1))
			re += f * c
			im += f * s
		}
		top = max(top, math.Hypot(re, im))
	}
	return top
}

// IQConfig sets up an IQ.
type IQConfig struct {
	// Rate is the sample rate of the I/Q, from 48000 to 20e6 Hz; 0 means
	// 2.4e6. Tier0 takes 48000, 96000 or 192000.
	Rate int
	// OffsetHz is the carrier offset from 0 Hz, within a quarter of Rate.
	// Tier0 rounds it to whole Hz, which must be within a quarter of Rate
	// too.
	OffsetHz float64
	// RampSeconds is the length of the power ramp of Key, up to 10 ms; 0
	// means 1 ms.
	RampSeconds float64
	// Tier selects the modulator. The zero value TierDefault is Tier1.
	Tier Tier
	// Exact makes the pulse of Tier1 and Tier2 a Nyquist pulse after a
	// receive root-raised-cosine filter, with the gain of a run of one
	// symbol at the symbol times, and the phase of the I/Q that of FM by
	// the shaped deviation: the pulse inverts the linear interpolation from
	// the shaper to Rate and the sum of the phase steps at Rate.
	Exact bool
	// Pulse sets the pulse of Tier1 and Tier2; its zero value is the
	// pulse of the tier.
	Pulse PulseConfig
	// Amplitude is the I/Q amplitude of Tier1 and Tier2 as a share of the
	// full scale of 16 bits, up to 0.995 so that the rounding to 8 bits
	// stays in range; 0 means 0.9.
	Amplitude float64
	// Shape is the number of optimizer iterations per block of Tier2 that
	// add a phase correction above 3 kHz to lower the power outside the
	// channel; 0 means none. It adds 85 symbols to the lag of the output.
	Shape int
	// ShapePred adds to Tier1 and Tier2 a fixed phase correction above
	// 3 kHz, a polynomial function of the symbols fitted to that of Shape;
	// it adds nothing to the lag. Shape wins over it. It needs Exact and
	// the pulse Span 16, Window 3, Beta 17, Flat.
	ShapePred bool
	// NoiseShape is the noise shaping of the rounding to 8 bits of
	// AppendS8: 0 rounds each value, 1 to 4 feed the rounding error of I
	// and of Q back so that its spectrum has that many pairs of zeros, at
	// the frequencies of NoiseZerosHz, less the last ones while the gain
	// of the noise is above 16.
	NoiseShape int
	// NoiseZerosHz are the frequencies of the zeros of the noise transfer
	// function of NoiseShape, in Hz.
	NoiseZerosHz [4]float64
	// NoisePoleR, from 0 to 1, puts a pole at NoisePoleR times each zero of
	// NoiseShape, so that each pair of zeros is a notch with a gain near 1
	// outside it; 0 means no poles.
	NoisePoleR float64
	// Delay holds the symbols back by that many symbols of no modulation,
	// up to maxDelay, so the carrier runs longer before the first symbol.
	Delay int
	// Rotate makes Tier1 and Tier2 turn the phase into I/Q by a complex
	// rotation at each output sample, with the sine and cosine of a 64-bit
	// phase once per symbol, in place of a table or a sine and cosine at
	// each output sample. The phase is the same sum of steps.
	Rotate bool
}

// maxDelay is the longest Delay of IQConfig, in symbols.
const maxDelay = 64

// IQ modulates complex I/Q for a software radio: it FM-modulates the
// shaped deviation at 48 kHz, interpolated linearly to the output rate,
// onto a carrier, with a raised-cosine power ramp for TDMA bursts. Tier0
// is MCUIQ; Tier1 turns a 32-bit phase into I/Q by a cosine table of 1024
// entries with linear interpolation; Tier2 by float64 sine and cosine.
// NewIQ allocates its state, and its methods allocate nothing but the
// growth of dst. An IQ is not safe for concurrent use.
type IQ struct {
	amp   float64
	mcu   MCUIQ
	out   []int16
	k     *poly
	buf   []float64
	l, m  int
	acc   int
	prev  float64
	step  float64
	off   float64
	phase uint32
	ph64  float64
	tab   []float32
	env   int
	ramp  int
	on    bool
	opt   *optimizer
	pred  *predictor
	shape int
	fb    [8]float64
	fbp   [8]float64
	pole  bool
	err   [2][8]float64
	dl    [maxDelay]int8
	dn    int
	di    int
	rot   bool
	ph    uint64
	rad   float64
	inv   float64
}

const iqShaperHz = 48000

const iqTableBits = 10

// NewIQ returns an IQ for cfg. It allocates the IQ, once.
func NewIQ(cfg IQConfig) (*IQ, error) {
	if cfg.Rate == 0 {
		cfg.Rate = 2400000
	}
	if cfg.RampSeconds == 0 {
		cfg.RampSeconds = 1e-3
	}
	switch {
	case cfg.Rate < iqShaperHz || cfg.Rate > 20e6:
		return nil, errHz
	case !(4*math.Abs(cfg.OffsetHz) < float64(cfg.Rate)):
		return nil, errCarrier
	case !(cfg.RampSeconds > 0 && cfg.RampSeconds <= 0.01):
		return nil, errRamp
	}
	ramp := int(math.Round(cfg.RampSeconds * float64(cfg.Rate)))
	if ramp < 1 {
		return nil, errRamp
	}
	if cfg.Delay < 0 || cfg.Delay > maxDelay {
		return nil, errDelay
	}
	q := &IQ{ramp: ramp, dn: cfg.Delay}
	for q.shape = min(max(cfg.NoiseShape, 0), 4); q.shape > 0; q.shape-- {
		q.fb = ntf(q.shape, cfg.NoiseZerosHz, float64(cfg.Rate))
		if ntfPeak(q.fb[:2*q.shape]) <= ntfMaxGain {
			break
		}
	}
	if cfg.NoisePoleR < 0 || cfg.NoisePoleR >= 1 {
		return nil, errNoise
	}
	if cfg.NoisePoleR > 0 && cfg.NoiseShape > 0 {
		q.shape = min(cfg.NoiseShape, 4)
		q.fb = ntf(q.shape, cfg.NoiseZerosHz, float64(cfg.Rate))
		q.pole = true
		r := cfg.NoisePoleR
		for i := range q.fbp {
			q.fbp[i] = q.fb[i] * r
			r *= cfg.NoisePoleR
		}
	}
	switch cfg.Tier {
	case Tier0:
		if err := q.mcu.Init(cfg.Rate, int(math.Round(cfg.OffsetHz)), min(ramp, 1<<15)); err != nil {
			return nil, err
		}
		q.out = make([]int16, 2*q.mcu.Samples())
		return q, nil
	case TierDefault, Tier1, Tier2:
	default:
		return nil, errTier
	}
	pu := pulseOf(cfg.Tier, cfg.Pulse)
	if cfg.Exact {
		pu.nyquist()
		pu.invert(iqShaperHz, interpChain(iqShaperHz, float64(cfg.Rate)))
	}
	k, err := newPoly(pu, iqShaperHz, cfg.Tier != Tier2)
	if err != nil {
		return nil, err
	}
	g := gcd(cfg.Rate, iqShaperHz)
	q.k, q.buf, q.l, q.m = k, make([]float64, k.l/k.m+1), cfg.Rate/g, iqShaperHz/g
	q.out = make([]int16, 2*(q.l/q.m+1)*len(q.buf))
	q.step = (1 << 32) / float64(cfg.Rate)
	q.amp = 0.9 * 32767
	if cfg.Amplitude > 0 {
		q.amp = min(cfg.Amplitude, 0.995) * 32767
	}
	q.off, q.prev = cfg.OffsetHz, cfg.OffsetHz
	if cfg.Shape > 0 && cfg.Tier == Tier2 {
		q.opt = newOptimizer(cfg.Shape)
	} else if cfg.ShapePred {
		if !cfg.Exact || !predFits(cfg.Tier, cfg.Pulse) {
			return nil, errPred
		}
		q.pred = &predictor{kern: &shapeKern}
	}
	q.rot, q.rad, q.inv = cfg.Rotate, 2*math.Pi/float64(cfg.Rate), 1/float64(cfg.Rate)
	if cfg.Tier != Tier2 {
		q.tab = make([]float32, 1<<iqTableBits+1)
		for i := range q.tab {
			q.tab[i] = float32(math.Cos(2 * math.Pi * float64(i) / (1 << iqTableBits)))
		}
	}
	return q, nil
}

// Key keys the carrier up or down: the power ramps over RampSeconds from
// where it is. An IQ starts keyed down.
func (q *IQ) Key(on bool) {
	q.on = on
	q.mcu.Key(on)
}

// AppendS8 shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation,
// and appends the I/Q of the symbol period that it completes to dst as
// signed 8-bit interleaved I/Q, as hackrf_transfer reads it, at 0.9 of
// full scale. The noise of 8 bits spreads over the rate, so this format
// meets the adjacent channel power limit only from about 1 MHz; AppendS16
// has 48 dB more room. The output lags the symbols by the half-length of
// the pulse and the interpolation. It allocates only to grow dst.
func (q *IQ) AppendS8(dst []byte, s int8) []byte {
	n := q.symbol(s)
	if q.shape == 0 {
		for _, v := range q.out[:n] {
			dst = append(dst, byte(int8((int32(v)+1<<7)>>8)))
		}
		return dst
	}
	if q.pole {
		return q.appendPole(dst, n)
	}
	for j, v := range q.out[:n] {
		e := &q.err[j&1]
		x := float64(v) / 256
		for i, f := range q.fb[:2*q.shape] {
			x += f * e[i]
		}
		c := min(max(math.Floor(x+0.5), -128), 127)
		d := c - x
		if d > 1 || d < -1 {
			d = 0
		}
		copy(e[1:], e[:7])
		e[0] = d
		dst = append(dst, byte(int8(c)))
	}
	return dst
}

// appendPole is AppendS8 with the noise transfer function A(z)/A(z/r) of
// IQConfig.NoisePoleR: e holds the rounding error filtered by 1/A(z/r).
func (q *IQ) appendPole(dst []byte, n int) []byte {
	m := 2 * q.shape
	for j, v := range q.out[:n] {
		e := &q.err[j&1]
		u := float64(v) / 256
		for i := range m {
			u += (q.fb[i] - q.fbp[i]) * e[i]
		}
		c := min(max(math.Floor(u+0.5), -128), 127)
		d := c - u
		if d > 1 || d < -1 {
			d = 0
		}
		for i := range m {
			d -= q.fbp[i] * e[i]
		}
		copy(e[1:], e[:7])
		e[0] = d
		dst = append(dst, byte(int8(c)))
	}
	return dst
}

// AppendS16 is AppendS8 with signed 16-bit interleaved I/Q, at 0.9 of
// full scale. It allocates only to grow dst.
func (q *IQ) AppendS16(dst []int16, s int8) []int16 {
	n := q.symbol(s)
	return append(dst, q.out[:n]...)
}

func (q *IQ) symbol(s int8) int {
	if q.k == nil {
		return q.mcu.Symbol(q.out, s)
	}
	if q.dn > 0 {
		s, q.dl[q.di] = q.dl[q.di], s
		q.di = (q.di + 1) % q.dn
	}
	n := q.k.symbol(q.buf, s)
	if q.opt != nil {
		q.opt.process(q.buf[:n])
	} else if q.pred != nil {
		q.pred.process(q.buf[:n], s)
	}
	if q.rot {
		return q.rotate(n)
	}
	j := 0
	for _, f := range q.buf[:n] {
		w := f + q.off
		dw := (w - q.prev) * float64(q.m) / float64(q.l)
		v := q.prev + (w-q.prev)*float64(q.acc)/float64(q.l)
		for ; q.acc < q.l; q.acc += q.m {
			q.out[j], q.out[j+1] = q.sample(v)
			j += 2
			v += dw
		}
		q.acc -= q.l
		q.prev = w
	}
	return j
}

// rotate writes the I/Q of the n shaper samples in buf to out by a complex
// rotation and returns the number of values. The carrier phasor z and the
// step r start from a sine and cosine once per call and when the carrier
// comes back on; between shaper samples r takes the change of slope by a
// small-angle rotation, and the frequency step d is one too.
func (q *IQ) rotate(n int) int {
	j := 0
	l, m := float64(q.l), float64(q.m)
	var zr, zi, rr, ri, last float64
	seeded := false
	for _, f := range q.buf[:n] {
		w := f + q.off
		dv := (w - q.prev) / l
		a0 := float64(q.acc)
		v0 := q.prev + dv*a0
		dw := dv * m
		c := (q.l - q.acc + q.m - 1) / q.m
		q.acc += c*q.m - q.l
		q.prev = w
		out := q.out[j : j+2*c]
		j += 2 * c
		if !q.on && q.env <= 0 {
			clear(out)
			seeded = false
		} else {
			if seeded {
				si, co := smallRot((dv - last) * a0 * q.rad)
				rr, ri = rr*co-ri*si, rr*si+ri*co
			} else {
				zi, zr = math.Sincos(float64(q.ph>>11) * (2 * math.Pi / (1 << 53)))
				ri, rr = math.Sincos(v0 * q.rad)
				seeded = true
			}
			last = dv
			di, dr := smallRot(dw * q.rad)
			if q.on && q.env >= q.ramp {
				a := q.amp
				for k := 0; k+1 < len(out); k += 2 {
					zr, zi = zr*rr-zi*ri, zr*ri+zi*rr
					rr, ri = rr*dr-ri*di, rr*di+ri*dr
					out[k], out[k+1] = round16(zr*a), round16(zi*a)
				}
			} else {
				for k := 0; k+1 < len(out); k += 2 {
					zr, zi = zr*rr-zi*ri, zr*ri+zi*rr
					rr, ri = rr*dr-ri*di, rr*di+ri*dr
					a := q.amp
					if q.on && q.env < q.ramp || !q.on && q.env > 0 {
						if q.on {
							q.env++
						} else {
							q.env--
						}
						a *= 0.5 - 0.5*math.Cos(math.Pi*float64(q.env)/float64(q.ramp))
					} else if !q.on {
						a = 0
					}
					out[k], out[k+1] = sat16(zr*a), sat16(zi*a)
				}
			}
		}
		x := (float64(c)*v0 + dw*float64(c*(c-1)/2)) * q.inv
		x -= math.Floor(x)
		q.ph += uint64(x*(1<<62)) << 2
	}
	return j
}

// smallRot returns the sine and cosine of x by their Taylor series to the
// 9th and 8th power; for |x| up to 0.5 the error is under 3e-10.
func smallRot(x float64) (s, c float64) {
	x2 := x * x
	s = x * (1 - x2/6*(1-x2/20*(1-x2/42*(1-x2/72))))
	c = 1 - x2/2*(1-x2/12*(1-x2/30*(1-x2/56)))
	return s, c
}

func (q *IQ) sample(hz float64) (i, qd int16) {
	a := q.amp
	if q.on && q.env < q.ramp || !q.on && q.env > 0 {
		if q.on {
			q.env++
		} else {
			q.env--
		}
		a *= 0.5 - 0.5*math.Cos(math.Pi*float64(q.env)/float64(q.ramp))
	} else if !q.on {
		a = 0
	}
	var c, s float64
	if q.tab == nil {
		q.ph64 += 2 * math.Pi * hz * q.step / (1 << 32)
		if q.ph64 > math.Pi {
			q.ph64 -= 2 * math.Pi
		} else if q.ph64 < -math.Pi {
			q.ph64 += 2 * math.Pi
		}
		s, c = math.Sincos(q.ph64)
	} else {
		q.phase += uint32(int32(math.Round(hz * q.step)))
		const sh = 32 - iqTableBits
		i := q.phase >> sh
		j := (i - 1<<iqTableBits/4) & (1<<iqTableBits - 1)
		fr := float32(q.phase<<iqTableBits>>iqTableBits) / (1 << sh)
		c = float64(q.tab[i] + (q.tab[i+1]-q.tab[i])*fr)
		s = float64(q.tab[j] + (q.tab[j+1]-q.tab[j])*fr)
	}
	return sat16(c * a), sat16(s * a)
}

// round16 rounds x, which must be within the range of int16, to the
// nearest int16, ties to even, by the rounding of an add of 1.5 times
// 2**52.
func round16(x float64) int16 {
	return int16(math.Float64bits(x + 0x1.8p52))
}

func sat16(x float64) int16 {
	switch {
	case x >= math.MaxInt16:
		return math.MaxInt16
	case x <= math.MinInt16:
		return math.MinInt16
	case x >= 0:
		return int16(x + 0.5)
	default:
		return int16(x - 0.5)
	}
}
