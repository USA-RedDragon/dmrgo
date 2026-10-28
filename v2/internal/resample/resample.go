// Package resample changes the sample rate of a stream of complex or real
// samples, up or down, by any ratio. Each output is the dot product of the
// last taps inputs with one phase of a polyphase Kaiser-windowed sinc low
// pass: the phase just before the position of the output, or with Lerp a
// blend of the two phases around it, so that a table of few phases serves.
package resample

import "math"

// Design fills h with len(h)/taps rows of taps coefficients of a low pass
// with its cutoff at fc cycles per input sample and a Kaiser window of
// beta. Row p delays the output by p/phases of an input sample, so a table
// for Lerp has phases+1 rows. It does not allocate.
func Design(h []float32, taps, phases int, fc, beta float64) {
	if taps <= 0 || phases <= 0 {
		return
	}
	for p := range len(h) / taps {
		for j := range taps {
			u := float64(taps/2-1-j) + float64(p)/float64(phases)
			v := 2 * fc
			if u != 0 {
				v = math.Sin(2*math.Pi*fc*u) / (math.Pi * u)
			}
			r := u / float64(taps/2)
			v *= Bessel0(beta*math.Sqrt(max(0, 1-r*r))) / Bessel0(beta)
			h[p*taps+j] = float32(v)
		}
	}
}

// Bessel0 returns the modified Bessel function of the first kind of order 0
// at x, for Kaiser windows.
func Bessel0(x float64) float64 {
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

// Filter is the table and timing of a resampler: Taps coefficients per
// phase, Phases phases, and Step input samples per output.
type Filter struct {
	h      []float32
	taps   int
	phases int
	lerp   bool
	t      float64
	step   float64
	n      int64
}

// Init sets f up with the table h of Design, of phases rows, or phases+1
// rows with lerp, and step input samples per output: above 1 to decimate,
// below 1 to interpolate. It returns false, with f unchanged, when h does
// not hold the rows or step is not positive.
func (f *Filter) Init(h []float32, taps, phases int, lerp bool, step float64) bool {
	rows := phases
	if lerp {
		rows++
	}
	if taps <= 0 || phases <= 0 || len(h) < rows*taps || !(step > 0) || math.IsInf(step, 1) {
		return false
	}
	*f = Filter{h: h, taps: taps, phases: phases, lerp: lerp, step: step}
	return true
}

// MaxOut returns the most outputs that one input can give.
func (f *Filter) MaxOut() int {
	if f.step <= 0 {
		return 0
	}
	return int(math.Ceil(1/f.step)) + 1
}

func (f *Filter) due() bool {
	return f.taps > 0 && f.n >= int64(math.Floor(f.t))+int64(f.taps/2)
}

func (f *Filter) phase() (h0, h1 []float32, w float32) {
	frac := f.t - math.Floor(f.t)
	if !f.lerp {
		return f.h[int(frac*float64(f.phases))*f.taps:][:f.taps], nil, 0
	}
	q := frac * float64(f.phases)
	i := min(int(q), f.phases-1)
	return f.h[i*f.taps:][:f.taps], f.h[(i+1)*f.taps:][:f.taps], float32(q - float64(i))
}

// Complex resamples complex samples.
type Complex struct {
	Filter
	hist []complex64
	head int
}

// Init sets r up as Filter.Init does, with hist for its history, which
// must hold 2*taps samples.
func (r *Complex) Init(h []float32, taps, phases int, lerp bool, step float64, hist []complex64) bool {
	if len(hist) < 2*taps || !r.Filter.Init(h, taps, phases, lerp, step) {
		return false
	}
	r.hist, r.head = hist[:2*taps], 0
	clear(r.hist)
	return true
}

// Push takes the input v, writes the outputs that it completes to out and
// returns their number, at most len(out).
func (r *Complex) Push(v complex64, out []complex64) int {
	t := r.taps
	if t == 0 {
		return 0
	}
	r.hist[r.head] = v
	r.hist[r.head+t] = v
	r.head = (r.head + 1) % t
	w := 0
	for w < len(out) && r.due() {
		x := r.hist[r.head : r.head+t]
		h0, h1, k := r.phase()
		var acc complex64
		if h1 == nil {
			for j, c := range h0 {
				acc += x[j] * complex(c, 0)
			}
		} else {
			for j, c := range h0 {
				acc += x[j] * complex(c+k*(h1[j]-c), 0)
			}
		}
		out[w] = acc
		w++
		r.t += r.step
	}
	r.n++
	return w
}

// Real resamples real samples of 16 bits.
type Real struct {
	Filter
	hist []int16
	head int
}

// Init sets r up as Filter.Init does, with hist for its history, which
// must hold taps samples.
func (r *Real) Init(h []float32, taps, phases int, lerp bool, step float64, hist []int16) bool {
	if len(hist) < taps || !r.Filter.Init(h, taps, phases, lerp, step) {
		return false
	}
	r.hist, r.head = hist[:taps], 0
	clear(r.hist)
	return true
}

// Push takes the input v, writes the outputs that it completes to out,
// rounded and saturated to 16 bits, and returns their number, at most
// len(out).
func (r *Real) Push(v int16, out []int16) int {
	t := r.taps
	if t == 0 {
		return 0
	}
	r.hist[r.head] = v
	r.head++
	if r.head == t {
		r.head = 0
	}
	w := 0
	for w < len(out) && r.due() {
		h0, h1, k := r.phase()
		var acc float32
		j := 0
		for _, s := range [2][]int16{r.hist[r.head:], r.hist[:r.head]} {
			for _, x := range s {
				c := h0[j]
				if h1 != nil {
					c += k * (h1[j] - c)
				}
				acc += float32(x) * c
				j++
			}
		}
		out[w] = int16(max(min(acc+0.5*sign(acc), 32767), -32768))
		w++
		r.t += r.step
	}
	r.n++
	return w
}

func sign(v float32) float32 {
	if v < 0 {
		return -1
	}
	return 1
}
