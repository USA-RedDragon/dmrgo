package rf

import (
	"math"
	"math/bits"
	"slices"
)

const (
	fftSize      = 1 << fftBits
	fftBits      = 8
	specFrames   = 96
	specBlocks   = 8
	specQuick    = 3
	specFloorQ   = 30
	specCutLevel = 0.1
	specRise     = 2
	specRearm    = 1.5
	specPowerN   = 48
	specFloorN   = 4800
	specSettle   = 480
	channelHz    = 12500.0
)

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

type spectrum struct {
	rate    float64
	pass    int
	hist    []complex64
	save    []complex64
	fill    int
	power   []float32
	frames  int
	blocks  [specBlocks][]float32
	head    int
	nblocks int
	sorted  []float32
	Hz, CN  float64
	OK      bool
	ready   bool
	pw      float32
	floor   float32
	quick   int
	armed   bool
	settled int
	fast    bool
}

func (s *spectrum) reset() {
	clear(s.power)
	s.frames, s.fill, s.ready = 0, 0, false
	s.nblocks = 0
}

func newSpectrum(rate float64) *spectrum {
	pass := int(wideHz*rate/basebandRate) * fftSize / int(rate)
	s := &spectrum{rate: rate, pass: pass, hist: make([]complex64, fftSize), save: make([]complex64, fftSize/2),
		power: make([]float32, fftSize), sorted: make([]float32, 2*pass+1)}
	for i := range s.blocks {
		s.blocks[i] = make([]float32, fftSize)
	}
	return s
}

func (s *spectrum) push(v complex64) bool {
	if s.fast {
		s.onset(v)
	}
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
	case s.quick > 0 && s.frames == s.quick:
		s.quick = 0
		s.estimate()
		return true
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

func (s *spectrum) onset(v complex64) {
	s.pw += (real(v)*real(v) + imag(v)*imag(v) - s.pw) * (1.0 / specPowerN)
	switch {
	case s.settled < specSettle:
		if s.settled == 0 {
			s.quick = specQuick
		}
		s.settled++
		s.floor = s.pw
	case s.armed && s.pw > specRise*s.floor:
		s.armed = false
		clear(s.power)
		s.frames, s.quick = 0, specQuick
	default:
		s.armed = s.armed || s.pw < specRearm*s.floor
		s.floor += (s.pw - s.floor) * (1.0 / specFloorN)
	}
}

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
	win := s.sorted[:2*half+1]
	for k := range win {
		win[k] = s.power[(center-half+k+fftSize)%fftSize]
	}
	slices.Sort(win)
	lo := 0
	for lo < len(win)-1 && win[lo] < specCutLevel*win[len(win)/2] {
		lo++
	}
	floor = max(floor, float64(win[lo+(len(win)-lo)*specFloorQ/100]))
	var num, den float64
	for k := center - half; k <= center+half; k++ {
		e := max(pass(k)-floor, 0)
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
