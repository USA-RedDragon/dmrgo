package fsktx

import "math"

// Constants of the DMR 4FSK modulation (ETSI TS 102 361-1 clause 10.2).
const (
	// SymbolRate is the symbol rate, in symbols per second.
	SymbolRate = 4800
	// DeviationHz is the deviation of symbol +1, in Hz. Symbol +3 deviates
	// three times as far, 1944 Hz.
	DeviationHz = 648.0
	rrcAlpha    = 0.2
)

type window uint8

const (
	winRect window = iota
	winHamming
	winBlackman
	winKaiser
)

// ETSI TS 102 361-1 clause 10.2.2.2
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

type pulse struct {
	span     int
	win      window
	beta     float64
	comp     float64
	inv      [2]float64
	echo     *[4*maxTaps + 1]float64
	gain     float64
	tab      []float64
	per      int
	edge     float64
	flat     bool
	flatGain float64
}

func (p pulse) core(t float64) float64 {
	if p.tab != nil {
		return p.shape(t)
	}
	return rrc(t)
}

func (p pulse) taper(t float64) float64 {
	if p.tab != nil {
		if d := float64(p.span) - math.Abs(t); p.edge > 0 && d >= 0 && d < p.edge {
			return 0.5 * (1 - math.Cos(math.Pi*d/p.edge))
		}
		return taper(winRect, 0, t/float64(p.span))
	}
	return taper(p.win, p.beta, t/float64(p.span))
}

func (p pulse) shape(t float64) float64 {
	x := (t + float64(p.span)) * float64(p.per)
	n := len(p.tab)
	if x < 0 || x > float64(n-1) {
		return 0
	}
	i := int(x)
	f := x - float64(i)
	at := func(j int) float64 {
		if j < 0 || j >= n {
			return 0
		}
		return p.tab[j]
	}
	y0, y1, y2, y3 := at(i-1), at(i), at(i+1), at(i+2)
	return y1 + 0.5*f*(y2-y0+f*(2*y0-5*y1+4*y2-y3+f*(3*(y1-y2)+y3-y0)))
}

func (p pulse) at(t, d float64) float64 {
	if p.inv != [2]float64{} {
		c1, c2 := p.inv[0], p.inv[1]
		return (1-2*c1-2*c2)*p.base(t) + c1*(p.base(t-d)+p.base(t+d)) + c2*(p.base(t-2*d)+p.base(t+2*d))
	}
	if p.comp == 0 {
		return p.base(t)
	}
	return (1+2*p.comp)*p.base(t) - p.comp*p.base(t-d) - p.comp*p.base(t+d)
}

func (p *pulse) invert(hz float64, chain func(f float64) float64) {
	var a11, a12, a22, b1, b2 float64
	const n = 512
	for i := range n {
		f := (float64(i) + 0.5) / n * (1 + rrcAlpha) * SymbolRate / 2
		w := rcSpectrum(f / SymbolRate)
		om := 2 * math.Pi * f / hz
		x1, x2 := 2*(math.Cos(om)-1), 2*(math.Cos(2*om)-1)
		y := 1/chain(f) - 1
		a11 += w * x1 * x1
		a12 += w * x1 * x2
		a22 += w * x2 * x2
		b1 += w * x1 * y
		b2 += w * x2 * y
	}
	det := a11*a22 - a12*a12
	p.inv = [2]float64{(b1*a22 - b2*a12) / det, (a11*b2 - a12*b1) / det}
}

// etsiHz is the tone of the deviation test pattern +3 +3 -3 -3 (ETSI TS
const etsiHz = SymbolRate / 4.0

func (p pulse) baseGain(f float64) float64 {
	const grid = 64
	var a, b float64
	for j := -p.span * grid; j <= p.span*grid; j++ {
		t := float64(j) / grid
		v := p.base(t)
		a += v * math.Cos(2*math.Pi*f*t)
		b += v
	}
	return a / b
}

