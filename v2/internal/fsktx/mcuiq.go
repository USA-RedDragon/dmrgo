package fsktx

// The cosine table of MCUIQ: cosTableBits bits of phase index a turn, a
// quarter turn more lets the sine be the negative cosine a quarter on, and
// tier0IQWide, with tier0IQFine, makes cosTable 32-bit: the cosine at 16
// fraction bits plus a half for the rounding, at the middle of each
// entry, starting a quarter turn back so that the sine is the entry a
// quarter before the cosine. The
// output then has one rounding, not the two of the 16-bit table and its
// first-order term.
// tier0IQCorr adds to the deviation of MCUIQ the correction of corrIQ
// for the context of corrIQSyms symbols that ends corrIQSkip symbols
// before the newest, which cuts the power outside the channel.
const (
	tier0IQCorr = false
	tier0IQWide = false
)

const (
	cosTableBits = 12
	iqAmplitude  = 29491
)

// The shaper of MCUIQ: iqHz is its rate, iqPhases its samples of a symbol
// period and iqSpan the symbols of the pulse of tapsIQ on each side.
const (
	iqPhases = len(tapsIQ)
	iqHz     = iqPhases * SymbolRate
	iqSpan   = len(tapsIQ[0]) / 2
	iqWords  = (len(tapsIQ[0]) + 3) / 4
)

// MCUIQ is the Tier0 I/Q modulator, for a software radio driven by a
// microcontroller: the shaping of MCUAudio at 24 kHz over a pulse of four
// symbols on each side, a 32-bit phase accumulator and a cosine table of
// 4096 entries, into interleaved 16-bit I/Q at 0.9 of full scale at 2, 4
// or 8 times 24 kHz, with the phase step interpolated linearly between
// the samples at 24 kHz. Key ramps the power up and down over a raised
// cosine for TDMA bursts. It keeps about 90 bytes of state, allocates
// nothing and fits a microcontroller: Init sets it up in memory of the
// caller. An MCUIQ is not safe for concurrent use.
type MCUIQ struct {
	// hist holds a byte for each symbol of the pulse, the newest in the
	// low byte of hist[0]: 5 times the code of the symbol plus the code
	// of the one before, the index of tapsIQLUT. The codes 0 to 4 are the
	// symbols -3, -1, +1, +3 and 0.
	hist    [iqWords]uint32
	buf     [iqPhases]int32
	values  int32
	upShift uint32
	phase   uint32
	step    int32
	off     int32
	prev    int32
	env     int32
	ramp    int32
	// envQ and envR are the quotient and remainder of env times half
	// the cosine table by ramp, the index of the ramp in cosTable, and
	// stepQ and stepR those of one step of env.
	envQ, envR   int32
	stepQ, stepR int32
	// codes holds 2 bits for each recent symbol, the newest lowest, and
	// zeros a bit for each that is 0, for tier0IQCorr.
	codes, zeros uint32
	on           bool
}

// Init sets up m for I/Q at rate, 48000, 96000 or 192000 Hz, with the
// carrier offsetHz from 0 Hz, within a quarter of rate. rampSamples is the
// length of the power ramp of Key at rate, from 1 to 32768. m starts
// keyed down. On an error m stays as it was.
func (m *MCUIQ) Init(rate, offsetHz, rampSamples int) error {
	switch {
	case rate != 48000 && rate != 96000 && rate != 192000, rate < iqHz:
		return errRate
	case offsetHz >= rate/4 || offsetHz <= -rate/4:
		return errCarrier
	case rampSamples < 1 || rampSamples > 1<<15:
		return errRamp
	}
	*m = MCUIQ{ramp: int32(rampSamples), zeros: ^uint32(0)}
	for i := range &m.hist {
		m.hist[i] = 0x18181818
	}
	m.stepQ = int32(1 << cosTableBits / 2 / rampSamples)
	m.stepR = int32(1 << cosTableBits / 2 % rampSamples)
	for rate > iqHz<<m.upShift {
		m.upShift++
	}
	m.values = int32(2 * iqPhases << m.upShift)
	m.step = int32((uint64(1944)<<32/tableUnits + uint64(rate/2)) / uint64(rate))
	m.off = int32((int64(offsetHz)<<32 + int64(rate/2)) / int64(rate))
	m.prev = m.off
	return nil
}

// Samples returns the number of I/Q samples of each symbol; Symbol writes
// twice as many values.
func (m *MCUIQ) Samples() int { return int(m.values / 2) }

// Key keys the carrier up or down: the power ramps over the samples of
// Init from where it is.
func (m *MCUIQ) Key(on bool) { m.on = on }

