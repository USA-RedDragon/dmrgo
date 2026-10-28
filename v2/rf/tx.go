package rf

import (
	"encoding/binary"
	"math"
	"unsafe"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

// TxConfig sets up a Transmitter. The zero TxConfig is the strictest
// setup: every region, every licensed service and the highest legal
// power of each, so that what it makes keeps the limits wherever it is
// accepted.
type TxConfig struct {
	// Tier selects the modulator: Tier0, Tier1 or Tier2. The zero value
	// TierDefault is Tier1 for NewTransmitter, and Tier0 for Init and under
	// TinyGo. Tier3 has no modulator of its own yet and is refused.
	Tier Tier
	// Rate is the sample rate of the I/Q, in Hz. 0 means 2400000. Tier1
	// and Tier2 make any Rate from 48000 to 20000000, and Tier0 makes
	// 96000. Under 62500 the image of the main lobe falls within two
	// channels of the carrier, where no limit can count on a filter of the
	// radio, so the limits refuse it.
	Rate int
	// Format is the layout of the I/Q samples. 0 means IQFormatCS16.
	// IQFormatCF32 values lie on the grid of IQFormatCS16.
	Format IQFormat
	// OffsetHz is the frequency of the carrier relative to the center of
	// the I/Q, in Hz, under Rate/4 in size. Tier0 rounds it to whole Hz.
	OffsetHz float64
	// OnSamples, when not nil, selects stream mode: Write and Flush hand
	// the samples to it in chunks of at most MaxOutput(0) samples, in
	// storage of the transmitter that is valid only during the call. When
	// nil, the transmitter is in append mode, for AppendSamples and
	// AppendFlush.
	OnSamples func([]byte)
	// Region is the regulatory region whose limits the signal keeps.
	Region Region
	// Service is the radio service of the station. ServiceUnknown keeps
	// the limits of the land mobile and amateur services at once. A station
	// of the license-free service must state it.
	Service Service
	// Power is the most power of the station, in dBm, from 1 to 63. 0
	// means the highest legal power of the Service in the Region.
	Power int
}

// Transmitter turns DMR bursts into I/Q samples for a software radio:
// 4FSK on a carrier, with the power ramps of TDMA bursts. It reads the
// bits of each Burst from Decoded, with its CACH when HasCACH, and places
// it on the 30 ms slot grid by Stream and Slot. A BS-sourced burst keeps
// the carrier up into the next slot; any other burst is an MS burst, with
// the power ramps in its guard time. A gap of one slot within a Stream is
// an idle slot with the carrier down, as an MS on one TDMA channel sends.
// A new Stream, or a gap of more than one slot, keys down, and the next
// burst keys up at once: the time between transmissions is the caller's.
// The transmitter ignores Seconds, Distance, Sync and Voice. Make it with
// NewTransmitter, or set it up with Init in memory of the caller. A
// Transmitter is not safe for concurrent use, and it must not be copied
// after Init or NewTransmitter.
type Transmitter struct {
	core   txCore
	host   txHost
	mcu    fsktx.MCUIQ
	s16    [2 * tier0IQSamples]int16
	small  [tier0IQSamples * 8]byte
	chunk  []byte
	out    []byte
	on     func([]byte)
	offHz  float64
	rate   int32
	sps    int32
	format IQFormat
	tier   Tier
	set    bool
}

const (
	tier0IQRate    = 96000
	tier0IQSamples = tier0IQRate / symbolRate
	defaultTxRate  = 2400000
	minTxIQRate    = 62500
	maxTxIQRate    = 20000000
	shaperIQRate   = 48000
	txDeviationHz  = 3 * deviationHz
	tier0IQLag     = 8
)

type txPlan struct {
	tier  Tier
	rate  int
	lag   int
	lf    bool
	sb    int
	class int
}

// NewTransmitter returns a Transmitter of c. A nil c means the zero
// TxConfig. It allocates the Transmitter, its modulator and, in stream
// mode, the chunk that it hands to OnSamples. It returns a *dmrgo.ConfigError when a field
// of c is out of range or names a setup that the modulator cannot make,
// and a *RegionError when the limits of the Region, Service and Power
// refuse the setup.
func NewTransmitter(c *TxConfig) (*Transmitter, error) {
	var cfg TxConfig
	if c != nil {
		cfg = *c
	}
	return newTransmitter(cfg, nil)
}

func newTransmitter(cfg TxConfig, g *txGate) (*Transmitter, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	p, err := cfg.plan(g)
	if err != nil {
		return nil, err
	}
	t := &Transmitter{}
	if p.tier == Tier0 {
		t.setup(&cfg, &p)
		return t, nil
	}
	sps := cfg.Rate/symbolRate + 2
	if err := t.host.newIQ(p.tier, cfg.Rate, cfg.OffsetHz); err != nil {
		return nil, iqErrs.rate
	}
	t.setup(&cfg, &p)
	t.sps = int32(sps)
	if cfg.OnSamples != nil {
		t.chunk = newChunk(t.MaxOutput(0) * p.sb)
	}
	return t, nil
}

// Init sets up t for c, in place of any state it holds, in Tier0 at a
// Rate of 96000. It does not allocate, so that t can live in memory of the
// caller. It returns dmrgo.ErrNilReceiver for a nil t, and a
// *dmrgo.ConfigError or *RegionError, with t unchanged, as
// NewTransmitter does.
func (t *Transmitter) Init(c TxConfig) error {
	return t.init(c, nil)
}

func (t *Transmitter) init(c TxConfig, g *txGate) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.Tier == TierDefault {
		c.Tier = Tier0
	}
	if c.Tier != Tier0 {
		return iqErrs.tier
	}
	p, err := c.plan(g)
	if err != nil {
		return err
	}
	t.setup(&c, &p)
	return nil
}

