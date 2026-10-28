package rf

import (
	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fsktx"
)

// AudioTxConfig sets up an AudioTransmitter. The zero AudioTxConfig is the
// strictest setup, as the zero TxConfig is.
type AudioTxConfig struct {
	// Tier selects the modulator: Tier0, Tier1 or Tier2. The zero value
	// TierDefault is Tier1 for NewAudioTransmitter, and Tier0 for Init and
	// under TinyGo. Tier3 has no modulator of its own yet and is refused.
	Tier Tier
	// Rate is the sample rate of the converter, in Hz. 0 means 48000.
	// Tier0 makes 24000 and 48000. Tier1 and Tier2 make any rate from
	// 8000 whose ratio to 4800 has a numerator of at most 1024 in lowest
	// terms, such as 8000, 22050 or 44100, and the limits are measured to
	// 96000.
	Rate int
	// Bits is the resolution of the converter, from 8 to 16. 0 means 16.
	// Each sample holds one code of Bits bits in its low bits.
	Bits int
	// Unsigned selects offset binary codes, from 0 to 1<<Bits-1, in place
	// of two's complement codes.
	Unsigned bool
	// OnSamples, when not nil, selects stream mode: Write and Flush hand
	// the samples to it in chunks of at most MaxOutput(0) samples, in
	// storage of the transmitter that is valid only during the call. When
	// nil, the transmitter is in append mode, for AppendSamples and
	// AppendFlush.
	OnSamples func([]int16)
	// Level is the code of a run of symbol +3, 1944 Hz of deviation, less
	// the center, from 1 to 16000. 0 means the most that fits in Bits with
	// Offset over every run of symbols. The limits are measured at about
	// half of full scale, so a Level under that, 1040 codes at 12 bits, is
	// refused. Tier1 and Tier2 at 24000 Hz and above fit that level only
	// at 16 bits.
	Level int
	// Offset is added to the center code, 0 or 1<<(Bits-1) when Unsigned.
	Offset int
	// PreEmphasis states that the radio pre-emphasizes its modulator
	// input from 300 Hz to 3 kHz, so that the transmitter inverts it. The
	// limits with it are not measured yet, so it is refused.
	PreEmphasis bool
	// InputLowpassHz states the cutoff in Hz of a low pass of at least two
	// poles that the radio puts before its modulator input; 0 means none,
	// a converter that drives the modulator with no filter. The limits are
	// measured with a two-pole Butterworth low pass at 8000, 5000 and
	// 3000 Hz, and a stated cutoff takes the next looser of these, so 6000
	// counts as 8000 and a cutoff above 8000 as none. A low pass lowers the
	// least Rate that keeps the limits. A negative cutoff is refused.
	InputLowpassHz int
	// Region is the regulatory region whose limits the signal keeps.
	Region Region
	// Service is the radio service of the station, as in TxConfig.
	Service Service
	// Power is the most power of the station in dBm, as in TxConfig.
	Power int
}

// AudioTransmitter turns DMR bursts into the deviation signal that drives
// the modulator input of an FM radio, as the radio chips of hotspots and
// MMDVM modems take it: codes of a converter, with the CACH of BS-sourced
// bursts and the guard time of MS bursts as a Transmitter places them.
// Active is the push-to-talk of the radio. Make it with
// NewAudioTransmitter, or set it up with Init in memory of the caller. An
// AudioTransmitter is not safe for concurrent use, and it must not be
// copied after Init or NewAudioTransmitter.
type AudioTransmitter struct {
	core   txCore
	host   txHost
	mcu    fsktx.MCUAudio
	small  [tier0AudioSamples]int16
	chunk  []int16
	out    []int16
	on     func([]int16)
	level  int32
	offset int32
	bias   int32
	dc     int32
	rate   int32
	sps    int32
	tier   Tier
	set    bool
}

const (
	tier0AudioSamples = 48000 / symbolRate
	defaultTxAudio    = 48000
	minTxAudio        = 8000
	maxTxAudio        = 96000
	maxTxLevel        = 16000
)

