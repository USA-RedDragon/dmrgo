package fsktx

import "errors"

// tableUnits is the output of an AudioTable for a run of symbol +3, the
// 1944 Hz of its deviation.
const tableUnits = 15000

// maxSamples is the most samples of a symbol period of an AudioTable, and
// maxTaps the most symbols that its pulse spans.
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
}

// Hz returns the sample rate of t.
func (t AudioTable) Hz() int { return t.hz }

// Span returns the number of symbols on each side of the centre of the
// pulse of t, which is also the lag of the output of a modulator behind
// its symbols, in symbol periods.
func (t AudioTable) Span() int { return t.span }

// Table24k returns the table for samples at 24 kHz, as MMDVM modems use:
// a pulse of 8 symbols on each side, 160 bytes.
func Table24k() AudioTable { return AudioTable{shape: shape24k, hz: 24000, span: len(taps24k[0]) / 2} }

// Table48k returns the table for samples at 48 kHz: a pulse of 6 symbols
// on each side, 240 bytes.
func Table48k() AudioTable { return AudioTable{shape: shape48k, hz: 48000, span: len(taps48k[0]) / 2} }

// tableIQ returns the table of MCUIQ, at 24 kHz with a pulse of 4 symbols
// on each side.
func tableIQ() AudioTable { return AudioTable{shape: shapeIQ, hz: 24000, span: len(tapsIQ[0]) / 2} }

// shape24k writes the deviation of the samples of a symbol period at 24
// kHz to dst, in output units, for the symbols r, newest first, and
// returns the number of samples.
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

// shape48k is shape24k at 48 kHz.
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

// shapeIQ is shape24k for the table of MCUIQ.
func shapeIQ(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	for p := range tapsIQ {
		t := &tapsIQ[p]
		var y int32
		for i := range t {
			y += int32(t[i]) * int32(r[i])
		}
		dst[p] = y
	}
	return len(tapsIQ)
}

// The errors of the Init methods. Each is a static value, so that Init
// does not allocate.
var (
	errTable = errors.New("fsktx: no table")
	errLevel = errors.New("fsktx: level out of range")
	errRate  = errors.New("fsktx: rate is not a multiple of the table rate")
)

// shaperMAC is the shaping state of the Tier0 modulators: the last
// maxTaps symbols, twice in a row so that a window of them is contiguous.
type shaperMAC struct {
	t    AudioTable
	ring [2 * maxTaps]int8
	pos  int
	buf  [maxSamples]int32
}

// reset fills the ring with symbols 0.
func (k *shaperMAC) reset(t AudioTable) {
	*k = shaperMAC{t: t}
}

// symbol puts the symbol s in the ring and writes the deviation of the
// samples of the symbol period that it completes to buf, in output units.
// It returns the number of samples. A symbol other than -3, -1, +1 and +3
// counts as 0.
func (k *shaperMAC) symbol(s int8) int {
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
// root-raised-cosine pulse of ETSI TS 102 361-1 clause 10.2.2.2, by a
// fixed-point multiply and accumulate per sample over the taps of an
// AudioTable in read-only memory. It keeps 96 bytes of state, allocates
// nothing and fits a microcontroller: Init sets it up in memory of the
// caller, such as a package variable. An MCUAudio is not safe for
// concurrent use.
type MCUAudio struct {
	k      shaperMAC
	scale  int32
	offset int32
}

// Init sets up m for the samples of table t, at level units of the output
// for a run of symbol +3 (1944 Hz), from 1 to 16000, around offset units,
// as an unsigned DAC needs: an output of level 15000 and offset 0 spans
// about -28500 to 28500, and a 12-bit DAC takes level 1100 and offset
// 2048. Level sets the deviation of the radio together with its gain; the
// deviation test of ETSI TS 102 361-1 clause 10.2.2.3, the symbols +3 +3
// -3 -3 repeated, should give 2749 Hz peak. m starts as after symbols 0,
// no modulation.
func (m *MCUAudio) Init(t AudioTable, level, offset int) error {
	switch {
	case t.shape == nil:
		return errTable
	case level < 1 || level > 16000:
		return errLevel
	}
	m.k.reset(t)
	m.scale = int32((level<<15 + tableUnits/2) / tableUnits)
	m.offset = int32(offset)
	return nil
}

// Samples returns the number of samples of each symbol.
func (m *MCUAudio) Samples() int { return m.k.t.hz / SymbolRate }

// Symbol shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation, and
// writes the samples of the symbol period that it completes to dst, which
// needs room for Samples of them. It returns the number it wrote. The
// output lags the symbols by the Span of the table: twice that many
// symbols 0 after the last bring its tail to rest.
func (m *MCUAudio) Symbol(dst []int16, s int8) int {
	n := m.k.symbol(s)
	dst = dst[:n]
	for p := range dst {
		y := (m.k.buf[p]*m.scale+1<<14)>>15 + m.offset
		dst[p] = int16(min(max(y, -32768), 32767))
	}
	return n
}
