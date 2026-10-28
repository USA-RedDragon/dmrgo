package fsktx

import "unsafe"

const (
	tier0IQCorr = true
	tier0IQWide = true
)

const (
	cosTableBits = 11
	iqAmplitude  = 32600
)

const (
	iqPhases = len(tapsIQ)
	iqHz     = iqPhases * SymbolRate
	iqSpan   = len(tapsIQ[0]) / 2
	iqWords  = (len(tapsIQ[0]) + 3) / 4
)

// MCUIQ is the Tier0 I/Q modulator, for a software radio driven by a
// microcontroller: a fixed-point shaper at 48 kHz over a pulse of eight
// symbols on each side, a 32-bit phase accumulator and a 32-bit cosine
// table of 2048 entries, into interleaved 16-bit I/Q at 0.995 of full
// scale at 1, 2 or 4 times 48 kHz, with the phase step interpolated
// linearly between the samples at 48 kHz. Key ramps the power up and down
// over a raised cosine for TDMA bursts. It keeps about 120 bytes of state,
// allocates nothing and fits a microcontroller: Init sets it up in memory
// of the caller. An MCUIQ is not safe for concurrent use.
type MCUIQ struct {
	buf          [iqPhases]int32
	hist         [iqWords]uint32
	values       int32
	upShift      uint32
	phase        uint32
	step         int32
	off          int32
	prev         int32
	env          int32
	ramp         int32
	envQ, envR   int32
	stepQ, stepR int32
	codes, zeros uint32
	frac         uint32
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
	*m = MCUIQ{ramp: int32(rampSamples), zeros: ^uint32(0), frac: cosFracWide}
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
// the symbol and returns 0. The output lags the symbols by eight symbol
// periods and one sample of the shaper, at 48 kHz.
//
//go:nobounds
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
	if iqWords == 4 {
		h := (*[4]uint32)(unsafe.Pointer(&m.hist))
		h[3] = h[3]<<8 | h[2]>>24
		h[2] = h[2]<<8 | h[1]>>24
		h[1] = h[1]<<8 | h[0]>>24
	} else {
		for i := iqWords - 1; i > 0; i-- {
			m.hist[i] = m.hist[i]<<8 | m.hist[i-1]>>24
		}
	}
	m.hist[0] = m.hist[0]<<8 | 5*c + m.hist[0]&255*13>>6
	m.shape(c)
	dst = dst[:n]
	word := littleEndian && uintptr(unsafe.Pointer(unsafe.SliceData(dst)))&3 == 0
	switch {
	case m.on && m.env == m.ramp:
		if word && tier0IQFine && tier0IQWide && cosFracWide%2 == 0 && m.upShift == 1 {
			m.steady1(dst)
		} else {
			m.steady(dst, word)
		}
	case !m.on && m.env == 0:
		m.silent(dst)
	default:
		m.ramping(dst)
	}
	return n
}

func (m *MCUIQ) shape(c uint32) {
	if len(tapsIQLUT[0][0]) == 5 {
		m.shape5(c)
		return
	}
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
		m.buf[2*g] = y0*m.step + m.off
		if 2*g+1 < iqPhases {
			m.buf[2*g+1] = (y-y0)>>16*m.step + m.off
		}
	}
}

//go:nobounds
func (m *MCUIQ) shape5(c uint32) {
	var a0, a1, a2, a3, a4 int32
	if tier0IQCorr {
		a0, a1, a2, a3, a4 = m.corr5(c)
	}
	for w := range iqWords {
		h := m.hist[w]
		t := (*[5]int32)(tapsIQLUT[2*w][h&31][:])
		a0 += t[0]
		a1 += t[1]
		a2 += t[2]
		a3 += t[3]
		a4 += t[4]
		if len(tapsIQLUT) == 2*iqWords || 2*w+1 < len(tapsIQLUT) {
			t = (*[5]int32)(tapsIQLUT[2*w+1][h>>16&31][:])
			a0 += t[0]
			a1 += t[1]
			a2 += t[2]
			a3 += t[3]
			a4 += t[4]
		}
	}
	b, step, off := &m.buf, m.step, m.off
	split(b, 0, a0, step, off)
	split(b, 1, a1, step, off)
	split(b, 2, a2, step, off)
	split(b, 3, a3, step, off)
	split(b, 4, a4, step, off)
}

func split(b *[iqPhases]int32, g int, y, step, off int32) {
	y0 := int32(int16(y))
	b[2*g] = y0*step + off
	if 2*g+1 < iqPhases {
		b[2*g+1] = (y-y0)>>16*step + off
	}
}

const (
	cosShift   = 32 - cosTableBits
	cosQuarter = 1 << cosTableBits / 4
	cosMask    = 1<<cosTableBits - 1
)

func cosSin(phase uint32) (int16, int16) {
	i := phase >> cosShift
	c, ns := cosTable[i], cosTable[i+cosQuarter]
	if tier0IQFine && tier0IQWide {
		m := cosFrac32(phase)
		sn, c := cosTable[i], cosTable[i+cosQuarter]
		co := c - int32(int64(sn)*int64(m)>>32)
		si := sn + int32(int64(c)*int64(m)>>32)
		return int16(co >> 16), int16(si >> 16)
	}
	if tier0IQFine {
		m := int32(uint64(phase<<cosTableBits) * cosFrac >> 32)
		return int16(c + ns*m>>24), int16(c*m>>24 - ns)
	}
	return int16(c), int16(-ns)
}