type audioPlan struct {
	txPlan
	level, bits int
	table       fsktx.AudioTable
}

// NewAudioTransmitter returns an AudioTransmitter of c. A nil c means the
// zero AudioTxConfig. It allocates the AudioTransmitter, its modulator
// and, in stream mode, the chunk that it hands to OnSamples. It returns the
// errors of NewTransmitter.
func NewAudioTransmitter(c *AudioTxConfig) (*AudioTransmitter, error) {
	var cfg AudioTxConfig
	if c != nil {
		cfg = *c
	}
	return newAudioTransmitter(cfg, nil)
}

func newAudioTransmitter(cfg AudioTxConfig, g *txGate) (*AudioTransmitter, error) {
	if cfg.Tier == TierDefault {
		cfg.Tier = discDefaultTier
	}
	p, avail, err := cfg.prepare()
	if err != nil {
		return nil, err
	}
	var level int
	if cfg.Tier == Tier0 {
		level, err = cfg.fitTable(&p, avail)
	} else {
		level, err = cfg.fitHost(avail)
	}
	if err == nil {
		err = cfg.plan(&p, g, level)
	}
	if err != nil {
		return nil, err
	}
	t := &AudioTransmitter{}
	if p.tier == Tier0 {
		t.setup(&cfg, &p)
		return t, nil
	}
	sps := cfg.Rate/symbolRate + 2
	if err := t.host.newAudio(p.tier, cfg.Rate, p.level, cfg.Offset); err != nil {
		return nil, audioErrs.rate
	}
	t.setup(&cfg, &p)
	t.sps = int32(sps)
	if cfg.OnSamples != nil {
		t.chunk = newChunk16(t.MaxOutput(0))
	}
	return t, nil
}

// Init sets up t for c, in place of any state it holds, in Tier0 at a
// Rate of 24000 or 48000. It does not allocate, so that t can live in
// memory of the caller. It returns the errors of Transmitter.Init.
func (t *AudioTransmitter) Init(c AudioTxConfig) error {
	return t.init(c, nil)
}

func (t *AudioTransmitter) init(c AudioTxConfig, g *txGate) error {
	if t == nil {
		return dmrgo.ErrNilReceiver
	}

	if c.Tier == TierDefault {
		c.Tier = Tier0
	}
	if c.Tier != Tier0 {
		return audioErrs.tier
	}
	p, avail, err := c.prepare()
	if err != nil {
		return err
	}
	level, err := c.fitTable(&p, avail)
	if err == nil {
		err = c.plan(&p, g, level)
	}
	if err != nil {
		return err
	}
	t.setup(&c, &p)
	return nil
}

func (c *AudioTxConfig) prepare() (audioPlan, int, error) {
	e := &audioErrs
	if c.Tier < Tier0 || c.Tier > Tier2 || c.Tier != Tier0 && !txHostBuilt {
		return audioPlan{}, 0, e.tier
	}
	if c.Rate == 0 {
		c.Rate = defaultTxAudio
	}
	var p audioPlan
	switch {
	case c.Tier == Tier0 && c.Rate == 24000:
		p.table = fsktx.Table24k()
	case c.Tier == Tier0 && c.Rate == 48000:
		p.table = fsktx.Table48k()
	case c.Tier == Tier0, c.Rate < minTxAudio:
		return audioPlan{}, 0, e.rate
	}
	if c.Bits == 0 {
		c.Bits = 16
	}
	if c.Bits < 8 || c.Bits > 16 {
		return audioPlan{}, 0, e.bits
	}
	half := 1<<(c.Bits-1) - 1
	if c.Offset <= -half || c.Offset >= half {
		return audioPlan{}, 0, e.offsetCodes
	}
	if c.InputLowpassHz < 0 {
		return audioPlan{}, 0, e.lowpass
	}
	if c.Level < 0 || c.Level > maxTxLevel {
		return audioPlan{}, 0, e.level
	}
	return p, half - max(c.Offset, -c.Offset), nil
}

