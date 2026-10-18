package fsktx

import "math"

// Constants of the DMR 4FSK modulation (ETSI TS 102 361-1 clause 10.2).
const (
	// SymbolRate is the symbol rate, in symbols per second.
	SymbolRate = 4800
	// DeviationHz is the deviation of symbol +1, in Hz. Symbol +3 deviates
	// three times as far, 1944 Hz.
	DeviationHz = 648.0
	// rrcAlpha is the roll-off of the root-raised-cosine filter.
	rrcAlpha = 0.2
)

// window is the taper of a truncated pulse.
type window uint8

// The tapers of a pulse.
const (
	winRect window = iota
	winHamming
	winBlackman
	winKaiser
)

// rrc returns the root-raised-cosine pulse of roll-off 0.2 at t symbols
// from its centre (ETSI TS 102 361-1 clause 10.2.2.2).
func rrc(t float64) float64 {
	const a = rrcAlpha
	switch {
	case t == 0:
		return 1 - a + 4*a/math.Pi
	case math.Abs(math.Abs(4*a*t)-1) < 1e-9:
		return a / math.Sqrt2 * ((1+2/math.Pi)*math.Sin(math.Pi/(4*a)) + (1-2/math.Pi)*math.Cos(math.Pi/(4*a)))
	default:
		return (math.Sin(math.Pi*t*(1-a)) + 4*a*t*math.Cos(math.Pi*t*(1+a))) / (math.Pi * t * (1 - (4*a*t)*(4*a*t)))
	}
}

// taper returns the window w at x, from -1 to 1. beta is the shape of
// winKaiser.
func taper(w window, beta, x float64) float64 {
	if x <= -1 || x >= 1 {
		if w == winRect && x > -1-1e-12 && x < 1+1e-12 {
			return 1
		}
		return 0
	}
	switch w {
	case winHamming:
		return 0.54 + 0.46*math.Cos(math.Pi*x)
	case winBlackman:
		return 0.42 + 0.5*math.Cos(math.Pi*x) + 0.08*math.Cos(2*math.Pi*x)
	case winKaiser:
		return besselI0(beta*math.Sqrt(1-x*x)) / besselI0(beta)
	default:
		return 1
	}
}

// besselI0 returns the modified Bessel function of the first kind of order
// 0 at x, by its power series.
func besselI0(x float64) float64 {
	sum, term := 1.0, 1.0
	for k := 1; k < 64; k++ {
		term *= (x / 2) / float64(k)
		sum += term * term
		if term*term < 1e-17*sum {
			break
		}
	}
	return sum
}

// pulse is the transmit pulse: the root-raised-cosine pulse with span
// symbols on each side of its centre, tapered by win, and with comp the
// weight of a three-tap inverse-sinc that makes up for the droop of a
// zero-order hold at the output rate (1/24 matches it at low frequency).
type pulse struct {
	span int
	win  window
	beta float64
	comp float64
}

// at returns the pulse at t symbols, for samples d symbols apart.
func (p pulse) at(t, d float64) float64 {
	h := func(t float64) float64 { return rrc(t) * taper(p.win, p.beta, t/float64(p.span)) }
	if p.comp == 0 {
		return h(t)
	}
	return (1+2*p.comp)*h(t) - p.comp*h(t-d) - p.comp*h(t+d)
}

// taps returns the polyphase taps of the pulse at the output rate of l/m
// samples per symbol: tap i of phase p, at index p*2*span+i, is the pulse
// at i-span+p/l symbols, which weighs the symbol i symbols before the
// newest one for the sample p/l symbols after the time of the symbol span
// symbols before the newest. The taps are scaled so that the mean over
// the phases of their sums is 1, so a run of one symbol value passes at
// its deviation.
func (p pulse) taps(l, m int) []float64 {
	n := 2 * p.span
	h := make([]float64, l*n)
	var sum float64
	d := float64(m) / float64(l)
	for ph := range l {
		for i := range n {
			t := float64(i-p.span) + float64(ph)/float64(l)
			v := p.at(t, d)
			h[ph*n+i] = v
			sum += v
		}
	}
	for i := range h {
		h[i] *= float64(l) / sum
	}
	return h
}

// ratio returns the output rate hz in samples per symbol as l/m in lowest
// terms.
func ratio(hz int) (l, m int) {
	g := gcd(hz, SymbolRate)
	return hz / g, SymbolRate / g
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
