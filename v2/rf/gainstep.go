package rf

const (
	gainStepN       = 8
	gainStepRatio   = 2
	gainStepFlat    = 1.25
	gainStepSeconds = 0.002
	gainStepDecay   = 0.98963740
)

// gainStep turns a step of the input level, as from a change of the tuner
// gain, into an exponential approach to the new level with a time constant
// of gainStepSeconds. The step multiplies every signal of the input, and the
// sidebands it puts on a strong adjacent channel reach into the channel above
// the wanted signal for the length of the channel filter; the approach keeps
// most of their power within about 80 Hz of that channel. A step is a jump of
// at least gainStepRatio in power between two samples with gainStepN samples
// on each side flat to within gainStepFlat, which a constant-envelope input
// holds and noise does not. The samples come out gainStepN samples late, so
// the gain moves from the first sample after the step; the I/Q receiver runs
// it only while the spectrum hunt has an adjacent channel, where a step
// matters, so the input has no added latency otherwise.
type gainStep struct {
	buf  [gainStepN]complex64
	pw   [2 * gainStepN]float32
	head int
	n    int
	c    float32
}

func (g *gainStep) init() { *g = gainStep{c: 1} }

func (g *gainStep) push(v complex64) (complex64, bool) {
	p := real(v)*real(v) + imag(v)*imag(v)
	k := g.head % gainStepN
	out := g.buf[k]
	g.buf[k] = v
	g.pw[g.head] = p
	g.head = (g.head + 1) % (2 * gainStepN)
	if g.n < 2*gainStepN {
		g.n++
	}
	if g.n <= gainStepN {
		return 0, false
	}
	o := out * complex(g.c, 0)
	g.c = 1 + (g.c-1)*gainStepDecay
	if g.n == 2*gainStepN {
		g.detect()
	}
	return o, true
}

func (g *gainStep) detect() {
	pre, post := g.pw[(g.head+gainStepN-1)%(2*gainStepN)], g.pw[(g.head+gainStepN)%(2*gainStepN)]
	if pre*gainStepRatio > post && post*gainStepRatio > pre {
		return
	}
	a, b, ok := g.window(g.head)
	if !ok {
		return
	}
	c, d, ok := g.window(g.head + gainStepN)
	if !ok || b*gainStepRatio > c && d*gainStepRatio > a {
		return
	}
	g.c *= sqrtf(fsum(g.pw[:], g.head) / fsum(g.pw[:], g.head+gainStepN))
}

func (g *gainStep) window(at int) (lo, hi float32, ok bool) {
	lo, hi = g.pw[at%(2*gainStepN)], g.pw[at%(2*gainStepN)]
	for i := 1; i < gainStepN; i++ {
		p := g.pw[(at+i)%(2*gainStepN)]
		lo, hi = min32(lo, p), max32(hi, p)
	}
	return lo, hi, lo > 0 && hi <= gainStepFlat*lo
}

func fsum(pw []float32, at int) float32 {
	var s float32
	for i := range gainStepN {
		s += pw[(at+i)%(2*gainStepN)]
	}
	return s
}

func (g *gainStep) drain(emit func(complex64)) {
	m := min(g.n, gainStepN)
	for j := range m {
		emit(g.buf[(g.head-m+j+2*gainStepN)%gainStepN] * complex(g.c, 0))
		g.c = 1 + (g.c-1)*gainStepDecay
	}
	g.init()
}
