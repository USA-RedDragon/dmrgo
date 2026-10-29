package rf

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2"
)

// RxConfig sets up a Receiver.
type RxConfig struct {
	// Tier selects the detection. The zero value TierDefault is Tier1 for
	// NewReceiver, and Tier0 for Init and under TinyGo.
	Tier Tier
	// Rate is the sample rate of the I/Q, in Hz. 0 means 2400000. Tier0 to
	// Tier3 take a Rate from 48000 to 1e9, and Tier0 and Tier1 also one
	// from 24000, which runs their receiver with a complex front end in
	// place of the FM discriminator, with about 2 KB of state for Tier0.
	// Tier0 also takes 9600 and 12000, for I/Q that a radio chip filtered
	// to the channel, with about 1.4 KB of state; Init sets up only these.
	Rate int
	// Format is the layout of the I/Q samples. 0 means IQFormatCS16.
	Format IQFormat
	// OffsetHz is the frequency of the carrier relative to the center of
	// the I/Q, in Hz, when the oscillator of the radio is known to be off,
	// as for an RTL-SDR without ppm correction (rtl_fm -p): the offset that
	// OffsetHz of a Receiver reported for that radio. The receiver tunes
	// there before it searches 6.25 kHz around it for the carrier. It must
	// be under 24000 in size, or under Rate/2 for a Rate under 48000. 0
	// means a centered carrier.
	OffsetHz float64
	// OnBursts, when not nil, selects stream mode: Write and Flush hand
	// the bursts to it. Each call carries the bursts that are ready at
	// the same point of the stream, in storage of the receiver that is
	// valid only during the call: usually one, and after a late lock the
	// LookbackSlots bursts decoded again. When nil, the receiver is in
	// append mode, for AppendBursts and AppendFlush.
	OnBursts func([]Burst)
	// LookbackSlots is the number of 30 ms slots before the first SYNC it
	// locks to that the receiver keeps and decodes: 0 means 4 for Tier1, 8
	// for Tier2, and 16 for Tier3, and at most MaxLookbackSlots. Tier0
	// keeps none, and loses the start of a stream that it enters late.
	LookbackSlots int
	// OnShortLC, when not nil, receives each Short LC that the CACH of
	// the bursts carries and that passes its CRC-8, in stream and in
	// append mode, in storage of the receiver that is valid only during
	// the call: usually one. The receiver calls it as it outputs the burst
	// of the last fragment, before that burst reaches OnBursts or the
	// slice of AppendBursts. It decodes the fragments of the Decoded CACH
	// as layer2.ShortLCAssembler does, and from Tier1 up decodes those
	// that fail again from their soft values. Tier0 gives the hard
	// decode only. Init and TinyGo builds take no OnShortLC and return a
	// *dmrgo.ConfigError for it.
	OnShortLC func([]ShortLC)
}

type rxSetup struct {
	Rate          float64
	Signed        bool
	Format        IQFormat
	Tier          Tier
	BasebandHz    int
	OffsetHz      float64
	LookbackSlots int
	OnBurst       func(*Burst)
	sink          *burstSink
	disc          *discRx
}

func (s *rxSetup) discOf() *discRx {
	if s.disc != nil {
		return s.disc
	}
	return new(discRx)
}

type rxImpl interface {
	write(x []byte)
	writeBaseband(x []complex64)
	flush()
	active() bool
	offsetHz() float64
	deviationHz() float64
	maxOutput(n int) int
	lag() int
}

const (
	rxModeNone uint8 = iota
	rxModeLow
	rxModeFull
)

// Receiver recovers DMR bursts from a stream of I/Q samples. It locks to
// the slot timing of the first SYNC it finds, follows the timing and the
// carrier through the transmission, and gives every burst it recovers.
// Make it with NewReceiver, or set it up with Init in memory of the
// caller. A Receiver is not safe for concurrent use, and it must not be
// copied after Init or NewReceiver.
type Receiver struct {
	full   rxImpl
	low    iqLowRx
	sink   burstSink
	carry  sampleCarry
	format IQFormat
	mode   uint8
}

var (
	errRxTier     = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.Tier"}
	errRxRate     = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.Rate"}
	errRxFormat   = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.Format"}
	errRxOffset   = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.OffsetHz"}
	errRxLookback = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.LookbackSlots"}
	errRxOnBursts = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.OnBursts"}
	errRxShortLC  = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.OnShortLC"}
	errRxOpt      = &dmrgo.ConfigError{Type: "rf", Field: "RxConfig.opt"}
)

const defaultRxRate = 2400000

// MaxLookbackSlots is the largest RxConfig.LookbackSlots, 7.68 s of
// signal.
const MaxLookbackSlots = 256

