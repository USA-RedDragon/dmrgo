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
	f *fft
	// win holds the window from its middle out: sample i and fftSize-i
	// take win[min(i, fftSize-i)].
	win []float32
	// hist holds the samples of the frame, and save the second half while
	// the transform runs in hist.
	hist   []complex64
	save   []complex64
	fill   int
	power  []float32
	frames int
	// sorted holds the power of the passband bins in increasing order.
	sorted []float32
	// Hz, CN and OK hold the last estimate: carrier offset, C/N in dB in
	// 12.5 kHz, and whether a signal stands above the noise.
	Hz, CN float64
	OK     bool
	ready  bool
}

// reset drops the frames of the estimate in progress, so that the next
// estimate holds only frames after the reset.
func (s *spectrum) reset() {
	clear(s.power)
	s.frames, s.fill, s.ready = 0, 0, false
}

// newSpectrum returns an estimator with no frames yet.
func newSpectrum() *spectrum {
	s := &spectrum{f: newFFT(fftSize), win: make([]float32, fftSize/2+1), hist: make([]complex64, fftSize),
		save: make([]complex64, fftSize/2), power: make([]float32, fftSize), sorted: make([]float32, 2*passBins+1)}
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
	copy(s.save, s.hist[fftSize/2:])
	for i := range s.hist {
		s.hist[i] *= complex(s.win[min(i, fftSize-i)], 0)
	}
	s.f.transform(s.hist)
	for i, x := range s.hist {
		s.power[i] += real(x)*real(x) + imag(x)*imag(x)
	}
	copy(s.hist, s.save)
	s.fill = fftSize / 2
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
	pass := func(k int) float64 { return float64(s.power[(k+fftSize)%fftSize]) }
	for k := -passBins; k <= passBins; k++ {
		s.sorted[k+passBins] = s.power[(k+fftSize)%fftSize]
	}
	slices.Sort(s.sorted)
	floor := float64(s.sorted[len(s.sorted)/4])
	half := int(channelHz / 2 / bin)
	limit := passBins - half
	var e float64
	for k := -limit - half; k <= -limit+half; k++ {
		e += pass(k) - floor
	}
	best, center := e, -limit
	for c := -limit + 1; c <= limit; c++ {
		e += pass(c+half) - pass(c-half-1)
		if e > best {
			best, center = e, c
		}
	}
	var sum float64
	var n int
	for k := -passBins; k <= passBins; k++ {
		if k < center-half-8 || k > center+half+8 {
			sum += pass(k)
			n++
		}
	}
	if n > 8 {
		floor = sum / float64(n)
	}
	var num, den float64
	for k := center - half; k <= center+half; k++ {
		e := pass(k) - floor
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
