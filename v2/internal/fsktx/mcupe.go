package fsktx

import "math"

// PESpan is the Span of MCUAudioPE.
const PESpan = peSpan

const (
	peSamples = 10
	peSpan    = len(taps24kPre[0])/2 + 1
	pePeak    = taps24kPrePeak * 5 / 4
)

// MCUAudioPE is the Tier0 modulator of MCUAudio for a radio that
// pre-emphasizes its modulator input with a zero at 300 Hz and a pole at
// 3 kHz, at 48 kHz: the pulse at 24 kHz with the inverse of that
// pre-emphasis folded in, its tail past 8 symbols as a one-pole recursion,
// then a 4-tap half-band interpolation by 2 and a correction table fitted
// so that the output through the radio matches that of MCUAudio with
// Table48k through a flat radio. It is a type of its own so that a program
// of MCUAudio links none of it. Built by TinyGo 0.43 at -opt z for a
// Cortex-M4 it takes 3.4 MHz in 23.8 KB of flash, with 132 bytes of
// state. It allocates nothing and is not safe for concurrent use.
type MCUAudioPE struct {
	ring         [2 * maxTaps]int8
	pos          int
	buf          [maxSamples]int32
	prev         [6]int32
	e            int32
	scale        int32
	offset       int32
	codes, zeros uint32
	rerr, rerr2  int32
	shape32      bool
}

func peBounds(scale int32) (lo, hi int) {
	peak := 3*int64(pePeak) + mcuCorrPEPeak
	hi = int((peak*int64(scale)+1<<(14+tapShift))>>(15+tapShift)) + 1
	lo = int((-peak*int64(scale)+1<<(14+tapShift))>>(15+tapShift)) - 1
	return lo, hi
}

// PeakPE returns the most that the output of an MCUAudioPE at level
// reaches from its offset, in codes, over every run of symbols.
func PeakPE(level int) int {
	lo, hi := peBounds(int32((level<<15 + tableUnits/2) / tableUnits))
	return max(-lo, hi)
}

// Init sets up m at level units for a run of symbol +3 around offset
// units, as MCUAudio.Init does. On an error m stays as it was; otherwise
// it starts with no modulation.
func (m *MCUAudioPE) Init(level, offset int) error {
	if level < 1 || level > 16000 {
		return errLevel
	}
	scale := int32((level<<15 + tableUnits/2) / tableUnits)
	lo, hi := peBounds(scale)
	if offset > math.MaxInt16-hi || offset < math.MinInt16-lo {
		return errOffset
	}
	*m = MCUAudioPE{scale: scale, offset: int32(offset), zeros: ^uint32(0)}
	m.shape32 = 4*int64(pePeak)*int64(scale)+1<<(15+tapShift) < math.MaxInt32
	return nil
}

// Samples returns the number of samples of each symbol, 10.
func (m *MCUAudioPE) Samples() int { return peSamples }

// Span returns the lag of the output behind its symbols, in symbol
// periods: 9.
func (m *MCUAudioPE) Span() int { return peSpan }

// Symbol shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation, and
// writes the 10 samples of the symbol period that it completes to dst. It
// returns the number it wrote. With a dst shorter than 10, or before Init,
// it writes nothing, does not take the symbol and returns 0. The output
// lags the symbols by Span symbol periods.
func (m *MCUAudioPE) Symbol(dst []int16, s int8) int {
	if len(dst) < peSamples || m.scale == 0 {
		return 0
	}
	switch s {
	case -3, -1, 1, 3:
	default:
		s = 0
	}
	m.pos--
	if m.pos < 0 {
		m.pos = maxTaps - 1
	}
	d := m.ring[m.pos]
	m.ring[m.pos] = s
	m.ring[m.pos+maxTaps] = s
	var n int
	m.e, n = shape24kPre(&m.buf, (*[maxTaps]int8)(m.ring[m.pos:m.pos+maxTaps]), d, m.e)
	if n != peSamples/2 {
		return 0
	}
	i, ok := m.corrIdx(s)
	m.interp(i, ok)
	if tier0Shape && m.shape32 {
		m.shaped((*[peSamples]int16)(dst))
		return peSamples
	}
	sc, off, b := int64(m.scale), m.offset, &m.buf
	for p := range dst[:peSamples] {
		dst[p] = int16(int32((int64(b[p])*sc+1<<(14+tapShift))>>(15+tapShift)) + off)
	}
	return peSamples
}

func (m *MCUAudioPE) interp(i uint32, ok bool) {
	b, h := &m.buf, &m.prev
	q, p0, p1, p2, p3, p4 := h[0], h[1], h[2], h[3], h[4], h[5]
	c0, c1 := b[0], b[1]
	h[0], h[1], h[2], h[3], h[4], h[5] = p4, b[0], b[1], b[2], b[3], b[4]
	b[0], b[1] = p0, (9*(p0+p1)-q-p2+8)>>4
	b[2], b[3] = p1, (9*(p1+p2)-p0-p3+8)>>4
	b[4], b[5] = p2, (9*(p2+p3)-p1-p4+8)>>4
	b[6], b[7] = p3, (9*(p3+p4)-p2-c0+8)>>4
	b[8], b[9] = p4, (9*(p4+c0)-p3-c1+8)>>4
	if tier0Corr && ok {
		corrPE(b, i)
	}
}

