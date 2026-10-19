package fsktx

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
	iqWords  = len(tapsIQ[0]) / 4
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
	// low byte of hist[0]: 8 times the code of the symbol plus the code
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
	*m = MCUIQ{ramp: int32(rampSamples)}
	for i := range &m.hist {
		m.hist[i] = 0x24242424
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
	m.hist[0] = m.hist[0]<<8 | c<<3 | m.hist[0]>>3&7
	m.shape()
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
// phases, the sum of the words of tapsIQLUT of each two symbols. The sums
// are those of tapsIQ.
func (m *MCUIQ) shape() {
	var q [len(tapsIQLUT[0])]uint32
	for j := range q {
		q[j] = m.hist[j/2] >> (16 * (j % 2)) & 63
	}
	for g := range tapsIQLUT {
		t := &tapsIQLUT[g]
		var y int32
		for j := range t {
			y += t[j][q[j]]
		}
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

// steady0 writes the I/Q of the deviation samples of the shaper to dst at
// full power and the shaper rate, the path of all but the samples of a
// power ramp. steady1, steady2 and steady3 are steady0 at two, four and
// eight times the shaper rate, with the phase step interpolated linearly.
func (m *MCUIQ) steady0(dst []int16) {
	phase, step, off := m.phase, m.step, m.off
	var w int32
	for p, x := range &m.buf {
		w = x*step + off
		d := dst[2*p : 2*p+2 : 2*p+2]
		phase += uint32(w)
		i := phase >> cosShift
		d[0], d[1] = cosTable[i], cosTable[(i-cosQuarter)&cosMask]
	}
	m.phase, m.prev = phase, w
}

func (m *MCUIQ) steady1(dst []int16) {
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	for p, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> 1
		v := prev + dw
		prev = w
		d := dst[4*p : 4*p+4 : 4*p+4]
		phase += uint32(v)
		i := phase >> cosShift
		d[0], d[1] = cosTable[i], cosTable[(i-cosQuarter)&cosMask]
		phase += uint32(v + dw)
		i = phase >> cosShift
		d[2], d[3] = cosTable[i], cosTable[(i-cosQuarter)&cosMask]
	}
	m.phase, m.prev = phase, prev
}

func (m *MCUIQ) steady2(dst []int16) {
	phase, prev, step, off := m.phase, m.prev, m.step, m.off
	for p, x := range &m.buf {
		w := x*step + off
		dw := (w - prev) >> 2
		v := prev
		prev = w
		d := (*[8]int16)(dst[8*p : 8*p+8])
		for j := 0; j < 8; j += 2 {
			v += dw
			phase += uint32(v)
			i := phase >> cosShift
			d[j], d[j+1] = cosTable[i], cosTable[(i-cosQuarter)&cosMask]
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
		prev = w
		d := (*[16]int16)(dst[16*p : 16*p+16])
		for j := 0; j < 16; j += 2 {
			v += dw
			phase += uint32(v)
			i := phase >> cosShift
			d[j], d[j+1] = cosTable[i], cosTable[(i-cosQuarter)&cosMask]
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
		phase += uint32(prev)<<us + uint32(dw)*tri
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
		prev = w
		for range 1 << us {
			v += dw
			phase += uint32(v)
			i := phase >> cosShift
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
			a := int32(uint32(iqAmplitude-int32(cosTable[q&cosMask])) * 32768 / (2 * iqAmplitude))
			d := dst[j : j+2 : j+2]
			d[0] = int16(int32(cosTable[i]) * a >> 15)
			d[1] = int16(int32(cosTable[(i-cosQuarter)&cosMask]) * a >> 15)
			j += 2
		}
	}
	m.phase, m.prev, m.env, m.envQ, m.envR = phase, prev, env, q, r
}
