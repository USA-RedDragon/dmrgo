package rf

import (
	"errors"
	"math"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// ErrNotSetUp is the error of Write and Flush on a receiver that neither
// its constructor nor Init set up.
var ErrNotSetUp = errors.New("rf: receiver not set up; make it with its constructor or Init")

// AudioRxConfig sets up an AudioReceiver.
type AudioRxConfig struct {
	// Tier is Tier0, Tier1, Tier2 or Tier3. The zero value TierDefault is
	// Tier1 for NewAudioReceiver, and Tier0 for Init, which does not
	// allocate the stages of the higher tiers, and under TinyGo, which has
	// neither. Tier2 detects the bursts again coherently, on the carrier
	// phase that it rebuilds from the samples, at about 5 times the CPU
	// time of Tier1. Tier3 runs Tier2 and, beside it, a receiver with more
	// coherent CPU time and four passes over each data burst. It keeps
	// every burst of Tier2 and adds the bursts that Tier2 misses, the data
	// bursts that pass a CRC where those of Tier2 fail, and the embedded
	// LCs that pass where those of Tier2 fail, at about twice the CPU time
	// and memory of Tier2, and 3 s of delay.
	Tier Tier
	// Rate is the sample rate of the discriminator samples, in Hz, from
	// 8000 to 1000000, and for Tier0 to 48000. 0 means 48000. The
	// receiver runs at 48000, as sound cards sample, 24000, as MMDVM modems
	// sample, 12000 or 9600: at Rate when it is one of them, else at the
	// lowest of them at or above Rate, or 48000 above 48000, through a
	// resampler. Under TinyGo Rate must be one of the four. A lower rate
	// costs less: the receiver halves 48 kHz and then 24 kHz with a
	// half-band filter before its matched filter, which runs at 12 kHz or
	// 9.6 kHz.
	Rate int
	// Bits is the resolution of the converter, from 8 to 16. 0 means 16.
	// Each sample holds one code of Bits bits in its low bits.
	Bits int
	// Unsigned selects offset binary codes, from 0 to 1<<Bits-1, in place
	// of two's complement codes.
	Unsigned bool
	// OnBursts, when not nil, selects stream mode: Write and Flush hand
	// the bursts to it. Each call carries the bursts that are ready at
	// the same point of the stream, in storage of the receiver that is
	// valid only during the call: usually one, and after a late lock the
	// bursts of the slots before the lock, decoded again. When nil, the
	// receiver is in append mode, for AppendBursts and AppendFlush.
	OnBursts func([]Burst)
}

// AudioReceiver recovers DMR bursts from the output of an FM
// discriminator, as the radio chips of hotspots and MMDVM modems give it:
// real samples in any units, with any gain and DC offset, which the
// receiver measures itself. With Tier0 it keeps about 1.4 KB of state, for
// microcontrollers; on a Cortex-M4F it needs about 3.8 MHz and 1.9 KB with
// its stack, built with TinyGo -opt z. An AudioReceiver that Init sets up
// in memory of the caller, such as a package variable, uses no heap in
// stream mode. An AudioReceiver is not safe for concurrent use, and it
// must not be copied after Init or NewAudioReceiver.
type AudioReceiver struct {
	discRx
	bursts burstSink
	audioRate
	rate     int32
	bits     uint8
	unsigned bool
	set      bool
}

var (
	errAudioRate     = &dmrgo.ConfigError{Type: "rf", Field: "AudioRxConfig.Rate"}
	errAudioTier     = &dmrgo.ConfigError{Type: "rf", Field: "AudioRxConfig.Tier"}
	errAudioBits     = &dmrgo.ConfigError{Type: "rf", Field: "AudioRxConfig.Bits"}
	errAudioOnBursts = &dmrgo.ConfigError{Type: "rf", Field: "AudioRxConfig.OnBursts"}
)

// NewAudioReceiver returns an AudioReceiver of c. A nil c means the zero
// AudioRxConfig. It allocates the receiver, and in stream mode the storage
// of the bursts that it hands to OnBursts. It returns a *dmrgo.ConfigError
// when a field of c is out of range.
func NewAudioReceiver(c *AudioRxConfig) (*AudioReceiver, error) {
	var cfg AudioRxConfig
	if c != nil {
		cfg = *c
	}
	return newAudioReceiver(cfg, nil)
}

func newAudioReceiver(cfg AudioRxConfig, o *options) (*AudioReceiver, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	if cfg.Tier < Tier0 || cfg.Tier > Tier3 {
		return nil, errAudioTier
	}
	if err := cfg.check(cfg.Tier); err != nil {
		return nil, err
	}
	opt := discTierOptions(cfg.Tier)
	if o != nil {
		opt = *o
	}
	r := &AudioReceiver{}
	r.configure(cfg, &opt)
	r.designResample(cfg.Tier)
	if o == nil && opt.SoftExt {
		if err := r.addSoft(&opt); err != nil {
			return nil, err
		}
	}
	if n := streamBursts(opt.ExtLookback); cfg.OnBursts != nil && n > 0 {
		r.bursts.out = make([]Burst, 0, n)
	}
	return r, nil
}

func (c *AudioRxConfig) check(t Tier) error {
	if c.Rate != 0 && !audioRateOK(c.Rate, t) {
		return errAudioRate
	}
	if c.Bits != 0 && (c.Bits < 8 || c.Bits > 16) {
		return errAudioBits
	}
	return nil
}

func (r *AudioReceiver) configure(cfg AudioRxConfig, opt *options) {
	rate := cfg.Rate
	if rate == 0 {
		rate = 2 * discRate
	}
	native := audioNative(rate)
	hz, dec := native, native == 2*discRate
	if dec {
		hz = discRate
	}
	bits := cfg.Bits
	if bits == 0 {
		bits = 16
	}
	*r = AudioReceiver{}
	r.bits, r.unsigned, r.set, r.rate = uint8(bits), cfg.Unsigned, true, int32(rate)
	if rate != native {
		r.setResample(rate, native)
	}
	r.sink = &r.bursts
	r.setup(discSetup{Hz: hz, opt: opt}, dec)
	r.bursts.on = cfg.OnBursts
}

// Init sets up r for c, in place of any state it holds, in Tier0. It does
// not allocate, so that r can live in memory of the caller. It returns
// dmrgo.ErrNilReceiver for a nil r, and a *dmrgo.ConfigError, with r
// unchanged, when a field of c is out of range.
func (r *AudioReceiver) Init(c AudioRxConfig) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.Tier != TierDefault && c.Tier != Tier0 {
		return errAudioTier
	}
	if err := c.check(Tier0); err != nil {
		return err
	}
	r.configure(c, nil)
	return nil
}