// Symbol shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation, and
// writes the I/Q of the symbol period that it completes to dst,
// interleaved. It returns the number of values it wrote. With a dst
// shorter than 2*Samples, or before Init, it writes nothing, does not take
// the symbol and returns 0. The output lags the symbols by four symbol
// periods and one sample of the shaper, at 24 kHz.
func (m *MCUIQ) Symbol(dst []int16, s int8) int {
	n := int(m.values)
	if n == 0 || len(dst) < n {
		return 0
	}
	var c uint32
	switch s {
	case -3:
		c = 0
	case -1:
		c = 1
	case 1:
		c = 2
	case 3:
		c = 3
	default:
		c = 4
	}
	for i := iqWords - 1; i > 0; i-- {
		m.hist[i] = m.hist[i]<<8 | m.hist[i-1]>>24
	}
	m.hist[0] = m.hist[0]<<8 | 5*c + m.hist[0]&255/5
	m.shape(c)
	dst = dst[:n]
	switch {
	case m.on && m.env == m.ramp:
		switch m.upShift {
		case 0:
			m.steady0(dst)
		case 1:
			m.steady1(dst)
		case 2:
			m.steady2(dst)
		default:
			m.steady3(dst)
		}
	case !m.on && m.env == 0:
		m.silent()
		for len(dst) >= 4 {
			*(*[4]int16)(dst) = [4]int16{}
			dst = dst[4:]
		}
	default:
		m.ramping(dst)
	}
	return n
}

// shape writes the deviation of the samples of the symbol period that
// the symbols of hist complete to buf, in output units: for each two
// phases, the sum of the words of tapsIQLUT of each two symbols, plus with
// tier0IQCorr the correction of corr for the newest symbol of code c. The
// sums are those of tapsIQ.
func (m *MCUIQ) shape(c uint32) {
	var acc [len(tapsIQLUT[0][0])]int32
	if tier0IQCorr {
		acc = m.corr(c)
	}
	for j := range tapsIQLUT {
		t := &tapsIQLUT[j][m.hist[j/2]>>(16*(j%2))&31]
		for g := range acc {
			acc[g] += t[g]
		}
	}
	for g, y := range acc {
		y0 := int32(int16(y))
		m.buf[2*g] = y0
		if 2*g+1 < iqPhases {
			m.buf[2*g+1] = (y - y0) >> 16
		}
	}
}

// The shifts, quarter turn and mask of the index of cosTable.
const (
	cosShift   = 32 - cosTableBits
	cosQuarter = 1 << cosTableBits / 4
	cosMask    = 1<<cosTableBits - 1
)

// cosSin returns the cosine and sine of phase from cosTable; with
// tier0IQFine, corrected to first order for the bits of phase below its
// index, which leaves the phase error at the square of half an index.
func cosSin(phase uint32) (int16, int16) {
	i := phase >> cosShift
	c, ns := int32(cosTable[i]), int32(cosTable[i+cosQuarter])
	if tier0IQFine && tier0IQWide {
		// m is the phase from the middle of the entry in 2^-32 radians,
		// signed so that the products are those of a signed multiply.
		m := int32(uint64(phase<<cosTableBits)*cosFracWide>>32) - cosFracWide/2
		sn, c := int32(cosTable[i]), int32(cosTable[i+cosQuarter])
		co := c - int32(int64(sn)*int64(m)>>32)
		si := sn + int32(int64(c)*int64(m)>>32)
		return int16(co >> 16), int16(si >> 16)
	}
	if tier0IQFine {
		// m is the phase below the index in 2^-24 radians.
		m := int32(uint64(phase<<cosTableBits) * cosFrac >> 32)
		return int16(c + ns*m>>24), int16(c*m>>24 - ns)
	}
	return int16(c), int16(-ns)
}

// cosFrac is 2 pi 2^24 over the entries of cosTable, rounded, and
// cosFracWide 2 pi 2^32 over them, for a table of 2^10 entries or more.
const (
	cosFrac     = 25736
	cosFracWide = 26353589 >> (cosTableBits - 10)
)

// steady0 writes the I/Q of the deviation samples of the shaper to dst at
// full power and the shaper rate, the path of all but the samples of a
// power ramp. steady1, steady2 and steady3 are steady0 at two, four and
// eight times the shaper rate, with the phase step interpolated linearly;
// with tier0Exact, at the midpoints of the output samples.
func (m *MCUIQ) steady0(dst []int16) {
	phase, step, off := m.phase, m.step, m.off
	prev := m.prev
	for p, x := range &m.buf {
		w := x*step + off
		v := w
		if tier0Exact {
			v = prev + (w-prev)>>1
		}
		prev = w
		d := dst[2*p : 2*p+2 : 2*p+2]
		phase += uint32(v)
		d[0], d[1] = cosSin(phase)
	}
	m.phase, m.prev = phase, prev
}

