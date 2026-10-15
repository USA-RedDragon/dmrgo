package main

import (
	"math"
	"math/cmplx"

	"github.com/USA-RedDragon/dmrgo/v2/bit"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const (
	symbolRate   = 4800
	deviationHz  = 648.0
	basebandRate = 48000
	samplesPerSy = basebandRate / symbolRate
	rrcAlpha     = 0.2
	rrcSpan      = 8
	burstSymbols = 132
	cachSymbols  = 12
	frameSamples = 2 * slotSeconds * basebandRate
	slotSeconds  = 0.030
	burstSeconds = 0.0275
)

// rrcTaps returns a root-raised-cosine filter with alpha 0.2 (ETSI TS 102 361-1
// clause 10.2.2.2) with unity DC gain, so a steady frequency passes unchanged
// and a receiver measures the true deviation.
func rrcTaps() []float64 {
	n := 2*rrcSpan*samplesPerSy + 1
	h := make([]float64, n)
	for i := range h {
		t := float64(i-n/2) / samplesPerSy
		switch {
		case t == 0:
			h[i] = 1 - rrcAlpha + 4*rrcAlpha/math.Pi
		case math.Abs(math.Abs(4*rrcAlpha*t)-1) < 1e-9:
			h[i] = rrcAlpha / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*rrcAlpha)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*rrcAlpha)))
		default:
			h[i] = (math.Sin(math.Pi*t*(1-rrcAlpha)) + 4*rrcAlpha*t*math.Cos(math.Pi*t*(1+rrcAlpha))) /
				(math.Pi * t * (1 - (4*rrcAlpha*t)*(4*rrcAlpha*t)))
		}
	}
	var c float64
	for i := range h {
		c += h[i]
	}
	for i := range h {
		h[i] /= c
	}
	return h
}

func convolve(x, h []float64) []float64 {
	out := make([]float64, len(x))
	half := len(h) / 2
	for i := range x {
		if x[i] == 0 {
			continue
		}
		for j, v := range h {
			k := i + j - half
			if k >= 0 && k < len(out) {
				out[k] += x[i] * v
			}
		}
	}
	return out
}

func filter(x, h []float64) []float64 {
	out := make([]float64, len(x))
	half := len(h) / 2
	for i := range out {
		var s float64
		for j, v := range h {
			k := i - j + half
			if k >= 0 && k < len(x) {
				s += x[k] * v
			}
		}
		out[i] = s
	}
	return out
}

// symbolsFromBits maps bits to symbols using the dibit map of ETSI TS 102
// 361-1 Table 10.3: 01 +3, 00 +1, 10 -1, 11 -3.
func symbolsFromBits(bits []bit.Bit) []int {
	s := make([]int, len(bits)/2)
	for i := range s {
		switch bits[2*i]<<1 | bits[2*i+1] {
		case 0b01:
			s[i] = 3
		case 0b00:
			s[i] = 1
		case 0b10:
			s[i] = -1
		default:
			s[i] = -3
		}
	}
	return s
}

// symbolsFromBurst maps the 264 bits of a burst to 132 symbols.
func symbolsFromBurst(b [33]byte) []int {
	bits := bit.Unpack264(b)
	return symbolsFromBits(bits[:])
}

func dibitFromSymbol(v float64) (bit.Bit, bit.Bit) {
	switch {
	case v >= 2*deviationHz:
		return 0, 1
	case v >= 0:
		return 0, 0
	case v >= -2*deviationHz:
		return 1, 0
	default:
		return 1, 1
	}
}

// Schedule places bursts on the air: slot is 0 or 1 for TDMA (one burst per
// 60 ms frame) and -1 for continuous back-to-back slots.
type Schedule struct {
	Slot int
}

func (s Schedule) start(i int) float64 {
	if s.Slot < 0 {
		return float64(i) * slotSeconds
	}
	return float64(2*i+s.Slot) * slotSeconds
}

// Modulate turns bursts into the frequency deviation in Hz at the baseband
// rate and the matching amplitude envelope (0 off, 1 full power).
func Modulate(bursts []layer2.RawBurst, sched Schedule) (freq, env []float64) {
	if len(bursts) == 0 {
		return nil, nil
	}
	total := sched.start(len(bursts)-1) + slotSeconds
	n := int(math.Ceil(total*basebandRate)) + 1
	impulses := make([]float64, n)
	env = make([]float64, n)
	guard := (slotSeconds - burstSeconds) / 2
	ramp := guard
	for i, b := range bursts {
		t0 := sched.start(i) + guard
		syms := symbolsFromBurst(b)
		for k, s := range syms {
			idx := int(math.Round((t0 + float64(k)/symbolRate) * basebandRate))
			impulses[idx] = float64(s) * deviationHz * samplesPerSy
		}
		on := int(math.Round((t0 - ramp) * basebandRate))
		off := int(math.Round((t0 + burstSeconds + ramp) * basebandRate))
		rn := int(math.Round(ramp * basebandRate))
		for j := max(on, 0); j < min(off, n); j++ {
			a := 1.0
			if j-on < rn {
				a = 0.5 - 0.5*math.Cos(math.Pi*float64(j-on)/float64(rn))
			} else if off-j < rn {
				a = 0.5 - 0.5*math.Cos(math.Pi*float64(off-j)/float64(rn))
			}
			env[j] = math.Max(env[j], a)
		}
	}
	return convolve(impulses, rrcTaps()), env
}