func (c *AudioTxConfig) plan(p *audioPlan, g *txGate, level int) error {
	e := &audioErrs
	if g == nil {
		g = &txGate{}
	}
	r := txRegs{region: c.Region, service: c.Service, power: c.Power}
	class, err := r.check(e, g)
	if err != nil {
		return err
	}
	if c.PreEmphasis {
		return e.preEmph
	}
	if level < measuredLevel(c.Bits) {
		return e.levelLow
	}
	if least := r.minRate(c.Tier, false, c.Bits, class, c.InputLowpassHz); least == 0 || c.Rate < least || c.Rate > maxTxAudio {
		return e.rateLimit
	}
	p.txPlan = txPlan{tier: c.Tier, rate: c.Rate, lag: 8, lf: c.Service == ServiceLicenseFree, sb: 2, class: class}
	p.level, p.bits = level, c.Bits
	switch c.Tier {
	case Tier0:
		p.lag = p.table.Span()
	case Tier2:
		p.lag = 16
	case TierDefault, Tier1, Tier3:
	}
	return nil
}

func (c *AudioTxConfig) fitTable(p *audioPlan, avail int) (int, error) {
	if c.Level != 0 {
		if p.table.Peak(c.Level) > avail {
			return 0, audioErrs.level
		}
		return c.Level, nil
	}
	lo, hi := 0, maxTxLevel+1
	for hi-lo > 1 {
		if m := (lo + hi) / 2; p.table.Peak(m) <= avail {
			lo = m
		} else {
			hi = m
		}
	}
	if lo < 1 {
		return 0, audioErrs.offsetCodes
	}
	return lo, nil
}

func (c *AudioTxConfig) fitHost(avail int) (int, error) {
	if _, err := audioPeak(c.Tier, c.Rate, 1); err != nil {
		return 0, audioErrs.rate
	}
	if c.Level != 0 {
		if !c.hostFits(c.Level, avail) {
			return 0, audioErrs.level
		}
		return c.Level, nil
	}
	lo, hi := 0, maxTxLevel+1
	for hi-lo > 1 {
		if m := (lo + hi) / 2; c.hostFits(m, avail) {
			lo = m
		} else {
			hi = m
		}
	}
	if lo < 1 {
		return 0, audioErrs.offsetCodes
	}
	return lo, nil
}

func (c *AudioTxConfig) hostFits(level, avail int) bool {
	pk, err := audioPeak(c.Tier, c.Rate, level)
	return err == nil && pk <= avail
}

func measuredLevel(bits int) int {
	switch {
	case bits >= 16:
		return 15000
	case bits >= 12:
		return 1040 << (bits - 12)
	case bits >= 10:
		return 260 << (bits - 10)
	}
	return 65 << (bits - 8)
}

func (t *AudioTransmitter) setup(c *AudioTxConfig, p *audioPlan) {
	h := t.host
	*t = AudioTransmitter{}
	t.core.reset(p.lag, p.lf)
	t.level, t.offset, t.rate, t.tier, t.on, t.set = int32(p.level), int32(c.Offset), int32(p.rate), p.tier, c.OnSamples, true
	if c.Unsigned {
		t.bias = 1 << (p.bits - 1)
	}
	t.dc = t.bias
	if p.tier == Tier0 {
		in := c.Offset
		if p.bits < 16 {
			in, t.bias = in+int(t.bias), 0
		}
		_ = t.mcu.Init(p.table, p.level, in)
		t.sps = int32(t.mcu.Samples())
	} else {
		t.host = h
	}
	if t.on != nil && p.tier == Tier0 {
		t.chunk = t.small[:t.sps]
	}
}