// NewReceiver returns a Receiver of c. A nil c means the zero RxConfig. It
// allocates every buffer of the Receiver, and it returns a
// *dmrgo.ConfigError when a field of c is out of range.
func NewReceiver(c *RxConfig) (*Receiver, error) {
	var cfg RxConfig
	if c != nil {
		cfg = *c
	}
	return newReceiver(cfg, nil)
}

func (c *RxConfig) check(o *options) (low bool, hz int, err error) {
	if c.Tier < Tier0 || c.Tier > Tier3 {
		return false, 0, errRxTier
	}
	if c.Format == IQFormatDefault {
		c.Format = IQFormatCS16
	}
	if c.Format > IQFormatCF32 {
		return false, 0, errRxFormat
	}
	if c.Rate == 0 {
		c.Rate = defaultRxRate
	}
	switch {
	case c.Rate == iq0LowHz || c.Rate == discRate/2:
		low, hz = true, c.Rate
		if o != nil && (o.IQFront != 4 || !o.IQDisc) || o == nil && c.Tier != Tier0 {
			return false, 0, errRxRate
		}
	case o != nil && o.IQHalf, c.Rate >= discRate && c.Rate < basebandRate:
		hz = discRate
		if c.Tier > Tier1 || c.Rate < discRate || c.Rate > 1e9 {
			return false, 0, errRxRate
		}
	case c.Rate >= basebandRate && c.Rate <= 1e9:
		hz = basebandRate
	default:
		return false, 0, errRxRate
	}
	if !(math.Abs(c.OffsetHz) < float64(hz)/2) {
		return false, 0, errRxOffset
	}
	if c.LookbackSlots < 0 || c.LookbackSlots > MaxLookbackSlots || c.Tier == Tier0 && c.LookbackSlots > 0 {
		return false, 0, errRxLookback
	}
	return low, hz, nil
}

func (r *Receiver) setLow(c RxConfig, opt *options) {
	*r = Receiver{}
	r.format, r.mode = c.Format, rxModeLow
	r.low.init(c.Rate, nil, opt)
	r.low.d.sink = &r.sink
	r.low.center(float32(c.OffsetHz))
	r.sink.on = c.OnBursts
}

// Init sets up r for c, in place of any state it holds, in Tier0 at a
// Rate of 9600 or 12000. It does not allocate, so that r can live in
// memory of the caller. On a Cortex-M4F at 9.6 kHz the receiver needs
// up to 5.0 MHz and 2,248 B with its stack, built with TinyGo -opt z, over
// 2 KB by the accepted cost of the detection of standalone RC bursts and of
// the MS bursts of both TDMA slots and of the clock rate tracking. Init
// returns dmrgo.ErrNilReceiver for a nil r, and a *dmrgo.ConfigError, with
// r unchanged, when a field of c is out of range.
func (r *Receiver) Init(c RxConfig) error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.Tier == TierDefault {
		c.Tier = Tier0
	}
	if c.Tier != Tier0 {
		return errRxTier
	}
	low, _, err := c.check(nil)
	if err != nil {
		return err
	}
	if !low {
		return errRxRate
	}
	if c.OnShortLC != nil {
		return errRxShortLC
	}
	r.setLow(c, nil)
	return nil
}

// Write processes the I/Q samples x in stream mode and hands the bursts
// that they complete to OnBursts. A sample can span two calls. It returns
// len(x) and nil, dmrgo.ErrNilReceiver for a nil r, ErrNotSetUp before
// Init or NewReceiver, and a *dmrgo.ConfigError for OnBursts in append
// mode.
func (r *Receiver) Write(x []byte) (int, error) {
	if r == nil {
		return 0, dmrgo.ErrNilReceiver
	}

	if err := r.streamErr(); err != nil {
		return 0, err
	}
	r.input(x)
	r.sink.deliver()
	return len(x), nil
}

// Flush ends the stream in stream mode and hands the remaining bursts to
// OnBursts. It drops a partial sample. It returns the errors of Write.
func (r *Receiver) Flush() error {
	if r == nil {
		return dmrgo.ErrNilReceiver
	}

	if err := r.streamErr(); err != nil {
		return err
	}
	r.end()
	r.sink.deliver()
	return nil
}

func (r *Receiver) streamErr() error {
	switch {
	case r.mode == rxModeNone:
		return ErrNotSetUp
	case r.sink.on == nil:
		return errRxOnBursts
	}
	return nil
}

