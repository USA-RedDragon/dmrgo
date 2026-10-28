package fsktx

import (
	"errors"
	"math"
)

const tableUnits = 15000

const (
	tier0Exact = true
)

const (
	tier0IQFine = true
)

const (
	tapShift = 2
)

const (
	tier0Corr = true
)

const (
	tier0Corr24 = true
)

const (
	tier0Shape2 = true
)

const (
	tier0Fold = true
)

const (
	tier0Shape = true
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

func (t AudioTable) bounds(scale int32) (lo, hi int) {
	peak := 3 * int64(t.peak)
	if tier0Corr && (t.hz == 48000 || tier0Corr24 && t.hz == 24000) {
		peak += mcuCorrPeak
	}
	hi = int((peak*int64(scale) + 1<<(14+tapShift)) >> (15 + tapShift))
	lo = int((-peak*int64(scale) + 1<<(14+tapShift)) >> (15 + tapShift))
	if tier0Shape {
		lo, hi = lo-1, hi+1
	}
	return lo, hi
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
	if tier0Fold {
		return shape24kFold(dst, r)
	}
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

func shape24kFold(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	r0, r1, r2, r3, r4, r5, r6, r7 := int32(r[0]), int32(r[1]), int32(r[2]), int32(r[3]), int32(r[4]), int32(r[5]), int32(r[6]), int32(r[7])
	r8, r9, r10, r11, r12, r13, r14, r15 := int32(r[8]), int32(r[9]), int32(r[10]), int32(r[11]), int32(r[12]), int32(r[13]), int32(r[14]), int32(r[15])
	f := &taps24kFold
	dst[0] = int32(f[0][0])*r0 + int32(f[0][1])*r1 + int32(f[0][2])*r2 + int32(f[0][3])*r3 + int32(f[0][4])*r4 + int32(f[0][5])*r5 + int32(f[0][6])*r6 + int32(f[0][7])*r7 +
		int32(f[0][8])*r8 + int32(f[0][9])*r9 + int32(f[0][10])*r10 + int32(f[0][11])*r11 + int32(f[0][12])*r12 + int32(f[0][13])*r13 + int32(f[0][14])*r14 + int32(f[0][15])*r15
	u0, u1, u2, u3, u4, u5, u6, u7 := r0+r15, r1+r14, r2+r13, r3+r12, r4+r11, r5+r10, r6+r9, r7+r8
	w0, w1, w2, w3, w4, w5, w6, w7 := r0-r15, r1-r14, r2-r13, r3-r12, r4-r11, r5-r10, r6-r9, r7-r8
	s1 := int32(f[1][0])*u0 + int32(f[1][1])*u1 + int32(f[1][2])*u2 + int32(f[1][3])*u3 + int32(f[1][4])*u4 + int32(f[1][5])*u5 + int32(f[1][6])*u6 + int32(f[1][7])*u7
	d1 := int32(f[1][8])*w0 + int32(f[1][9])*w1 + int32(f[1][10])*w2 + int32(f[1][11])*w3 + int32(f[1][12])*w4 + int32(f[1][13])*w5 + int32(f[1][14])*w6 + int32(f[1][15])*w7
	dst[1], dst[4] = (s1+d1)>>1, (s1-d1)>>1
	s2 := int32(f[2][0])*u0 + int32(f[2][1])*u1 + int32(f[2][2])*u2 + int32(f[2][3])*u3 + int32(f[2][4])*u4 + int32(f[2][5])*u5 + int32(f[2][6])*u6 + int32(f[2][7])*u7
	d2 := int32(f[2][8])*w0 + int32(f[2][9])*w1 + int32(f[2][10])*w2 + int32(f[2][11])*w3 + int32(f[2][12])*w4 + int32(f[2][13])*w5 + int32(f[2][14])*w6 + int32(f[2][15])*w7
	dst[2], dst[3] = (s2+d2)>>1, (s2-d2)>>1
	return len(taps24k)
}

func shape48k(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	if tier0Fold {
		return shape48kFold(dst, r)
	}
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

func shape48kFold(dst *[maxSamples]int32, r *[maxTaps]int8) int {
	r0, r1, r2, r3, r4, r5 := int32(r[0]), int32(r[1]), int32(r[2]), int32(r[3]), int32(r[4]), int32(r[5])
	r6, r7, r8, r9, r10, r11 := int32(r[6]), int32(r[7]), int32(r[8]), int32(r[9]), int32(r[10]), int32(r[11])
	u0, u1, u2, u3, u4, u5 := r0+r11, r1+r10, r2+r9, r3+r8, r4+r7, r5+r6
	w0, w1, w2, w3, w4, w5 := r0-r11, r1-r10, r2-r9, r3-r8, r4-r7, r5-r6
	f := &taps48kFold
	s0 := int32(f[0][0])*u0 + int32(f[0][1])*u1 + int32(f[0][2])*u2 + int32(f[0][3])*u3 + int32(f[0][4])*u4 + int32(f[0][5])*u5
	d0 := int32(f[0][6])*w0 + int32(f[0][7])*w1 + int32(f[0][8])*w2 + int32(f[0][9])*w3 + int32(f[0][10])*w4 + int32(f[0][11])*w5
	dst[1], dst[9] = (s0+d0)>>1, (s0-d0)>>1
	s1 := int32(f[1][0])*u0 + int32(f[1][1])*u1 + int32(f[1][2])*u2 + int32(f[1][3])*u3 + int32(f[1][4])*u4 + int32(f[1][5])*u5
	d1 := int32(f[1][6])*w0 + int32(f[1][7])*w1 + int32(f[1][8])*w2 + int32(f[1][9])*w3 + int32(f[1][10])*w4 + int32(f[1][11])*w5
	dst[2], dst[8] = (s1+d1)>>1, (s1-d1)>>1
	s2 := int32(f[2][0])*u0 + int32(f[2][1])*u1 + int32(f[2][2])*u2 + int32(f[2][3])*u3 + int32(f[2][4])*u4 + int32(f[2][5])*u5
	d2 := int32(f[2][6])*w0 + int32(f[2][7])*w1 + int32(f[2][8])*w2 + int32(f[2][9])*w3 + int32(f[2][10])*w4 + int32(f[2][11])*w5
	dst[3], dst[7] = (s2+d2)>>1, (s2-d2)>>1
	s3 := int32(f[3][0])*u0 + int32(f[3][1])*u1 + int32(f[3][2])*u2 + int32(f[3][3])*u3 + int32(f[3][4])*u4 + int32(f[3][5])*u5
	d3 := int32(f[3][6])*w0 + int32(f[3][7])*w1 + int32(f[3][8])*w2 + int32(f[3][9])*w3 + int32(f[3][10])*w4 + int32(f[3][11])*w5
	dst[4], dst[6] = (s3+d3)>>1, (s3-d3)>>1
	dst[5] = int32(f[4][0])*u0 + int32(f[4][1])*u1 + int32(f[4][2])*u2 + int32(f[4][3])*u3 + int32(f[4][4])*u4 + int32(f[4][5])*u5
	dst[0] = int32(f[4][6])*(r1+r11) + int32(f[4][7])*(r2+r10) + int32(f[4][8])*(r3+r9) + int32(f[4][9])*(r4+r8) +
		int32(f[4][10])*(r5+r7) + int32(f[4][11])*r6
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
// sample over the taps of an AudioTable in read-only memory. It keeps 132
// bytes of state, allocates nothing and fits a microcontroller: Init sets it
// up in memory of the caller, such as a package variable. An MCUAudio is not
// safe for concurrent use. (ETSI TS 102 361-1 clause 10.2.2.2)
type MCUAudio struct {
	k            shaperMAC
	scale        int32
	offset       int32
	codes, zeros uint32
	idx5         uint32
	corr         bool
	rerr         int32
	rerr2        int32
	shape32      bool
	c24          bool
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
	lo, hi := t.bounds(scale)
	if offset > math.MaxInt16-hi || offset < math.MinInt16-lo {
		return errOffset
	}
	m.k.reset(t)
	m.scale = scale
	m.offset = int32(offset)
	m.codes, m.zeros, m.corr = 0, ^uint32(0), tier0Corr && (t.hz == 48000 || tier0Corr24 && t.hz == 24000)
	m.c24 = tier0Corr24 && t.hz == 24000
	m.rerr, m.rerr2 = 0, 0
	m.shape32 = tier0Shape && 4*int64(t.peak)*int64(scale)+1<<(15+tapShift) < math.MaxInt32
	m.idx5 = uint32(len(mcuCorr) - 1)
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
	if tier0Shape && m.shape32 {
		return m.shaped(dst[:n], n, s)
	}
	if tier0Corr24 && m.corr && m.c24 && 2*n == len(mcuCorr[0]) {
		if i, ok := m.corrIdx(s); ok {
			t := &mcuCorr[i]
			d := (*[len(mcuCorr[0]) / 2]int16)(dst)
			for p := range d {
				d[p] = int16(int32((int64(buf[p]+int32(t[2*p]))*sc+1<<(14+tapShift))>>(15+tapShift)) + off)
			}
			return n
		}
	}
	if tier0Corr && m.corr && !m.c24 && n == len(mcuCorr[0]) {
		if i, ok := m.corrIdx(s); ok {
			t := &mcuCorr[i]
			d := (*[len(mcuCorr[0])]int16)(dst)
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

func (m *MCUAudio) shaped(dst []int16, n int, s int8) int {
	if tier0Shape2 {
		return m.shaped2(dst, n, s)
	}
	const sh = 15 + tapShift
	const half = 1 << (sh - 1)
	sc, off, buf, e := m.scale, m.offset, &m.k.buf, m.rerr
	if tier0Corr24 && m.corr && m.c24 && 2*n == len(mcuCorr[0]) {
		if i, ok := m.corrIdx(s); ok {
			t := &mcuCorr[i]
			d := (*[len(mcuCorr[0]) / 2]int16)(dst)
			for p := range d {
				v := (buf[p]+int32(t[2*p]))*sc - e + half
				e = v&(1<<sh-1) - half
				d[p] = int16(v>>sh + off)
			}
			m.rerr = e
			return n
		}
	}
	if tier0Corr && m.corr && !m.c24 && n == len(mcuCorr[0]) {
		if i, ok := m.corrIdx(s); ok {
			t := &mcuCorr[i]
			d := (*[len(mcuCorr[0])]int16)(dst)
			for p := range d {
				v := (buf[p]+int32(t[p]))*sc - e + half
				e = v&(1<<sh-1) - half
				d[p] = int16(v>>sh + off)
			}
			m.rerr = e
			return n
		}
	}
	for p := range dst {
		v := buf[p]*sc - e + half
		e = v&(1<<sh-1) - half
		dst[p] = int16(v>>sh + off)
	}
	m.rerr = e
	return n
}

func (m *MCUAudio) shaped2(dst []int16, n int, s int8) int {
	const sh = 15 + tapShift
	const half = 1 << (sh - 1)
	sc, off, buf, e1, e2 := m.scale, m.offset, &m.k.buf, m.rerr, m.rerr2
	var t *[10]int16
	if tier0Corr && m.corr {
		if i, ok := m.corrIdx(s); ok {
			t = &mcuCorr[i]
		}
	}
	if t != nil && !m.c24 && n == len(mcuCorr[0]) {
		d := (*[10]int16)(dst)
		var v int32
		v = (buf[0]+int32(t[0]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[0] = int16(v>>sh + off)
		v = (buf[1]+int32(t[1]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[1] = int16(v>>sh + off)
		v = (buf[2]+int32(t[2]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[2] = int16(v>>sh + off)
		v = (buf[3]+int32(t[3]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[3] = int16(v>>sh + off)
		v = (buf[4]+int32(t[4]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[4] = int16(v>>sh + off)
		v = (buf[5]+int32(t[5]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[5] = int16(v>>sh + off)
		v = (buf[6]+int32(t[6]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[6] = int16(v>>sh + off)
		v = (buf[7]+int32(t[7]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[7] = int16(v>>sh + off)
		v = (buf[8]+int32(t[8]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[8] = int16(v>>sh + off)
		v = (buf[9]+int32(t[9]))*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[9] = int16(v>>sh + off)
		m.rerr, m.rerr2 = e1, e2
		return n
	}
	if t == nil && n == len(mcuCorr[0]) {
		d := (*[10]int16)(dst)
		var v int32
		v = buf[0]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[0] = int16(v>>sh + off)
		v = buf[1]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[1] = int16(v>>sh + off)
		v = buf[2]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[2] = int16(v>>sh + off)
		v = buf[3]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[3] = int16(v>>sh + off)
		v = buf[4]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[4] = int16(v>>sh + off)
		v = buf[5]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[5] = int16(v>>sh + off)
		v = buf[6]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[6] = int16(v>>sh + off)
		v = buf[7]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[7] = int16(v>>sh + off)
		v = buf[8]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[8] = int16(v>>sh + off)
		v = buf[9]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[9] = int16(v>>sh + off)
		m.rerr, m.rerr2 = e1, e2
		return n
	}
	step := 1
	if m.c24 {
		step = 2
	}
	for p := range dst[:n] {
		x := buf[p]
		if t != nil {
			x += int32(t[p*step])
		}
		v := x*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		dst[p] = int16(v>>sh + off)
	}
	m.rerr, m.rerr2 = e1, e2
	return n
}

func (m *MCUAudio) corrIdx(s int8) (uint32, bool) {
	skip := mcuCorrSkip
	if tier0Corr24 && m.c24 {
		skip += 2
	}
	if mcuCorrBase == 5 {
		r := &m.k.ring
		e := code5[(r[m.k.pos+skip]+3)&7]
		l := code5[(r[m.k.pos+skip+mcuCorrSyms]+3)&7]
		m.idx5 = (m.idx5-uint32(l)*mcuCorrTop)*5 + uint32(e)
		return m.idx5, true
	}
	z := uint32(0)
	switch s {
	case -3, -1, 1, 3:
	default:
		z, s = 1, -3
	}
	m.codes = m.codes<<2 | uint32(s+3)>>1
	m.zeros = m.zeros<<1 | z
	if m.zeros>>skip&(1<<mcuCorrSyms-1) != 0 {
		return 0, false
	}
	return m.codes >> (2 * skip) & (1<<(2*mcuCorrSyms) - 1), true
}

var code5 = [8]uint8{0, 0, 1, 4, 2, 0, 3, 0}