func (c *TxConfig) plan(g *txGate) (txPlan, error) {
	e := &iqErrs
	if g == nil {
		g = &txGate{}
	}
	if c.Tier < Tier0 || c.Tier > Tier2 || c.Tier != Tier0 && !txHostBuilt {
		return txPlan{}, e.tier
	}
	if c.Format == IQFormatDefault {
		c.Format = IQFormatCS16
	}
	if c.Format > IQFormatCF32 {
		return txPlan{}, e.format
	}
	if c.Rate == 0 {
		c.Rate = defaultTxRate
	}
	image := false
	switch {
	case c.Tier == Tier0 && c.Rate == shaperIQRate:
		image = true
	case c.Tier == Tier0 && c.Rate != tier0IQRate:
		return txPlan{}, e.rate
	case c.Rate < shaperIQRate || c.Rate > maxTxIQRate:
		return txPlan{}, e.rate
	case c.Rate < minTxIQRate:
		image = true
	}
	off := c.OffsetHz
	if c.Tier == Tier0 {
		off = math.Round(off)
	}
	if !(4*math.Abs(off) < float64(c.Rate)) {
		return txPlan{}, e.offset
	}
	r := txRegs{region: c.Region, service: c.Service, power: c.Power}
	class, err := r.check(e, g)
	if err != nil {
		return txPlan{}, err
	}
	bits := 16
	if c.Format == IQFormatCU8 || c.Format == IQFormatCS8 {
		bits = 8
	}
	if least := r.minRate(c.Tier, true, bits, class, 0); image || least == 0 || c.Rate < least {
		return txPlan{}, e.rateLimit
	}
	p := txPlan{tier: c.Tier, rate: c.Rate, lag: 8, lf: c.Service == ServiceLicenseFree, sb: c.Format.bytes(), class: class}
	switch c.Tier {
	case Tier0:
		p.lag = tier0IQLag
	case Tier2:
		p.lag = 16
	case TierDefault, Tier1, Tier3:
	}
	return p, nil
}

func (t *Transmitter) setup(c *TxConfig, p *txPlan) {
	h := t.host
	*t = Transmitter{}
	if p.tier == Tier0 {
		_ = t.mcu.Init(p.rate, int(math.Round(c.OffsetHz)), int(math.Round(txRampSec*float64(p.rate))))
		t.sps = int32(t.mcu.Samples())
		t.offHz = math.Round(c.OffsetHz)
	} else {
		t.host, t.offHz = h, c.OffsetHz
	}
	t.core.reset(p.lag, p.lf)
	t.rate, t.format, t.tier, t.on, t.set = int32(p.rate), c.Format, p.tier, c.OnSamples, true
	if t.on != nil && p.tier == Tier0 {
		t.chunk = t.small[:int(t.sps)*p.sb]
	}
}

// Write turns the bursts x into samples in stream mode and hands them to
// OnSamples. It returns len(x) and nil, dmrgo.ErrNilReceiver for a nil t,
// ErrNotSetUp before Init or NewTransmitter, a *dmrgo.ConfigError for
// OnSamples in append mode, and for a burst that it refuses the count of
// bursts before it with ErrBurstOrder, ErrBurstService or ErrTxTimeout.
func (t *Transmitter) Write(x []Burst) (int, error) {
	if t == nil {
		return 0, dmrgo.ErrNilReceiver
	}

	if err := t.streamErr(); err != nil {
		return 0, err
	}
	t.out = t.chunk[:0]
	for i := range x {
		if err := t.core.put(t, &x[i]); err != nil {
			t.deliver()
			return i, err
		}
	}
	t.deliver()
	return len(x), nil
}

// Flush ends the transmission in stream mode: it hands the tail of the
// pulse and the power ramp down to OnSamples. It returns the errors of
// Write other than those of a burst.
func (t *Transmitter) Flush() error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if err := t.streamErr(); err != nil {
		return err
	}
	t.out = t.chunk[:0]
	t.core.down(t)
	t.deliver()
	return nil
}

func (t *Transmitter) streamErr() error {
	switch {
	case !t.set:
		return ErrNotSetUp
	case t.on == nil:
		return iqErrs.onSamples
	}
	return nil
}

