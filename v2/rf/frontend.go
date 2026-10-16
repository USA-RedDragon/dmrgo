package rf

import "math"

// Limits of the front end.
const (
	// cicOrder is the order of the integer decimator.
	cicOrder = 3
	// resamplePhases is the number of polyphase branches of the resampler.
	resamplePhases = 256
	// resampleTaps is the length of each polyphase branch.
	resampleTaps = 72
	// wideHz is the edge of the passband of the front end.
	wideHz = 17000.0
	// dcSeconds is the time constant of the DC offset removal.
	dcSeconds = 0.05
	// dcStartSeconds is the shortest time that the DC offset estimate
	// averages over at the start of a stream, so that it does not take a
	// carrier near 0 Hz for the offset.
	dcStartSeconds = 0.01
)

// frontEnd turns interleaved 8-bit I/Q at the input rate into complex
// baseband at basebandRate, flat to wideHz: a removal of the receiver's DC
// offset, a CIC decimator by an integer factor to at least four times the
// baseband rate, and a polyphase resampler to the baseband rate, which
// input at basebandRate skips. It keeps its state between calls and
// allocates only in newFrontEnd.
type frontEnd struct {
	signed  bool
	dec     int
	gain    float32
	integ   [cicOrder][2]int64
	comb    [cicOrder][2]int64
	phase   int
	coef    []float32
	hist    []complex64
	head    int
	t       float64
	step    float64
	n       int64
	dc      complex64
	dcGain  float32
	dcStart float32
}

// newFrontEnd returns a front end for input at rate, signed (HackRF) or
// unsigned (rtl_sdr) 8-bit I/Q.
func newFrontEnd(rate float64, signed bool) *frontEnd {
	dec := max(int(rate/(4*basebandRate)), 1)
	mid := rate / float64(dec)
	f := &frontEnd{signed: signed, dec: dec, step: mid / basebandRate}
	f.gain = float32(1 / (255 * math.Pow(float64(dec), cicOrder)))
	f.dcGain = float32(1 / (dcSeconds * mid))
	f.dcStart = float32(dcStartSeconds * mid)
	if f.step == 1 {
		return f
	}
	f.hist = make([]complex64, 2*resampleTaps)
	f.coef = make([]float32, resamplePhases*resampleTaps)
	fc := (wideHz + (basebandRate - wideHz)) / 2 / mid
	const beta = 8.0
	for p := range resamplePhases {
		for j := range resampleTaps {
			u := float64(resampleTaps/2-1-j) + float64(p)/resamplePhases
			v := 2 * fc
			if u != 0 {
				v = math.Sin(2*math.Pi*fc*u) / (math.Pi * u)
			}
			r := u / (resampleTaps / 2)
			v *= bessel0(beta*math.Sqrt(max(0, 1-r*r))) / bessel0(beta)
			f.coef[p*resampleTaps+j] = float32(v)
		}
	}
	return f
}

// bessel0 returns the modified Bessel function of the first kind of order 0.
func bessel0(x float64) float64 {
	s, t := 1.0, 1.0
	for k := 1; k < 50; k++ {
		t *= (x / (2 * float64(k))) * (x / (2 * float64(k)))
		s += t
		if t < 1e-12*s {
			break
		}
	}
	return s
}

// process converts the I/Q pairs of in and appends the baseband samples to
// out, which must have room for len(in)/2/(rate/basebandRate)+2 samples, and
// returns the number written.
func (f *frontEnd) process(in []byte, out []complex64) int {
	w := 0
	for i := 0; i+1 < len(in); i += 2 {
		var re, im int64
		if f.signed {
			re, im = 2*signed8(in[i]), 2*signed8(in[i+1])
		} else {
			re, im = 2*int64(in[i])-255, 2*int64(in[i+1])-255
		}
		for s := range cicOrder {
			f.integ[s][0] += re
			f.integ[s][1] += im
			re, im = f.integ[s][0], f.integ[s][1]
		}
		f.phase++
		if f.phase < f.dec {
			continue
		}
		f.phase = 0
		for s := range cicOrder {
			re, im, f.comb[s][0], f.comb[s][1] = re-f.comb[s][0], im-f.comb[s][1], re, im
		}
		v := complex(float32(re)*f.gain, float32(im)*f.gain)
		g := max(f.dcGain, 1/(float32(f.n)+f.dcStart))
		f.dc += (v - f.dc) * complex(g, 0)
		v -= f.dc
		if f.coef == nil {
			out[w] = v
			w++
			f.n++
			continue
		}
		f.hist[f.head] = v
		f.hist[f.head+resampleTaps] = v
		f.head = (f.head + 1) % resampleTaps
		if f.n >= int64(math.Floor(f.t))+resampleTaps/2 {
			frac := f.t - math.Floor(f.t)
			h := f.coef[int(frac*resamplePhases)*resampleTaps:][:resampleTaps]
			x := f.hist[f.head : f.head+resampleTaps]
			var acc complex64
			for k, c := range h {
				acc += x[k] * complex(c, 0)
			}
			out[w] = acc
			w++
			f.t += f.step
		}
		f.n++
	}
	return w
}

// signed8 returns the two's complement value of octet b.
func signed8(b byte) int64 {
	v := int64(b)
	if v > 127 {
		v -= 256
	}
	return v
}
