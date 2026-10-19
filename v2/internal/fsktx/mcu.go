package fsktx

import (
	"errors"
	"math"
)

const tableUnits = 15000

// tier0Exact makes the tables of the Tier0 modulators Nyquist pulses after
// a receive root-raised-cosine filter, with the inverse of the hold of a
// DAC or of the linear interpolation of MCUIQ, and each phase exact for a
// run of one symbol. MCUIQ then sums the phase steps at the midpoints of
// its output samples, so that its phase is the integral of the
// interpolated deviation at every rate.
const (
	tier0Exact = false
)

// tier0IQFine makes MCUIQ interpolate cosTable to first order in the phase.
const (
	tier0IQFine = false
)

// tapShift is the number of extra fraction bits of the taps of an
// AudioTable over its output units.
const (
	tapShift = 0
)

// tier0Corr adds to the samples of Table48k the correction of corr48k for
// the context of corr48kSyms symbols that ends corr48kSkip symbols before
// the newest, which cuts the power of the FM signal outside the channel.
const (
	tier0Corr = false
)

const (
	maxSamples = 10
	maxTaps    = 16
)

// An AudioTable is the pulse of the Tier0 modulators at one sample rate,
// in read-only memory: the polyphase taps of the root-raised-cosine pulse,
// in output units, for a multiply and accumulate over the symbols that it
// spans. A program links the table of each rate that it uses, and only
// those.
type AudioTable struct {
	shape func(dst *[maxSamples]int32, r *[maxTaps]int8) int
	hz    int
	span  int
	peak  int32
}

// Hz returns the sample rate of t.
func (t AudioTable) Hz() int { return t.hz }

// Span returns the number of symbols on each side of the centre of the
// pulse of t, which is also the lag of the output of a modulator behind
// its symbols, in symbol periods.
func (t AudioTable) Span() int { return t.span }

// Table24k returns the table for samples at 24 kHz, as MMDVM modems use:
// a pulse of 8 symbols on each side, 160 bytes.
func Table24k() AudioTable {
	return AudioTable{shape: shape24k, hz: 24000, span: len(taps24k[0]) / 2, peak: taps24kPeak}
}

// Table48k returns the table for samples at 48 kHz: a pulse of 6 symbols
// on each side, 240 bytes.
func Table48k() AudioTable {
	return AudioTable{shape: shape48k, hz: 48000, span: len(taps48k[0]) / 2, peak: taps48kPeak}
}

func shape24k(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	for p := range taps24k {
		t := &taps24k[p]
		var y int32
		for i := range t {
			y += int32(t[i]) * int32(r[i])
		}
		dst[p] = y
	}
	return len(taps24k)
}

func shape48k(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	for p := range taps48k {
		t := &taps48k[p]
		var y int32
		for i := range t {
			y += int32(t[i]) * int32(r[i])
		}
		dst[p] = y
	}
	return len(taps48k)
}

var (
	errTable   = errors.New("fsktx: no table")
	errLevel   = errors.New("fsktx: level out of range")
	errOffset  = errors.New("fsktx: level and offset pass the range of int16")
	errRate    = errors.New("fsktx: rate is not a multiple of the table rate")
	errCarrier = errors.New("fsktx: carrier offset out of range")
	errRamp    = errors.New("fsktx: ramp out of range")
)

type shaperMAC struct {
	t    AudioTable
	ring [2 * maxTaps]int8
	pos  int
	buf  [maxSamples]int32
}

func (k *shaperMAC) reset(t AudioTable) {
	*k = shaperMAC{t: t}
}

func (k *shaperMAC) symbol(s int8) int {
	if k.t.shape == nil {
		return 0
	}
	switch s {
	case -3, -1, 1, 3:
	default:
		s = 0
	}
	k.pos--
	if k.pos < 0 {
		k.pos = maxTaps - 1
	}
	k.ring[k.pos] = s
	k.ring[k.pos+maxTaps] = s
	return k.t.shape(&k.buf, (*[maxTaps]int8)(k.ring[k.pos:k.pos+maxTaps]))
}