// AppendBursts processes the I/Q samples x in append mode, appends the
// bursts that they complete to dst and returns the extended slice. A
// sample can span two calls. It allocates only when dst has no room for
// r.MaxOutput(len(x)/r.SampleBytes()+1) bursts. In stream mode, or before
// Init or NewReceiver, it returns dst.
func (r *Receiver) AppendBursts(dst []Burst, x []byte) []Burst {
	if r == nil || r.mode == rxModeNone || r.sink.on != nil {
		return dst
	}

	dst = growBursts(dst, r.MaxOutput(len(x)/r.format.bytes()+1))
	n := len(dst)
	r.sink.out = dst[n:n]
	r.input(x)
	m := len(r.sink.out)
	r.sink.out = nil
	return dst[:n+m]
}

// AppendFlush ends the stream in append mode, appends the remaining
// bursts to dst and returns the extended slice. It drops a partial sample.
// It allocates only when dst has no room for r.MaxOutput(0) bursts. In
// stream mode, or before Init or NewReceiver, it returns dst.
func (r *Receiver) AppendFlush(dst []Burst) []Burst {
	if r == nil || r.mode == rxModeNone || r.sink.on != nil {
		return dst
	}

	dst = growBursts(dst, r.MaxOutput(0))
	n := len(dst)
	r.sink.out = dst[n:n]
	r.end()
	m := len(r.sink.out)
	r.sink.out = nil
	return dst[:n+m]
}

func (r *Receiver) input(x []byte) {
	size := r.format.bytes()
	if r.carry.n > 0 && len(x) > 0 {
		var done bool
		if x, done = r.carry.fill(x, size); !done {
			return
		}
		r.whole(r.carry.bytes()[:size])
	}
	m := len(x) % size
	r.whole(x[:len(x)-m])
	r.carry.keep(x[len(x)-m:])
}

func (r *Receiver) whole(x []byte) {
	if len(x) == 0 {
		return
	}
	if r.mode == rxModeLow {
		r.low.write(x, r.format, &r.carry.held)
		return
	}
	r.full.write(x)
}

func (r *Receiver) end() {
	r.carry.n = 0
	if r.mode == rxModeLow {
		r.low.flush()
		return
	}
	r.full.flush()
}

// Active reports whether the stream is inside a transmission that the
// receiver is locked to.
func (r *Receiver) Active() bool {
	if r == nil {
		return false
	}

	switch r.mode {
	case rxModeLow:
		return r.low.d.locked
	case rxModeFull:
		return r.full.active()
	}
	return false
}

// Lag returns the most samples between the last sample of a burst and
// the call that hands the burst out.
func (r *Receiver) Lag() int {
	if r == nil {
		return 0
	}

	switch r.mode {
	case rxModeLow:
		return r.low.d.lagSamples(r.low.d.hz())
	case rxModeFull:
		return r.full.lag()
	}
	return 0
}

// MaxOutput returns the most bursts that AppendBursts can append for n
// samples, and for n == 0 the most that AppendFlush can append. It does
// not allocate.
func (r *Receiver) MaxOutput(n int) int {
	if r == nil {
		return 0
	}

	switch r.mode {
	case rxModeLow:
		if n <= 0 {
			return r.low.d.flushBursts()
		}
		return r.low.d.maxBursts(n)
	case rxModeFull:
		return r.full.maxOutput(max(n, 0))
	}
	return 0
}

// SampleBytes returns the bytes of one I/Q sample in the Format of r.
func (r *Receiver) SampleBytes() int {
	if r == nil || r.mode == rxModeNone {
		return 0
	}

	return r.format.bytes()
}

// OffsetHz returns the carrier offset of the last stream that ended, in Hz.
func (r *Receiver) OffsetHz() float64 {
	if r == nil {
		return 0
	}

	switch r.mode {
	case rxModeLow:
		return float64(r.low.hz) + r.low.d.offset()/iqUnitsPerHz
	case rxModeFull:
		return r.full.offsetHz()
	}
	return 0
}

// DeviationHz returns the outer deviation of the last locked stream, in Hz.
// Tier3 reads it from the fitted modulation index, or from its Tier2 side
// when its trellis has not locked. The lower tiers read it from the sync
// symbol levels and divide out the response of their channel filter and
// discriminator at the carrier offset from the tuned frequency. For a clean
// 1944 Hz signal the reading is 1 to 1.5% low and within 3% at every offset.
// A sync fit made before the timing settles or before an AFC retune can move
// it by a few percent more. At a Rate of 9600 the reading is up to 3% low,
// by the sub-sample phase of the bursts, and it does not include the channel
// filter of the radio chip.
func (r *Receiver) DeviationHz() float64 {
	if r == nil {
		return 0
	}

	switch r.mode {
	case rxModeLow:
		return r.low.deviation() / iqUnitsPerHz
	case rxModeFull:
		return r.full.deviationHz()
	}
	return 0
}