// Write processes the discriminator samples x in stream mode and hands
// the bursts that they complete to OnBursts. It returns len(x) and nil,
// dmrgo.ErrNilReceiver for a nil r, ErrNotSetUp before Init or
// NewAudioReceiver, and a *dmrgo.ConfigError for OnBursts in append mode.
func (r *AudioReceiver) Write(x []int16) (int, error) {
	if r == nil {
		return 0, dmrgo.ErrNilReceiver
	}

	if err := r.streamErr(); err != nil {
		return 0, err
	}
	r.feed(x)
	r.bursts.deliver()
	return len(x), nil
}

// Flush ends the stream in stream mode and hands the remaining bursts to
// OnBursts. It returns the errors of Write.
func (r *AudioReceiver) Flush() error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}

	if err := r.streamErr(); err != nil {
		return err
	}
	r.flush()
	r.bursts.deliver()
	return nil
}

func (r *AudioReceiver) streamErr() error {
	switch {
	case !r.set:
		return ErrNotSetUp
	case r.bursts.on == nil:
		return errAudioOnBursts
	}
	return nil
}

// AppendBursts processes the discriminator samples x in append mode,
// appends the bursts that they complete to dst and returns the extended
// slice. It allocates only when dst has no room for r.MaxOutput(len(x))
// bursts. In stream mode, or before Init or NewAudioReceiver, it returns
// dst.
func (r *AudioReceiver) AppendBursts(dst []Burst, x []int16) []Burst {
	if r == nil || !r.set || r.bursts.on != nil {
		return dst
	}

	dst = growBursts(dst, r.MaxOutput(len(x)))
	n := len(dst)
	r.bursts.out = dst[n:n]
	r.feed(x)
	m := len(r.bursts.out)
	r.bursts.out = nil
	return dst[:n+m]
}