// BSSlot is one 30 ms slot of a BS outbound stream: the CACH in transmit
// order and the burst that follows it.
type BSSlot struct {
	CACH  [layer2.CACHBits]bit.Bit
	Burst [33]byte
}

// ModulateBS turns a BS outbound stream into deviation and envelope. The
// carrier is continuous: each 30 ms slot is 144 symbols, 12 of CACH and 132
// of burst (ETSI TS 102 361-1 clause 6.3), with a ramp before the first slot
// and after the last.
func ModulateBS(slots []BSSlot) (freq, env []float64) {
	if len(slots) == 0 {
		return nil, nil
	}
	ramp := (slotSeconds - burstSeconds) / 2
	total := 2*ramp + float64(len(slots))*slotSeconds
	n := int(math.Ceil(total*basebandRate)) + 1
	impulses := make([]float64, n)
	put := func(t0 float64, syms []int) {
		for k, s := range syms {
			idx := int(math.Round((t0 + float64(k)/symbolRate) * basebandRate))
			impulses[idx] = float64(s) * deviationHz * samplesPerSy
		}
	}
	for i, s := range slots {
		t0 := ramp + float64(i)*slotSeconds
		put(t0, symbolsFromBits(s.CACH[:]))
		b := bit.Unpack264(s.Burst)
		put(t0+float64(cachSymbols)/symbolRate, symbolsFromBits(b[:]))
	}
	env = make([]float64, n)
	rn := ramp * basebandRate
	end := total * basebandRate
	for j := range env {
		x := float64(j)
		switch {
		case x < rn:
			env[j] = 0.5 - 0.5*math.Cos(math.Pi*x/rn)
		case x > end-rn:
			env[j] = 0.5 - 0.5*math.Cos(math.Pi*math.Max(end-x, 0)/rn)
		default:
			env[j] = 1
		}
	}
	return convolve(impulses, rrcTaps()), env
}

// padToFrames extends the deviation and envelope with silence to a whole
// number of 60 ms TDMA frames, so a repeated sequence keeps its slot.
func padToFrames(freq, env []float64) ([]float64, []float64) {
	frame := int(2 * slotSeconds * basebandRate)
	n := (len(freq) + frame - 1) / frame * frame
	for len(freq) < n {
		freq = append(freq, 0)
		env = append(env, 0)
	}
	return freq, env
}

// ToIQ FM-modulates the deviation signal onto a carrier at offsetHz from the
// tuned frequency and returns signed 8-bit interleaved I/Q at rate, the format
// hackrf_transfer -t reads.
func ToIQ(freq, env []float64, rate, offsetHz, level float64) []byte {
	n := int(float64(len(freq)) * rate / basebandRate)
	out := make([]byte, 2*n)
	var phase float64
	for i := range n {
		pos := float64(i) * basebandRate / rate
		k := int(pos)
		frac := pos - float64(k)
		f, a := freq[k], env[k]
		if k+1 < len(freq) {
			f += (freq[k+1] - freq[k]) * frac
			a += (env[k+1] - env[k]) * frac
		}
		phase += 2 * math.Pi * (f + offsetHz) / rate
		phase = math.Mod(phase, 2*math.Pi)
		amp := 127 * level * a
		out[2*i] = s8(amp * math.Cos(phase))
		out[2*i+1] = s8(amp * math.Sin(phase))
	}
	return out
}

// s8 returns f, clamped to [-127, 127], as a two's complement octet.
func s8(f float64) byte {
	v := int(math.Round(math.Max(-127, math.Min(127, f))))
	if v < 0 {
		v += 256
	}
	if v < 0 || v > 0xFF {
		return 0
	}
	return byte(v)
}

func fromS8(b byte) float64 {
	v := int(b)
	if v > 127 {
		v -= 256
	}
	return float64(v)
}