// MCUAudio is the Tier0 modulator of the deviation signal that drives the
// modulator input of an FM radio, as MMDVM modems drive it: the
// root-raised-cosine pulse, by a fixed-point multiply and accumulate per
// sample over the taps of an AudioTable in read-only memory. It keeps 96
// bytes of state, allocates nothing and fits a microcontroller: Init sets it
// up in memory of the caller, such as a package variable. An MCUAudio is not
// safe for concurrent use. (ETSI TS 102 361-1 clause 10.2.2.2)
type MCUAudio struct {
	k      shaperMAC
	scale  int32
	offset int32
	// codes holds 2 bits for each recent symbol, the newest lowest, and
	// zeros a bit for each that is 0; corr is set for Table48k.
	codes, zeros uint32
	idx5         uint32
	corr         bool
}

// Init sets up m for table t at level units for a run of symbol +3 (1944
// Hz), from 1 to 16000, around offset units: the worst run of symbols
// reaches 1.9 times level from offset, which must fit int16. A 12-bit DAC
// takes level 1080 and offset 2048. The deviation test should give 2749 Hz
// peak through the radio. On an error m stays as it was; otherwise it starts
// with no modulation. (ETSI TS 102 361-1 clause 10.2.2.3)
func (m *MCUAudio) Init(t AudioTable, level, offset int) error {
	switch {
	case t.shape == nil:
		return errTable
	case level < 1 || level > 16000:
		return errLevel
	}
	scale := int32((level<<15 + tableUnits/2) / tableUnits)
	hi := int((3*int64(t.peak)*int64(scale) + 1<<(14+tapShift)) >> (15 + tapShift))
	lo := int((-3*int64(t.peak)*int64(scale) + 1<<(14+tapShift)) >> (15 + tapShift))
	if offset > math.MaxInt16-hi || offset < math.MinInt16-lo {
		return errOffset
	}
	m.k.reset(t)
	m.scale = scale
	m.offset = int32(offset)
	m.codes, m.zeros, m.corr = 0, ^uint32(0), tier0Corr && t.hz == 48000
	m.idx5 = uint32(len(corr48k) - 1)
	return nil
}

// Samples returns the number of samples of each symbol.
func (m *MCUAudio) Samples() int { return m.k.t.hz / SymbolRate }

// Symbol shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation, and
// writes the samples of the symbol period that it completes to dst. It
// returns the number it wrote. With a dst shorter than Samples, or before
// Init, it writes nothing, does not take the symbol and returns 0.
// The output lags the symbols by the Span of the table: twice that many
// symbols 0 after the last bring its tail to rest.
func (m *MCUAudio) Symbol(dst []int16, s int8) int {
	if len(dst) < m.Samples() {
		return 0
	}
	n := m.k.symbol(s)
	sc, off, buf := int64(m.scale), m.offset, &m.k.buf
	if tier0Corr && m.corr && n == len(corr48k[0]) {
		if t := m.corrRow(s); t != nil {
			d := (*[len(corr48k[0])]int16)(dst)
			for p := range d {
				d[p] = int16(int32((int64(buf[p]+int32(t[p]))*sc+1<<(14+tapShift))>>(15+tapShift)) + off)
			}
			return n
		}
	}
	for p := range dst[:n] {
		dst[p] = int16(int32((int64(buf[p])*sc+1<<(14+tapShift))>>(15+tapShift)) + off)
	}
	return n
}

// corrRow takes the symbol s into the context and returns the correction
// of the context, nil for a context with a symbol 0.
func (m *MCUAudio) corrRow(s int8) *[10]int16 {
	if corr48kBase == 5 {
		r := &m.k.ring
		e := code5[(r[m.k.pos+corr48kSkip]+3)&7]
		l := code5[(r[m.k.pos+corr48kSkip+corr48kSyms]+3)&7]
		m.idx5 = (m.idx5-uint32(l)*corr48kTop)*5 + uint32(e)
		return &corr48k[m.idx5]
	}
	z := uint32(0)
	switch s {
	case -3, -1, 1, 3:
	default:
		z, s = 1, -3
	}
	m.codes = m.codes<<2 | uint32(s+3)>>1
	m.zeros = m.zeros<<1 | z
	if m.zeros>>corr48kSkip&(1<<corr48kSyms-1) != 0 {
		return nil
	}
	return &corr48k[m.codes>>(2*corr48kSkip)&(1<<(2*corr48kSyms)-1)]
}

// code5 is the digit of a symbol plus 3 in the index of corr48k in base
// 5: -3, -1, +1, +3 as 0 to 3 and 0 as 4.
var code5 = [8]uint8{0, 0, 1, 4, 2, 0, 3, 0}
