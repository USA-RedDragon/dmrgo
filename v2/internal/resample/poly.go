package resample

// Poly32 interpolates a real stream by an exact ratio of l outputs to m
// inputs: each input gives the outputs whose time falls in its period,
// each the dot product of the last taps inputs, newest first, with one of
// l rows of a table, such as the phases of a transmit pulse. Its timing
// counts in whole phases, so it never drifts. It does not allocate.
type Poly32 struct{ p poly[float32] }

// Poly64 is Poly32 with a float64 table and history.
type Poly64 struct{ p poly[float64] }

// Init sets p up with the table h of l rows of taps coefficients, row r
// for the output r/l of an input period after the newest input, and ring
// for the history, which must hold 2*taps values. It returns false, with
// p unchanged, when h or ring is short, a count is not positive, or m
// exceeds l.
func (p *Poly32) Init(h []float32, taps, l, m int, ring []float32) bool {
	return p.p.init(h, taps, l, m, ring)
}

// Table returns the table of p, row after row, which the caller may change
// between calls.
func (p *Poly32) Table() []float32 { return p.p.h }

// MaxOut returns the most outputs that one input gives.
func (p *Poly32) MaxOut() int { return p.p.maxOut() }

// Push takes the input v, writes the outputs that it completes to dst and
// returns their number. With a dst shorter than MaxOut it writes nothing,
// does not take v and returns 0.
func (p *Poly32) Push(v float32, dst []float64) int { return p.p.push(v, dst) }

// Init is Poly32.Init.
func (p *Poly64) Init(h []float64, taps, l, m int, ring []float64) bool {
	return p.p.init(h, taps, l, m, ring)
}

// Table is Poly32.Table.
func (p *Poly64) Table() []float64 { return p.p.h }

// MaxOut is Poly32.MaxOut.
func (p *Poly64) MaxOut() int { return p.p.maxOut() }

// Push is Poly32.Push.
func (p *Poly64) Push(v float64, dst []float64) int { return p.p.push(v, dst) }

type poly[T float32 | float64] struct {
	h    []T
	ring []T
	taps int
	pos  int
	l, m int
	acc  int
}

func (p *poly[T]) init(h []T, taps, l, m int, ring []T) bool {
	if taps <= 0 || l <= 0 || m <= 0 || m > l || len(h) < l*taps || len(ring) < 2*taps {
		return false
	}
	*p = poly[T]{h: h[:l*taps], ring: ring[:2*taps], taps: taps, l: l, m: m}
	clear(p.ring)
	return true
}

func (p *poly[T]) maxOut() int {
	if p.m == 0 {
		return 0
	}
	return (p.l + p.m - 1) / p.m
}

func (p *poly[T]) push(v T, dst []float64) int {
	if p.taps == 0 || len(dst) < p.maxOut() {
		return 0
	}
	p.pos--
	if p.pos < 0 {
		p.pos = p.taps - 1
	}
	p.ring[p.pos] = v
	p.ring[p.pos+p.taps] = v
	w := p.ring[p.pos : p.pos+p.taps]
	n := 0
	for ; p.acc < p.l; p.acc += p.m {
		t := p.h[p.acc*p.taps : p.acc*p.taps+p.taps]
		var y T
		for i, c := range t {
			y += c * w[i]
		}
		dst[n] = float64(y)
		n++
	}
	p.acc -= p.l
	return n
}