// AppendSamples turns the bursts x into samples in append mode, appends
// them to dst in Format and returns the extended slice. It stops at the
// first burst that Write would refuse. It allocates only when dst has no
// room for r.MaxOutput(len(x)) samples. In stream mode, or before Init or
// NewTransmitter, it returns dst.
func (t *Transmitter) AppendSamples(dst []byte, x []Burst) []byte {
	if t == nil || !t.set || t.on != nil {
		return dst
	}

	dst = growBytes(dst, t.MaxOutput(len(x))*t.format.bytes())
	n := len(dst)
	t.out = dst[n:n]
	for i := range x {
		if t.core.put(t, &x[i]) != nil {
			break
		}
	}
	m := len(t.out)
	t.out = nil
	return dst[:n+m]
}

// AppendFlush ends the transmission in append mode, appends the tail of
// the pulse and the power ramp down to dst and returns the extended slice.
// It allocates only when dst has no room for r.MaxOutput(0) samples. In
// stream mode, or before Init or NewTransmitter, it returns dst.
func (t *Transmitter) AppendFlush(dst []byte) []byte {
	if t == nil || !t.set || t.on != nil {
		return dst
	}

	dst = growBytes(dst, t.MaxOutput(0)*t.format.bytes())
	n := len(dst)
	t.out = dst[n:n]
	t.core.down(t)
	m := len(t.out)
	t.out = nil
	return dst[:n+m]
}

func (t *Transmitter) deliver() {
	if len(t.out) > 0 {
		t.on(t.out)
	}
	t.out = t.out[:0]
}

func (t *Transmitter) key(on bool) {
	if t.tier == Tier0 {
		t.mcu.Key(on)
		return
	}
	t.host.key(on)
}

func (t *Transmitter) symbol(s int8) {
	sb := t.format.bytes()
	if cap(t.out)-len(t.out) < int(t.sps)*sb {
		if t.on == nil {
			return
		}
		t.deliver()
		if cap(t.out) < int(t.sps)*sb {
			return
		}
	}
	switch {
	case t.tier == Tier0 && t.format == IQFormatCS16 && txLittle && uintptr(unsafe.Pointer(unsafe.SliceData(t.out)))&1 == 0:
		l := len(t.out)
		d := t.out[l : l+4*int(t.sps)]
		n := t.mcu.Symbol(unsafe.Slice((*int16)(unsafe.Pointer(unsafe.SliceData(d))), 2*t.sps), s)
		t.out = t.out[:l+2*n]
	case t.tier == Tier0:
		t.put16(t.s16[:t.mcu.Symbol(t.s16[:], s)])
	default:
		t.put16(t.host.iqS16(s))
	}
}

func (t *Transmitter) put16(v []int16) {
	l := len(t.out)
	switch t.format {
	case IQFormatCU8, IQFormatCS8:
		t.out = t.out[:l+len(v)]
		for i, x := range v {
			b := byte(int8((int32(x) + 1<<7) >> 8))
			if t.format == IQFormatCU8 {
				b ^= 0x80
			}
			t.out[l+i] = b
		}
	case IQFormatCF32:
		t.out = t.out[:l+4*len(v)]
		for i, x := range v {
			binary.LittleEndian.PutUint32(t.out[l+4*i:], math.Float32bits(float32(x)/32768))
		}
	default:
		t.out = t.out[:l+2*len(v)]
		for i, x := range v {
			binary.LittleEndian.PutUint16(t.out[l+2*i:], uint16(x))
		}
	}
}

// Active reports whether the stream is inside a transmission: from the
// key-up of its first burst to the key-down after its last. The carrier
// ramps down in each idle slot within it.
func (t *Transmitter) Active() bool {
	if t == nil || !t.set {
		return false
	}

	return t.core.open
}

// Lag returns the most samples between the start of a burst in its slot
// and its first sample.
func (t *Transmitter) Lag() int {
	if t == nil || !t.set {
		return 0
	}

	return (int(t.core.lag)*int(t.rate) + symbolRate - 1) / symbolRate
}

// MaxOutput returns the most samples that AppendSamples can append for n
// bursts, and for n == 0 the most that AppendFlush can append. Each
// OnSamples chunk holds at most MaxOutput(0) samples. It does not
// allocate.
func (t *Transmitter) MaxOutput(n int) int {
	if t == nil || !t.set {
		return 0
	}

	return int(min(txMaxSymbols(n, int(t.core.lag))*int64(t.sps), 1<<40))
}

// SampleBytes returns the bytes of one I/Q sample in the Format of t.
func (t *Transmitter) SampleBytes() int {
	if t == nil || !t.set {
		return 0
	}

	return t.format.bytes()
}

// OffsetHz returns the carrier offset that t sends, in Hz, as Tier0 rounds
// it.
func (t *Transmitter) OffsetHz() float64 {
	if t == nil || !t.set {
		return 0
	}

	return t.offHz
}

// DeviationHz returns the outer deviation that t sends, 1944 Hz.
func (t *Transmitter) DeviationHz() float64 {
	if t == nil || !t.set {
		return 0
	}

	return txDeviationHz
}

func growBytes(dst []byte, n int) []byte {
	for cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], 0)[:len(dst)]
	}
	return dst
}