func rcSpectrum(f float64) float64 {
	const a = rrcAlpha
	f = math.Abs(f)
	switch {
	case f <= (1-a)/2:
		return 1
	case f >= (1+a)/2:
		return 0
	default:
		return 0.5 * (1 + math.Cos(math.Pi/a*(f-(1-a)/2)))
	}
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func holdChain(hz float64) func(f float64) float64 {
	return func(f float64) float64 { return sinc(f / hz) }
}

func interpChain(hz, rate float64) func(f float64) float64 {
	if math.IsInf(rate, 1) {
		return func(f float64) float64 { s := sinc(f / hz); return s * s }
	}
	p := rate / float64(gcd(int(rate), int(hz)))
	return func(f float64) float64 {
		x := f / hz
		l := 1.0
		if x != 0 {
			l = math.Sin(math.Pi*x) / (p * math.Sin(math.Pi*x/p))
		}
		return l * l / sinc(f/rate)
	}
}

func (p pulse) base(t float64) float64 {
	w := p.taper(t)
	if w == 0 {
		return 0
	}
	v := p.core(t)
	if p.echo != nil {
		for k, b := range p.echo[:nyquistStep*2*p.span+1] {
			if b != 0 {
				v += b * p.core(t-float64(k-nyquistStep*p.span)/nyquistStep)
			}
		}
	}
	return v * w
}

const nyquistGrid = 64

const nyquistRxSpan = 32

const (
	nyquistStep  = 2
	nyquistRidge = 1e-6
)

const nyquistFlat = 1e3

func (p *pulse) nyquist() {
	n := nyquistStep*2*p.span + 1
	nm := 4*p.span + 1
	ng := 2 * p.span * nyquistGrid
	tg := func(j int) float64 { return float64(j)/nyquistGrid - float64(p.span) + 0.5/nyquistGrid }
	a := make([][]float64, nm+1)
	for i := range a {
		a[i] = make([]float64, n+1)
	}
	rx := make([]float64, 6*p.span*nyquistGrid)
	var area float64
	for i := -nyquistRxSpan * nyquistGrid; i < nyquistRxSpan*nyquistGrid; i++ {
		area += rrc((float64(i)+0.5)/nyquistGrid) / nyquistGrid
	}
	for i := range rx {
		if x := (float64(i)+0.5)/nyquistGrid - float64(3*p.span); math.Abs(x) < nyquistRxSpan {
			rx[i] = rrc(x) / area
		}
	}
	for j := range ng {
		t := tg(j)
		w := p.taper(t) / nyquistGrid
		for k := range n + 1 {
			var f float64
			if k == n {
				f = p.core(t) * w
			} else {
				f = p.core(t-float64(k-nyquistStep*p.span)/nyquistStep) * w
			}
			for m := range nm {
				a[m][k] += f * rx[(m+2*p.span)*nyquistGrid-j-1]
			}
			a[nm][k] += f
		}
	}
	rows := make([][]float64, nm, nm+1)
	for m := range nm {
		rows[m] = make([]float64, n+1)
		for k := range n + 1 {
			rows[m][k] = a[m][k]
			if m == 2*p.span {
				rows[m][k] -= a[nm][k]
			}
		}
	}
	if p.flat {
		e := make([]float64, n+1)
		for j := range ng {
			t := tg(j)
			w := p.taper(t) / nyquistGrid * nyquistFlat * (math.Cos(2*math.Pi*etsiHz/SymbolRate*t) - 1 - p.flatGain)
			for k := range n + 1 {
				if k == n {
					e[k] += p.core(t) * w
				} else {
					e[k] += p.core(t-float64(k-nyquistStep*p.span)/nyquistStep) * w
				}
			}
		}
		rows = append(rows, e)
	}
	g := make([][]float64, n)
	for i := range g {
		g[i] = make([]float64, n+1)
		for j := range n {
			for _, r := range rows {
				g[i][j] += r[i] * r[j]
			}
		}
		g[i][i] += nyquistRidge
		if i == nyquistStep*p.span {
			clear(g[i])
			g[i][i] = 1
			continue
		}
		for _, r := range rows {
			g[i][n] -= r[i] * r[n]
		}
	}
	for c := range n {
		pv := c
		for r := c + 1; r < n; r++ {
			if math.Abs(g[r][c]) > math.Abs(g[pv][c]) {
				pv = r
			}
		}
		g[c], g[pv] = g[pv], g[c]
		for r := range n {
			if r != c {
				f := g[r][c] / g[c][c]
				for j := c; j <= n; j++ {
					g[r][j] -= f * g[c][j]
				}
			}
		}
	}
	p.echo = new([4*maxTaps + 1]float64)
	for k := range n {
		p.echo[k] = g[k][n] / g[k][k]
	}
}

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
