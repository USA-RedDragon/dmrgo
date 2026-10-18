package rf

import (
	"math"
	"math/bits"
	"slices"
)

// Limits of the spectrum estimator.
const (
	// fftSize is the length of the transform of the spectrum estimator, 2
	// to the power fftBits.
	fftSize = 1 << fftBits
	fftBits = 8
	// specFrames is the number of half-overlapped frames in one estimate.
	specFrames = 96
	// specBlocks is the most estimates of specFrames frames, the newest,
	// that a longer estimate sums when the newest alone finds no signal: a
	// weak signal that is on part of the time, as an MS in direct mode,
	// stands above the noise of one estimate only at a few dB C/N. It
	// tries the newest 2, 4 and so on, so that a signal that starts is not
	// summed with the noise before it.
	specBlocks = 8
	// channelHz is the occupied bandwidth that the estimator looks for.
	channelHz = 12500.0
)

// fftTransform replaces x, fftSize samples, by its discrete Fourier
// transform, in place by radix 2 with the twiddles fftTwiddles,
// e^(-2*pi*j*i/fftSize) at index i.
func fftTransform(x []complex64) {
	for i := range fftSize {
		if r := int(bits.Reverse(uint(i)) >> (bits.UintSize - fftBits)); i < r {
			x[i], x[r] = x[r], x[i]
		}
	}
	for size := 2; size <= fftSize; size <<= 1 {
		half, step := size/2, fftSize/size
		for start := 0; start < fftSize; start += size {
			for k := range half {
				t := fftTwiddles[k*step] * x[start+k+half]
				x[start+k+half] = x[start+k] - t
				x[start+k] += t
			}
		}
	}
}

// spectrum estimates the carrier frequency and the C/N of the strongest
// channel-wide signal in the wide baseband from a Welch power spectrum.
// specWindow holds the window from its middle out: sample i and fftSize-i
// take specWindow[min(i, fftSize-i)].
type spectrum struct {
	// rate is the sample rate, and pass the number of bins on each side of
	// 0 Hz in the passband of the front end.
	rate float64
	pass int
	// hist holds the samples of the frame, and save the second half while
	// the transform runs in hist.
	hist   []complex64
	save   []complex64
	fill   int
	power  []float32
	frames int
	// blocks holds the power of the last nblocks estimates of specFrames
	// frames, the newest at blocks[head-1].
	blocks  [specBlocks][]float32
	head    int
	nblocks int
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
	s.nblocks = 0
}

// newSpectrum returns an estimator of samples at rate with no frames yet.
func newSpectrum(rate float64) *spectrum {
	pass := int(wideHz*rate/basebandRate) * fftSize / int(rate)
	s := &spectrum{rate: rate, pass: pass, hist: make([]complex64, fftSize), save: make([]complex64, fftSize/2),
		power: make([]float32, fftSize), sorted: make([]float32, 2*pass+1)}
	for i := range s.blocks {
		s.blocks[i] = make([]float32, fftSize)
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
		s.hist[i] *= complex(specWindow[min(i, fftSize-i)], 0)
	}
	fftTransform(s.hist)

	for i, x := range s.hist {
		s.power[i] += real(x)*real(x) + imag(x)*imag(x)
	}
	copy(s.hist, s.save)
	s.fill = fftSize / 2
	s.frames++
	switch {
	case s.frames == specFrames:
		s.estimateBlocks()
		clear(s.power)
		s.frames = 0
		return true
	case !s.ready && s.frames%(specFrames/8) == 0:
		s.estimate()
		return true
	}
	return false
}

// estimateBlocks keeps the power of the estimate of specFrames frames that
// just ended and estimates from it, and while no signal shows, from the
// sum of it and the blocks before it, 2, 4 and so on up to specBlocks.
func (s *spectrum) estimateBlocks() {
	copy(s.blocks[s.head], s.power)
	s.head = (s.head + 1) % specBlocks
	s.nblocks = min(s.nblocks+1, specBlocks)
	s.estimate()
	for w := 2; w <= s.nblocks && !s.OK; w *= 2 {
		clear(s.power)
		for i := range w {
			for k, v := range s.blocks[(s.head-1-i+specBlocks)%specBlocks] {
				s.power[k] += v
			}
		}
		s.frames = w * specFrames
		s.estimate()
	}
	s.ready = true
}

// estimate finds the channel-wide span with the most power above a low
// percentile of the passband bins, then the noise floor as the mean of the
// passband bins away from that span, and the centroid of the excess.
func (s *spectrum) estimate() {
	bin := s.rate / fftSize
	passBins := s.pass
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