func (m *MCUIQ) steady1(dst []int16) {
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	for p, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> 1
		v := prev + dw
		if tier0Exact {
			v -= dw >> 1
		}
		prev = w
		d := dst[4*p : 4*p+4 : 4*p+4]
		phase += uint32(v)
		d[0], d[1] = cosSin(phase)
		phase += uint32(v + dw)
		d[2], d[3] = cosSin(phase)
	}
	m.phase, m.prev = phase, prev
}

func (m *MCUIQ) steady2(dst []int16) {
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	for p, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> 2
		v := prev
		if tier0Exact {
			v -= dw >> 1
		}
		prev = w
		d := (*[8]int16)(dst[8*p : 8*p+8])
		for j := 0; j < 8; j += 2 {
			v += dw
			phase += uint32(v)
			d[j], d[j+1] = cosSin(phase)
		}
	}
	m.phase, m.prev = phase, prev
}

func (m *MCUIQ) steady3(dst []int16) {
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	for p, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> 3
		v := prev
		if tier0Exact {
			v -= dw >> 1
		}
		prev = w
		d := (*[16]int16)(dst[16*p : 16*p+16])
		for j := 0; j < 16; j += 2 {
			v += dw
			phase += uint32(v)
			d[j], d[j+1] = cosSin(phase)
		}
	}
	m.phase, m.prev = phase, prev
}

// silent steps the phase over the l deviation samples of the shaper as
// steady does, with no output: over the 2^u samples of a shaper sample
// it moves by 2^u times prev and 2^u(2^u+1)/2 times the step dw.
func (m *MCUIQ) silent() {
	phase, prev, step, off, us := m.phase, m.prev, m.step, m.off, m.upShift&31
	tri := uint32(1<<us) * (1<<us + 1) / 2
	for _, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> us
		v := prev
		if tier0Exact {
			v -= dw >> 1
		}
		phase += uint32(v)<<us + uint32(dw)*tri
		prev = w
	}
	m.phase, m.prev = phase, prev
}

// ramping is steady on the power ramp: it steps the ramp each sample, up
// to ramp while keyed up and down to 0 while keyed down. The amplitude is
// (1 - cos) / 2 in Q15 at env * (1 << cosTableBits / 2) / ramp, from
// cosTable of amplitude iqAmplitude. It stays out of line, so that it does
// not take the registers of the steady paths.
//
//go:noinline
func (m *MCUIQ) ramping(dst []int16) {
	us := m.upShift & 31
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	env, q, r, ramp, sq, sr, on := m.env, m.envQ, m.envR, m.ramp, m.stepQ, m.stepR, m.on
	j := 0
	for _, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> us
		v := prev
		if tier0Exact {
			v -= dw >> 1
		}
		prev = w
		for range 1 << us {
			v += dw
			phase += uint32(v)
			switch {
			case on && env < ramp:
				env++
				q += sq
				r += sr
				if r >= ramp {
					r -= ramp
					q++
				}
			case !on && env > 0:
				env--
				q -= sq
				r -= sr
				if r < 0 {
					r += ramp
					q--
				}
			}
			rc := int32(cosTable[q&cosMask])
			if tier0IQFine && tier0IQWide {
				rc = int32(cosTable[q&cosMask+cosQuarter]) >> 16
			}
			a := int32(uint32(iqAmplitude-rc) * 32768 / (2 * iqAmplitude))
			d := dst[j : j+2 : j+2]
			c, sn := cosSin(phase)
			d[0] = int16(int32(c) * a >> 15)
			d[1] = int16(int32(sn) * a >> 15)
			j += 2
		}
	}
	m.phase, m.prev, m.env, m.envQ, m.envR = phase, prev, env, q, r
}

// corr takes the symbol of code c into the context and returns the
// correction in the words of tapsIQLUT: for each table k of corrIQ, the
// row of the context k symbols newer than corrIQSkip, none while any of
// these contexts has a symbol 0. With corrIQSym a context whose oldest
// symbol is +1 or +3 takes the negated row of the negated symbols.
func (m *MCUIQ) corr(c uint32) (acc [len(tapsIQLUT[0][0])]int32) {
	const mask = 1<<(2*corrIQSyms) - 1
	const span = corrIQSyms + corrIQLen - 1
	z := uint32(0)
	if c == 4 {
		z, c = 1, 0
	}
	codes, zeros := m.codes<<2|c, m.zeros<<1|z
	m.codes, m.zeros = codes, zeros
	if zeros>>(corrIQSkip-corrIQLen+1)&(1<<span-1) != 0 {
		return acc
	}
	for k := range corrIQLen {
		i := codes >> uint32(2*(corrIQSkip-k)) & mask
		if corrIQSym && i > mask/2 {
			t := &corrIQ[k][(i^mask)&uint32(len(corrIQ[0])-1)]
			for g := range acc {
				acc[g] -= t[g]
			}
			continue
		}
		t := &corrIQ[k][i&uint32(len(corrIQ[0])-1)]
		for g := range acc {
			acc[g] += t[g]
		}
	}
	return acc
}