// CW returns an unmodulated carrier at offsetHz as signed 8-bit I/Q.
func CW(seconds, rate, offsetHz, level float64) []byte {
	n := int(seconds * rate)
	out := make([]byte, 2*n)
	for i := range n {
		p := 2 * math.Pi * offsetHz * float64(i) / rate
		out[2*i] = s8(127 * level * math.Cos(p))
		out[2*i+1] = s8(127 * level * math.Sin(p))
	}
	return out
}

// FromRTL converts rtl_sdr unsigned 8-bit interleaved I/Q to complex samples.
func FromRTL(b []byte) []complex128 {
	out := make([]complex128, len(b)/2)
	for i := range out {
		out[i] = complex((float64(b[2*i])-127.5)/127.5, (float64(b[2*i+1])-127.5)/127.5)
	}
	return out
}

// FromHackRF converts signed 8-bit interleaved I/Q to complex samples.
func FromHackRF(b []byte) []complex128 {
	out := make([]complex128, len(b)/2)
	for i := range out {
		out[i] = complex(fromS8(b[2*i])/127, fromS8(b[2*i+1])/127)
	}
	return out
}

// channelize removes the receiver's DC offset and resamples x from rate to the
// baseband rate. A boxcar to four times the baseband rate folds only noise
// from near its nulls into the channel; a FIR low-pass then removes the rest
// of the out-of-channel noise before the final decimation.
func channelize(x []complex128, rate float64) []complex128 {
	var dc complex128
	for _, v := range x {
		dc += v
	}
	if len(x) > 0 {
		dc /= complex(float64(len(x)), 0)
	}
	y := make([]complex128, len(x))
	for i, v := range x {
		y[i] = v - dc
	}
	const over = 4
	mid := boxcar(y, rate, over*basebandRate)
	f := firLowpass(mid, 8000.0/(over*basebandRate), 127)
	out := make([]complex128, len(f)/over)
	for i := range out {
		out[i] = f[i*over]
	}
	return out
}

// decimate low-pass filters with a boxcar and resamples x from rate to the
// baseband rate.
func decimate(x []complex128, rate float64) []complex128 {
	return boxcar(x, rate, basebandRate)
}

func boxcar(x []complex128, rate, to float64) []complex128 {
	ratio := rate / to
	n := int(float64(len(x)) / ratio)
	out := make([]complex128, n)
	for i := range n {
		lo := int(float64(i) * ratio)
		hi := min(int(float64(i+1)*ratio), len(x))
		var s complex128
		for _, v := range x[lo:hi] {
			s += v
		}
		out[i] = s / complex(float64(max(hi-lo, 1)), 0)
	}
	return out
}

func lowpass(x []complex128, cutoff float64) []complex128 {
	return firLowpass(x, cutoff/basebandRate, 63)
}

// firLowpass applies a Hamming-windowed sinc low-pass with cutoff fc as a
// fraction of the sample rate.
func firLowpass(x []complex128, fc float64, taps int) []complex128 {
	h := make([]float64, taps)
	var sum float64
	for i := range h {
		m := float64(i - taps/2)
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(taps-1))
		if m == 0 {
			h[i] = 2 * fc
		} else {
			h[i] = math.Sin(2*math.Pi*fc*m) / (math.Pi * m)
		}
		h[i] *= w
		sum += h[i]
	}
	out := make([]complex128, len(x))
	for i := range x {
		var s complex128
		for j, v := range h {
			k := i - j + taps/2
			if k >= 0 && k < len(x) {
				s += x[k] * complex(v/sum, 0)
			}
		}
		out[i] = s
	}
	return out
}

func discriminate(x []complex128) []float64 {
	out := make([]float64, len(x))
	for i := 1; i < len(x); i++ {
		out[i] = cmplx.Phase(x[i]*cmplx.Conj(x[i-1])) * basebandRate / (2 * math.Pi)
	}
	return out
}

func shift(x []complex128, hz float64) []complex128 {
	out := make([]complex128, len(x))
	for i, v := range x {
		out[i] = v * cmplx.Exp(complex(0, -2*math.Pi*hz*float64(i)/basebandRate))
	}
	return out
}

func power(x []complex128) []float64 {
	p := make([]float64, len(x))
	for i, v := range x {
		p[i] = real(v)*real(v) + imag(v)*imag(v)
	}
	return p
}

// carrierOffset estimates the carrier frequency from the mean discriminator
// output over the samples where the signal is present.
func carrierOffset(x []complex128) float64 {
	p := power(x)
	var peak float64
	for _, v := range p {
		peak = math.Max(peak, v)
	}
	d := discriminate(x)
	var s float64
	var n int
	for i := 1; i < len(d); i++ {
		if p[i] > peak/4 && p[i-1] > peak/4 {
			s += d[i]
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return s / float64(n)
}