func cosFrac32(phase uint32) int32 {
	return int32(uint64(phase<<cosTableBits)*cosFracWide>>32) - cosFracWide/2
}

func putIQ(d *[2]int16, phase uint32) {
	if tier0IQFine && tier0IQWide {
		i := phase >> cosShift
		m := cosFrac32(phase)
		sn, c := cosTable[i], cosTable[i+cosQuarter]
		co := c - int32(int64(sn)*int64(m)>>32)
		si := sn + int32(int64(c)*int64(m)>>32)
		*(*uint32)(unsafe.Pointer(d)) = uint32(si)&0xffff0000 | uint32(co)>>16
		return
	}
	d[0], d[1] = cosSin(phase)
}

const (
	cosFrac     = 25736
	cosFracWide = 26353589 >> (cosTableBits - 10)
)

//go:nobounds
func (m *MCUIQ) steady(dst []int16, word bool) {
	us := m.upShift & 3
	phase, prev := m.phase, m.prev
	j := 0
	for _, w := range &m.buf {
		dw := (w - prev) >> us
		v := prev
		if tier0Exact {
			if us == 0 {
				v += dw>>1 - dw
			} else {
				v -= dw >> 1
			}
		}
		prev = w
		if word {
			for range 1 << us {
				v += dw
				phase += uint32(v)
				putIQ((*[2]int16)(dst[j:j+2]), phase)
				j += 2
			}
			continue
		}
		for range 1 << us {
			v += dw
			phase += uint32(v)
			dst[j], dst[j+1] = cosSin(phase)
			j += 2
		}
	}
	m.phase, m.prev = phase, prev
}

//go:nobounds
func (m *MCUIQ) steady1(dst []int16) {
	phase, prev, fr := m.phase, m.prev, m.frac
	p := 0
	for {
		w := m.buf[p]
		dw := (w - prev) >> 1
		v := prev + dw
		if tier0Exact {
			v -= dw >> 1
		}
		prev = w
		phase += uint32(v)
		i, f := phase>>cosShift, int32(uint64(phase<<cosTableBits)*uint64(fr)>>32)-int32(fr>>1)
		sn, c := cosTable[i], cosTable[i+cosQuarter]
		lo := uint32(sn+int32(int64(c)*int64(f)>>32))&0xffff0000 | uint32(c-int32(int64(sn)*int64(f)>>32))>>16
		phase += uint32(v + dw)
		i, f = phase>>cosShift, int32(uint64(phase<<cosTableBits)*uint64(fr)>>32)-int32(fr>>1)
		sn, c = cosTable[i], cosTable[i+cosQuarter]
		hi := uint32(sn+int32(int64(c)*int64(f)>>32))&0xffff0000 | uint32(c-int32(int64(sn)*int64(f)>>32))>>16
		*(*[2]uint32)(unsafe.Pointer(&dst[4*p])) = [2]uint32{lo, hi}
		p++
		if p == iqPhases {
			break
		}
	}
	m.phase, m.prev = phase, prev
}

func (m *MCUIQ) silent(dst []int16) {
	for len(dst) >= 4 {
		*(*[4]int16)(dst) = [4]int16{}
		dst = dst[4:]
	}
	phase, prev, us := m.phase, m.prev, m.upShift&31
	tri := uint32(1<<us) * (1<<us + 1) / 2
	for _, w := range &m.buf {
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

//go:noinline
func (m *MCUIQ) ramping(dst []int16) {
	us := m.upShift & 31
	phase, prev := m.phase, m.prev
	env, q, r, ramp, sq, sr, on := m.env, m.envQ, m.envR, m.ramp, m.stepQ, m.stepR, m.on
	j := 0
	for _, w := range &m.buf {
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
			rc := cosTable[q&cosMask]
			if tier0IQFine && tier0IQWide {
				rc = cosTable[q&cosMask+cosQuarter] >> 16
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

func (m *MCUIQ) corr5(c uint32) (a0, a1, a2, a3, a4 int32) {
	const mask = 1<<(2*corrIQSyms) - 1
	const span = corrIQSyms + corrIQLen - 1
	z := uint32(0)
	if c == 4 {
		z, c = 1, 0
	}
	codes, zeros := m.codes<<2|c, m.zeros<<1|z
	m.codes, m.zeros = codes, zeros
	if zeros>>(corrIQSkip-corrIQLen+1)&(1<<span-1) != 0 {
		return
	}
	for k := range corrIQLen {
		i := codes >> uint32(2*(corrIQSkip-k)) & mask
		if corrIQSym && i > mask/2 {
			t := (*[5]int32)(corrIQ[k][(i^mask)&uint32(len(corrIQ[0])-1)][:])
			a0 -= t[0]
			a1 -= t[1]
			a2 -= t[2]
			a3 -= t[3]
			a4 -= t[4]
			continue
		}
		t := (*[5]int32)(corrIQ[k][i&uint32(len(corrIQ[0])-1)][:])
		a0 += t[0]
		a1 += t[1]
		a2 += t[2]
		a3 += t[3]
		a4 += t[4]
	}
	return
}
