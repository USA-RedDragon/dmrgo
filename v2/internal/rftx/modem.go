// Package rftx builds DMR test transmissions: burst sequences, 4FSK
// modulation of MS bursts and BS outbound streams, and the I/Q of an FM
// transmitter (ETSI TS 102 361-1 clause 10).
package rftx

import (
	"math"

	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

// BasebandRate is the sample rate of the deviation and envelope of the
// modulators, in Hz.
const BasebandRate = basebandRate

// SlotSeconds is the length of a TDMA slot.
const SlotSeconds = slotSeconds

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

// RRCTaps returns a root-raised-cosine filter with alpha 0.2 (ETSI TS 102 361-1
// clause 10.2.2.2) with unity DC gain, so a steady frequency passes unchanged
// and a receiver measures the true deviation.
func RRCTaps() []float64 {
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

// SymbolsFromBits maps bits to symbols using the dibit map of ETSI TS 102
// 361-1 Table 10.3: 01 +3, 00 +1, 10 -1, 11 -3.
func SymbolsFromBits(bits []unpacked.Bit) []int {
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

// SymbolsFromBurst maps the 264 bits of a burst to 132 symbols.
func SymbolsFromBurst(b [33]byte) []int {
	var bits [264]unpacked.Bit
	unpacked.UnpackAt(bits[:], b[:], 0)
	return SymbolsFromBits(bits[:])
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
		syms := SymbolsFromBurst(b)
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
	return convolve(impulses, RRCTaps()), env
}

// BSSlot is one 30 ms slot of a BS outbound stream: the CACH in transmit
// order and the burst that follows it.
type BSSlot struct {
	CACH  [3]byte
	Burst layer2.RawBurst
}

// ModulateBS turns a BS outbound stream into deviation and envelope. The
// carrier is continuous: each 30 ms slot is 144 symbols, 12 of CACH and 132
// of burst, with a ramp before the first slot and after the last.
// (ETSI TS 102 361-1 clause 6.3)
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
		put(t0, SymbolsFromBits(unpacked.Unpack(s.CACH[:])))
		var b [264]unpacked.Bit
		unpacked.UnpackAt(b[:], s.Burst[:], 0)
		put(t0+float64(cachSymbols)/symbolRate, SymbolsFromBits(b[:]))
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
	return convolve(impulses, RRCTaps()), env
}

// PadToFrames extends the deviation and envelope with silence to a whole
// number of 60 ms TDMA frames, so a repeated sequence keeps its slot.
func PadToFrames(freq, env []float64) ([]float64, []float64) {
	frame := int(2 * slotSeconds * basebandRate)
	n := (len(freq) + frame - 1) / frame * frame
	for len(freq) < n {
		freq = append(freq, 0)
		env = append(env, 0)
	}
	return freq, env
}

// ToIQ FM-modulates the deviation signal onto a carrier at offsetHz from the
// tuned frequency and returns signed 8-bit interleaved I/Q (cs8) at rate.
func ToIQ(freq, env []float64, rate, offsetHz, level float64) []byte {
	return MixIQ([]Carrier{{Freq: freq, Env: env, OffsetHz: offsetHz}}, rate, level)
}

// Carrier is one FM carrier of a multi-channel I/Q file: its deviation and
// envelope at the baseband rate and its offset from the tuned frequency.
type Carrier struct {
	Freq, Env []float64
	OffsetHz  float64
}

// MixIQ FM-modulates each carrier, sums them and returns signed 8-bit
// interleaved I/Q at rate, as ToIQ does for one carrier. Each carrier gets
// level divided by the number of carriers, so the sum never clips. The
// output is as long as the longest carrier.
func MixIQ(carriers []Carrier, rate, level float64) []byte {
	out := make([]byte, 0, 2*mixLen(carriers, rate))
	mix(carriers, rate, 127*level/float64(max(len(carriers), 1)), func(re, im float64) {
		out = append(out, s8(re), s8(im))
	})
	return out
}

// Mix FM-modulates each carrier, sums them and calls emit with each complex
// sample at rate, in units where 1 is full scale, as MixIQ does.
func Mix(carriers []Carrier, rate, level float64, emit func(re, im float64)) {
	mix(carriers, rate, level/float64(max(len(carriers), 1)), emit)
}

func mixLen(carriers []Carrier, rate float64) int {
	var longest int
	for _, c := range carriers {
		longest = max(longest, len(c.Freq))
	}
	return int(float64(longest) * rate / basebandRate)
}

func mix(carriers []Carrier, rate, scale float64, emit func(re, im float64)) {
	n := mixLen(carriers, rate)
	phase := make([]float64, len(carriers))
	for i := range n {
		pos := float64(i) * basebandRate / rate
		k := int(pos)
		frac := pos - float64(k)
		var re, im float64
		for j, c := range carriers {
			f, a := 0.0, 0.0
			if k < len(c.Freq) {
				f, a = c.Freq[k], c.Env[k]
				if k+1 < len(c.Freq) {
					f += (c.Freq[k+1] - c.Freq[k]) * frac
					a += (c.Env[k+1] - c.Env[k]) * frac
				}
			}
			phase[j] += 2 * math.Pi * (f + c.OffsetHz) / rate
			phase[j] = math.Mod(phase[j], 2*math.Pi)
			amp := scale * a
			re += amp * math.Cos(phase[j])
			im += amp * math.Sin(phase[j])
		}
		emit(re, im)
	}
}

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

// CW returns an unmodulated carrier at offsetHz as signed 8-bit I/Q (cs8).
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

// FromCU8 converts unsigned 8-bit interleaved I/Q (cu8) to complex samples.
func FromCU8(b []byte) []complex128 {
	out := make([]complex128, len(b)/2)
	for i := range out {
		out[i] = complex((float64(b[2*i])-127.5)/127.5, (float64(b[2*i+1])-127.5)/127.5)
	}
	return out
}

// FromCS8 converts signed 8-bit interleaved I/Q (cs8) to complex samples.
func FromCS8(b []byte) []complex128 {
	out := make([]complex128, len(b)/2)
	for i := range out {
		out[i] = complex(fromS8(b[2*i])/127, fromS8(b[2*i+1])/127)
	}
	return out
}

// Baseband FM-modulates the deviation and envelope of a modulator into
// complex baseband at BasebandRate, with the carrier offsetHz from 0 Hz.
func Baseband(freq, env []float64, offsetHz float64) []complex64 {
	x := make([]complex64, len(freq))
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (freq[i] + offsetHz) / basebandRate
		x[i] = complex(float32(env[i]*math.Cos(ph)), float32(env[i]*math.Sin(ph)))
	}
	return x
}