// Write turns the bursts x into samples in stream mode and hands them to
// OnSamples. It returns the errors of Transmitter.Write.
func (t *AudioTransmitter) Write(x []Burst) (int, error) {
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
// pulse to OnSamples. It returns the errors of Write other than those of
// a burst.
func (t *AudioTransmitter) Flush() error {
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

func (t *AudioTransmitter) streamErr() error {
	switch {
	case !t.set:
		return ErrNotSetUp
	case t.on == nil:
		return audioErrs.onSamples
	}
	return nil
}

// AppendSamples turns the bursts x into samples in append mode, appends
// them to dst and returns the extended slice. It stops at the first burst
// that Write would refuse. It allocates only when dst has no room for
// r.MaxOutput(len(x)) samples. In stream mode, or before Init or
// NewAudioTransmitter, it returns dst.
func (t *AudioTransmitter) AppendSamples(dst []int16, x []Burst) []int16 {
	if t == nil || !t.set || t.on != nil {
		return dst
	}

	dst = growInt16(dst, t.MaxOutput(len(x)))
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
// the pulse to dst and returns the extended slice. It allocates only when
// dst has no room for r.MaxOutput(0) samples. In stream mode, or before
// Init or NewAudioTransmitter, it returns dst.
func (t *AudioTransmitter) AppendFlush(dst []int16) []int16 {
	if t == nil || !t.set || t.on != nil {
		return dst
	}

	dst = growInt16(dst, t.MaxOutput(0))
	n := len(dst)
	t.out = dst[n:n]
	t.core.down(t)
	m := len(t.out)
	t.out = nil
	return dst[:n+m]
}

func (t *AudioTransmitter) deliver() {
	if len(t.out) > 0 {
		t.on(t.out)
	}
	t.out = t.out[:0]
}

func (*AudioTransmitter) key(bool) {}

func (t *AudioTransmitter) symbol(s int8) {
	if cap(t.out)-len(t.out) < int(t.sps) {
		if t.on == nil {
			return
		}
		t.deliver()
		if cap(t.out) < int(t.sps) {
			return
		}
	}
	l := len(t.out)
	if t.tier == Tier0 {
		t.out = t.out[:l+t.mcu.Symbol(t.out[l:l+int(t.sps)], s)]
	} else {
		v := t.host.audio(s)
		t.out = t.out[:l+copy(t.out[l:l+len(v)], v)]
	}
	if t.bias != 0 {
		for i := l; i < len(t.out); i++ {
			t.out[i] = int16(uint16(int32(t.out[i]) + t.bias))
		}
	}
}

// Active reports whether the stream is inside a transmission: from the
// key-up of its first burst to the key-down after its last, the time
// that the radio must be keyed.
func (t *AudioTransmitter) Active() bool {
	if t == nil || !t.set {
		return false
	}

	return t.core.open
}

// Lag returns the most samples between the start of a burst in its slot
// and its first sample.
func (t *AudioTransmitter) Lag() int {
	if t == nil || !t.set {
		return 0
	}

	return (int(t.core.lag)*int(t.rate) + symbolRate - 1) / symbolRate
}

// MaxOutput returns the most samples that AppendSamples can append for n
// bursts, and for n == 0 the most that AppendFlush can append. Each
// OnSamples chunk holds at most MaxOutput(0) samples. It does not
// allocate.
func (t *AudioTransmitter) MaxOutput(n int) int {
	if t == nil || !t.set {
		return 0
	}

	return int(min(txMaxSymbols(n, int(t.core.lag))*int64(t.sps), 1<<40))
}

// Level returns the code of a run of symbol +3 less the center that t
// sends, after the 0 default resolves.
func (t *AudioTransmitter) Level() int {
	if t == nil || !t.set {
		return 0
	}

	return int(t.level)
}

// Offset returns the DC code that t sends: the center and Offset.
func (t *AudioTransmitter) Offset() int {
	if t == nil || !t.set {
		return 0
	}

	return int(t.dc + t.offset)
}

func growInt16(dst []int16, n int) []int16 {
	for cap(dst)-len(dst) < n {
		dst = append(dst[:cap(dst)], 0)[:len(dst)]
	}
	return dst
}
