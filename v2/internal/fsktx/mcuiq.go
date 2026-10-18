package fsktx

// The cosine table of MCUIQ: cosTableBits bits of phase index it, and
// iqAmplitude is its amplitude, 0.9 of the full scale of 16 bits.
const (
	cosTableBits = 12
	iqAmplitude  = 29491
)

// MCUIQ is the Tier0 I/Q modulator, for a software radio driven by a
// microcontroller: the shaping of MCUAudio at 24 kHz over a pulse of four
// symbols on each side, a 32-bit phase accumulator and a cosine table of
// 4096 entries, into interleaved 16-bit I/Q at 0.9 of full scale at 2, 4
// or 8 times 24 kHz, with the phase step interpolated linearly between
// the samples at 24 kHz. Key ramps the power up and down over a raised
// cosine for TDMA bursts. It keeps about 120 bytes of state, allocates
// nothing and fits a microcontroller: Init sets it up in memory of the
// caller. An MCUIQ is not safe for concurrent use.
type MCUIQ struct {
	k       shaperMAC
	upShift uint32
	phase   uint32
	step    int32
	off     int32
	prev    int32
	env     int32
	ramp    int32
	on      bool
}

// Init sets up m for I/Q at rate, 48000, 96000 or 192000 Hz, with the
// carrier offsetHz from 0 Hz, within a quarter of rate. rampSamples is the
// length of the power ramp of Key at rate, from 1 to 32768. m starts
// keyed down. On an error m stays as it was.
func (m *MCUIQ) Init(rate, offsetHz, rampSamples int) error {
	t := tableIQ()
	switch {
	case rate != 2*t.hz && rate != 4*t.hz && rate != 8*t.hz:
		return errRate
	case rampSamples < 1 || rampSamples > 1<<15 || offsetHz >= rate/4 || offsetHz <= -rate/4:
		return errLevel
	}
	*m = MCUIQ{ramp: int32(rampSamples)}
	for rate > t.hz<<m.upShift {
		m.upShift++
	}
	m.k.reset(t)
	// The phase step of one output unit, 1944/15000 Hz, in 2^-32 turns.
	m.step = int32((uint64(1944)<<32/tableUnits + uint64(rate/2)) / uint64(rate))
	m.off = int32((int64(offsetHz)<<32 + int64(rate/2)) / int64(rate))
	return nil
}

// Samples returns the number of I/Q samples of each symbol; Symbol writes
// twice as many values.
func (m *MCUIQ) Samples() int { return m.k.t.hz / SymbolRate << m.upShift }

// Key keys the carrier up or down: the power ramps over the samples of
// Init from where it is.
func (m *MCUIQ) Key(on bool) { m.on = on }

// Symbol shapes the symbol s, -3, -1, +1, +3 or 0 for no modulation, and
// writes the I/Q of the symbol period that it completes to dst,
// interleaved. It returns the number of values it wrote. A dst shorter
// than 2*Samples takes nothing, and Symbol returns 0 without the symbol;
// so does an m without Init. The output lags the symbols by four symbol
// periods and one sample at 24 kHz.
func (m *MCUIQ) Symbol(dst []int16, s int8) int {
	if len(dst) < 2*m.Samples() {
		return 0
	}
	l := m.k.symbol(s)
	n := 2 * l << m.upShift
	dst = dst[:n]
	switch {
	case m.on && m.env == m.ramp:
		m.steady(dst, l)
	case !m.on && m.env == 0:
		m.steady(dst, l)
		clear(dst)
	default:
		m.ramping(dst, l)
	}
	return n
}

// steady writes the I/Q of the l deviation samples of the shaper to dst
// at full power, the path of all but the samples of a power ramp.
func (m *MCUIQ) steady(dst []int16, l int) {
	const sh = 32 - cosTableBits
	const quarter = 1 << cosTableBits / 4
	const mask = 1<<cosTableBits - 1
	phase, prev, step, off, us := m.phase, m.prev, m.step, m.off, m.upShift
	um := 1<<us - 1
	n := l << us
	dst = dst[:2*n]
	var v, dw int32
	for k := 0; k < n; k++ {
		if k&um == 0 {
			w := m.k.buf[k>>us]*step + off
			dw = (w - prev) >> us
			v, prev = prev, w
		}
		v += dw
		phase += uint32(v)
		i := phase >> sh
		dst[2*k] = cosTable[i]
		dst[2*k+1] = cosTable[(i-quarter)&mask]
	}
	m.phase, m.prev = phase, prev
}

// ramping is steady on the power ramp: it steps the ramp each sample.
func (m *MCUIQ) ramping(dst []int16, l int) {
	const sh = 32 - cosTableBits
	const quarter = 1 << cosTableBits / 4
	const mask = 1<<cosTableBits - 1
	us := m.upShift
	j := 0
	for p := range l {
		w := m.k.buf[p]*m.step + m.off
		dw := (w - m.prev) >> us
		v := m.prev
		m.prev = w
		for range 1 << us {
			v += dw
			m.phase += uint32(v)
			i := m.phase >> sh
			a := m.envelope()
			dst[j] = int16(int32(cosTable[i]) * a >> 15)
			dst[j+1] = int16(int32(cosTable[(i-quarter)&mask]) * a >> 15)
			j += 2
		}
	}
}

// envelope steps the power ramp and returns the amplitude at its new
// place, in Q15: a raised cosine from the cosine table.
func (m *MCUIQ) envelope() int32 {
	switch {
	case m.on && m.env < m.ramp:
		m.env++
	case !m.on && m.env > 0:
		m.env--
	}
	i := m.env * (1 << cosTableBits / 2) / m.ramp
	// (1 - cos) / 2 in Q15, from a table of amplitude iqAmplitude.
	return (iqAmplitude - int32(cosTable[i])) * 32768 / (2 * iqAmplitude)
}