func corrPE(b *[maxSamples]int32, i uint32) {
	t := &mcuCorrPE[i]
	b[0] += int32(t[0])
	b[1] += int32(t[1])
	b[2] += int32(t[2])
	b[3] += int32(t[3])
	b[4] += int32(t[4])
	b[5] += int32(t[5])
	b[6] += int32(t[6])
	b[7] += int32(t[7])
	b[8] += int32(t[8])
	b[9] += int32(t[9])
}

func (m *MCUAudioPE) shaped(d *[peSamples]int16) {
	const sh = 15 + tapShift
	const half = 1 << (sh - 1)
	sc, off, b, e1, e2 := m.scale, m.offset, &m.buf, m.rerr, m.rerr2
	for p := range d {
		v := b[p]*sc + e2>>1 + half
		e1, e2 = v&(1<<sh-1)-half+1, e1
		d[p] = int16(v>>sh + off)
	}
	m.rerr, m.rerr2 = e1, e2
}

func (m *MCUAudioPE) corrIdx(s int8) (uint32, bool) {
	z := uint32(0)
	switch s {
	case -3, -1, 1, 3:
	default:
		z, s = 1, -3
	}
	m.codes = m.codes<<2 | uint32(s+3)>>1
	m.zeros = m.zeros<<1 | z
	if m.zeros>>mcuCorrPESkip&(1<<mcuCorrSyms-1) != 0 {
		return 0, false
	}
	return m.codes >> (2 * mcuCorrPESkip) & (1<<(2*mcuCorrSyms) - 1), true
}

func shape24kPre(dst *[maxSamples]int32, r *[maxTaps]int8, d int8, e int32) (int32, int) {
	e = int32((int64(e)*tail24kRN+1<<29)>>30) + int32(d)*tail24kG
	r0 := int32(r[0])
	r1 := int32(r[1])
	r2 := int32(r[2])
	r3 := int32(r[3])
	r4 := int32(r[4])
	r5 := int32(r[5])
	r6 := int32(r[6])
	r7 := int32(r[7])
	r8 := int32(r[8])
	r9 := int32(r[9])
	r10 := int32(r[10])
	r11 := int32(r[11])
	r12 := int32(r[12])
	r13 := int32(r[13])
	r14 := int32(r[14])
	r15 := int32(r[15])
	f := &taps24kPre
	dst[0] = int32((int64(e)*int64(tail24kRP[0])+1<<37)>>38) + int32(f[0][0])*r0 + int32(f[0][1])*r1 + int32(f[0][2])*r2 + int32(f[0][3])*r3 + int32(f[0][4])*r4 + int32(f[0][5])*r5 + int32(f[0][6])*r6 + int32(f[0][7])*r7 + int32(f[0][8])*r8 + int32(f[0][9])*r9 + int32(f[0][10])*r10 + int32(f[0][11])*r11 + int32(f[0][12])*r12 + int32(f[0][13])*r13 + int32(f[0][14])*r14 + int32(f[0][15])*r15
	dst[1] = int32((int64(e)*int64(tail24kRP[1])+1<<37)>>38) + int32(f[1][0])*r0 + int32(f[1][1])*r1 + int32(f[1][2])*r2 + int32(f[1][3])*r3 + int32(f[1][4])*r4 + int32(f[1][5])*r5 + int32(f[1][6])*r6 + int32(f[1][7])*r7 + int32(f[1][8])*r8 + int32(f[1][9])*r9 + int32(f[1][10])*r10 + int32(f[1][11])*r11 + int32(f[1][12])*r12 + int32(f[1][13])*r13 + int32(f[1][14])*r14 + int32(f[1][15])*r15
	dst[2] = int32((int64(e)*int64(tail24kRP[2])+1<<37)>>38) + int32(f[2][0])*r0 + int32(f[2][1])*r1 + int32(f[2][2])*r2 + int32(f[2][3])*r3 + int32(f[2][4])*r4 + int32(f[2][5])*r5 + int32(f[2][6])*r6 + int32(f[2][7])*r7 + int32(f[2][8])*r8 + int32(f[2][9])*r9 + int32(f[2][10])*r10 + int32(f[2][11])*r11 + int32(f[2][12])*r12 + int32(f[2][13])*r13 + int32(f[2][14])*r14 + int32(f[2][15])*r15
	dst[3] = int32((int64(e)*int64(tail24kRP[3])+1<<37)>>38) + int32(f[3][0])*r0 + int32(f[3][1])*r1 + int32(f[3][2])*r2 + int32(f[3][3])*r3 + int32(f[3][4])*r4 + int32(f[3][5])*r5 + int32(f[3][6])*r6 + int32(f[3][7])*r7 + int32(f[3][8])*r8 + int32(f[3][9])*r9 + int32(f[3][10])*r10 + int32(f[3][11])*r11 + int32(f[3][12])*r12 + int32(f[3][13])*r13 + int32(f[3][14])*r14 + int32(f[3][15])*r15
	dst[4] = int32((int64(e)*int64(tail24kRP[4])+1<<37)>>38) + int32(f[4][0])*r0 + int32(f[4][1])*r1 + int32(f[4][2])*r2 + int32(f[4][3])*r3 + int32(f[4][4])*r4 + int32(f[4][5])*r5 + int32(f[4][6])*r6 + int32(f[4][7])*r7 + int32(f[4][8])*r8 + int32(f[4][9])*r9 + int32(f[4][10])*r10 + int32(f[4][11])*r11 + int32(f[4][12])*r12 + int32(f[4][13])*r13 + int32(f[4][14])*r14 + int32(f[4][15])*r15
	return e, 5
}
