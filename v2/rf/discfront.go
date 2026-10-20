package rf

import "math"

const (
	discRate = basebandRate / 2
	halfTaps = 11
	halfSide = (halfTaps/2 + 1) / 2
	halfBeta = 6
	preTaps  = 17
	discSpan = 4
	discSPS  = samplesPerSy / 2
	rrcTaps  = 2*discSpan*discSPS + 1
)

type discFront struct {
	half    bool
	odd     bool
	hbHist  [2 * halfTaps]complex64
	hbHead  int
	ncoStep complex64
	ncoRot  complex64
	ncoN    int
	preHist [2 * preTaps]complex64
	preHead int
	prev    complex64
	lag     complex64
	fHist   [2 * rrcTaps]float32
	fHead   int
}

func newDiscFront(half bool) *discFront {
	return &discFront{half: half, ncoRot: 1, ncoStep: 1}
}

func (f *discFront) center() int64 {
	c := -int64(preTaps-1) - 1 - discSpan*samplesPerSy
	if !f.half {
		c += 1 - halfTaps/2
	}
	return c
}

func (f *discFront) tune(hz float64) {
	p := -2 * math.Pi * hz / discRate
	f.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

func (f *discFront) push(v complex64) (s, power float32, ok bool) {
	if !f.half {
		f.hbHist[f.hbHead] = v
		f.hbHist[f.hbHead+halfTaps] = v
		if f.hbHead++; f.hbHead == halfTaps {
			f.hbHead = 0
		}
		if f.odd = !f.odd; f.odd {
			return 0, 0, false
		}
		x := f.hbHist[f.hbHead : f.hbHead+halfTaps]
		c := halfTaps / 2
		v = x[c] * complex(discHalfTaps[c], 0)
		for k := range halfSide {
			h := discHalfTaps[c-2*k-1]
			a, b := x[c-2*k-1], x[c+2*k+1]
			v += complex((real(a)+real(b))*h, (imag(a)+imag(b))*h)
		}
	}
	if f.ncoN++; f.ncoN == 512 {
		f.ncoN = 0
		g := (3 - real(f.ncoRot)*real(f.ncoRot) - imag(f.ncoRot)*imag(f.ncoRot)) / 2
		f.ncoRot *= complex(g, 0)
	}
	v *= f.ncoRot
	f.ncoRot *= f.ncoStep
	f.preHist[f.preHead] = v
	f.preHist[f.preHead+preTaps] = v
	if f.preHead++; f.preHead == preTaps {
		f.preHead = 0
	}
	x := f.preHist[f.preHead : f.preHead+preTaps]
	c := preTaps / 2
	y := x[c] * complex(discPreTaps[c], 0)
	for k, h := range discPreTaps[:c] {
		a, b := x[k], x[preTaps-1-k]
		y += complex((real(a)+real(b))*h, (imag(a)+imag(b))*h)
	}
	d := y * complex(real(f.prev), -imag(f.prev))
	f.prev, f.lag = y, d
	fr := atan2f(imag(d), real(d)) * (discRate / (2 * math.Pi))
	const nr = rrcTaps
	f.fHist[f.fHead] = fr
	f.fHist[f.fHead+nr] = fr
	if f.fHead++; f.fHead == nr {
		f.fHead = 0
	}
	h := f.fHist[f.fHead : f.fHead+nr]
	m := nr / 2
	s = h[m] * discRRCTaps[m]
	for k, t := range discRRCTaps[:m] {
		s += (h[k] + h[nr-1-k]) * t
	}
	return s, real(y)*real(y) + imag(y)*imag(y), true
}