// AppendFlush ends the stream in append mode, appends the remaining
// bursts to dst and returns the extended slice. It allocates only when dst
// has no room for r.MaxOutput(0) bursts. In stream mode, or before Init or
// NewAudioReceiver, it returns dst.
func (r *AudioReceiver) AppendFlush(dst []Burst) []Burst {
	if r == nil || !r.set || r.bursts.on != nil {
		return dst
	}

	dst = growBursts(dst, r.MaxOutput(0))
	n := len(dst)
	r.bursts.out = dst[n:n]
	r.flush()
	m := len(r.bursts.out)
	r.bursts.out = nil
	return dst[:n+m]
}

func (r *AudioReceiver) feed(x []int16) {
	if r.bits == 16 && !r.unsigned {
		r.native(x)
		return
	}
	var buf [32]int16
	shift := 16 - r.bits
	lo, hi := -int32(1)<<(r.bits-1), int32(1)<<(r.bits-1)-1
	var bias int32
	if r.unsigned {
		bias = 1 << (r.bits - 1)
	}
	for len(x) > 0 {
		m := min(len(x), len(buf))
		for i, v := range x[:m] {
			c := int32(v)
			if r.unsigned {
				c &= 0xffff
			}
			buf[i] = int16(min(max(c-bias, lo), hi) << shift)
		}
		r.native(buf[:m])
		x = x[m:]
	}
}

func (r *AudioReceiver) native(x []int16) {
	if r.resampling() {
		r.resampled(x)
		return
	}
	r.input(x)
}

// Active reports whether the stream is inside a transmission that the
// receiver is locked to.
func (r *AudioReceiver) Active() bool {
	if r == nil || !r.set {
		return false
	}

	return r.locked
}

// Lag returns the most samples between the last sample of a burst and
// the call that hands the burst out.
func (r *AudioReceiver) Lag() int {
	if r == nil || !r.set {
		return 0
	}

	n := r.lagSamples(int(r.rate))
	if r.resampling() {
		n += r.resampleTaps()/2 + 1
	}
	return n
}

// MaxOutput returns the most bursts that AppendBursts can append for n
// samples, and for n == 0 the most that AppendFlush can append. It does
// not allocate.
func (r *AudioReceiver) MaxOutput(n int) int {
	if r == nil || !r.set {
		return 0
	}

	if n <= 0 {
		return r.flushBursts()
	}
	if r.resampling() {
		hz := int64(r.inputHz())
		if r.d48.on {
			hz *= 2
		}
		n = int(min(int64(n)*hz/int64(r.rate)+int64(r.resampleTaps()), 1<<40))
	}
	return r.maxBursts(n)
}

// Level returns the outer level of the last locked stream: the code of a
// run of symbol +3 (1944 Hz) less the DC code.
func (r *AudioReceiver) Level() int {
	if r == nil || !r.set {
		return 0
	}

	return int(math.Round(r.deviation() / float64(int32(1)<<(16-r.bits))))
}

// Offset returns the DC code of the last locked stream: the code of the
// carrier, from its offset and the offset of the converter.
func (r *AudioReceiver) Offset() int {
	if r == nil || !r.set {
		return 0
	}

	v := int(math.Round(r.offset() / float64(int32(1)<<(16-r.bits))))
	if r.unsigned {
		v += 1 << (r.bits - 1)
	}
	return v
}
