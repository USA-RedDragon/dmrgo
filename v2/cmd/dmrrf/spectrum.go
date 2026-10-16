package main

import (
	"math"
	"math/bits"
	"slices"
)

// Limits of the spectrum estimator.
const (
	// fftSize is the length of the transform of the spectrum estimator.
	fftSize = 256
	// specFrames is the number of half-overlapped frames in one estimate.
	specFrames = 96
	// channelHz is the occupied bandwidth that the estimator looks for.
	channelHz = 12500.0
	// passBins is the number of bins on each side of 0 Hz in the passband
	// of the front end.
	passBins = int(wideHz) * fftSize / basebandRate
)

// fft is an in-place radix-2 FFT of a fixed power-of-two size.
type fft struct {
	n     int
	shift int
	tw    []complex64
}

// newFFT returns a transform of n points.
func newFFT(n int) *fft {
	f := &fft{n: n, shift: bits.UintSize - bits.Len(uint(n-1)), tw: make([]complex64, n/2)}
	for i := range f.tw {
		p := -2 * math.Pi * float64(i) / float64(n)
		f.tw[i] = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	return f
}

// transform replaces x by its discrete Fourier transform.
func (f *fft) transform(x []complex64) {
	for i := range f.n {
		if r := int(bits.Reverse(uint(i)) >> f.shift); i < r {
			x[i], x[r] = x[r], x[i]
		}
	}
	for size := 2; size <= f.n; size <<= 1 {
		half, step := size/2, f.n/size
		for start := 0; start < f.n; start += size {
			for k := range half {
				t := f.tw[k*step] * x[start+k+half]
				x[start+k+half] = x[start+k] - t
				x[start+k] += t
			}
		}
	}
}

// spectrum estimates the carrier frequency and the C/N of the strongest
// channel-wide signal in the wide baseband from a Welch power spectrum.
type spectrum struct {
	f      *fft
	win    []float32
	frame  []complex64
	hist   []complex64
	fill   int
	power  []float64
	frames int
	sorted []float64
	// Hz, CN and OK hold the last estimate: carrier offset, C/N in dB in
	// 12.5 kHz, and whether a signal stands above the noise.
	Hz, CN float64
	OK     bool
	ready  bool
}

// newSpectrum returns an estimator with no frames yet.
func newSpectrum() *spectrum {
	s := &spectrum{f: newFFT(fftSize), win: make([]float32, fftSize), frame: make([]complex64, fftSize),
		hist: make([]complex64, fftSize), power: make([]float64, fftSize), sorted: make([]float64, 2*passBins+1)}
	for i := range s.win {
		s.win[i] = float32(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/fftSize))
	}
	return s
}

// push adds one wide baseband sample and reports whether a new estimate is
// ready.
func (s *spectrum) push(v complex64) bool {
	s.hist[s.fill] = v
	s.fill++
	if s.fill < fftSize {
		return false
	}
	for i := range s.frame {
		s.frame[i] = s.hist[i] * complex(s.win[i], 0)
	}
	copy(s.hist, s.hist[fftSize/2:])
	s.fill = fftSize / 2
	s.f.transform(s.frame)
	for i, x := range s.frame {
		s.power[i] += float64(real(x)*real(x) + imag(x)*imag(x))
	}
	s.frames++
	switch {
	case s.frames == specFrames:
		s.estimate()
		clear(s.power)
		s.frames = 0
		return true
	case !s.ready && s.frames%(specFrames/8) == 0:
		s.estimate()
		return true
	}
	return false
}

// estimate finds the channel-wide span with the most power above a low
// percentile of the passband bins, then the noise floor as the mean of the
// passband bins away from that span, and the centroid of the excess.
func (s *spectrum) estimate() {
	bin := float64(basebandRate) / fftSize
	pass := int(wideHz / bin)
	m := 0
	for k := -pass; k <= pass; k++ {
		s.sorted[m] = s.power[(k+fftSize)%fftSize]
		m++
	}
	slices.Sort(s.sorted[:m])
	floor := s.sorted[m/4]
	half := int(channelHz / 2 / bin)
	limit := pass - half
	best, center := math.Inf(-1), 0
	for c := -limit; c <= limit; c++ {
		var e float64
		for k := c - half; k <= c+half; k++ {
			e += s.power[(k+fftSize)%fftSize] - floor
		}
		if e > best {
			best, center = e, c
		}
	}
	var sum float64
	var n int
	for k := -pass; k <= pass; k++ {
		if k < center-half-8 || k > center+half+8 {
			sum += s.power[(k+fftSize)%fftSize]
			n++
		}
	}
	if n > 8 {
		floor = sum / float64(n)
	}
	var num, den float64
	for k := center - half; k <= center+half; k++ {
		e := s.power[(k+fftSize)%fftSize] - floor
		num += e * float64(k)
		den += e
	}
	noise := floor * float64(2*half+1)
	s.ready = s.frames == specFrames
	s.OK = den > 6*noise/math.Sqrt(float64(s.frames))
	if den > 0 {
		s.Hz = num / den * bin
		s.CN = 10 * math.Log10(den/noise)
	}
}
