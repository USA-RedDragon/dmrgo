package resample

// Poly interpolates a real stream by an exact ratio of l outputs to m
// inputs: each input gives the outputs whose time falls in its period,
// each the dot product of the last taps inputs, newest first, with one of
// l rows of a table, such as the phases of a transmit pulse. Its timing
// counts in whole phases, so it never drifts. It does not allocate.
type Poly[T float32 | float64] struct {
	h    []T
	ring []T
	taps int
	pos  int
	l, m int
	acc  int
}

// Init sets p up with the table h of l rows of taps coefficients, row r
// for the output r/l of an input period after the newest input, and ring
// for the history, which must hold 2*taps values. It returns false, with
// p unchanged, when h or ring is short, a count is not positive, or m
// exceeds l.
func (p *Poly[T]) Init(h []T, taps, l, m int, ring []T) bool {
	if taps <= 0 || l <= 0 || m <= 0 || m > l || len(h) < l*taps || len(ring) < 2*taps {
		return false
	}
	*p = Poly[T]{h: h[:l*taps], ring: ring[:2*taps], taps: taps, l: l, m: m}
	clear(p.ring)
	return true
}

// Table returns the table of p, row after row, which the caller may change
// between calls.
func (p *Poly[T]) Table() []T { return p.h }

// MaxOut returns the most outputs that one input gives.
func (p *Poly[T]) MaxOut() int {
	if p.m == 0 {
		return 0
	}
	return (p.l + p.m - 1) / p.m
}

// Push takes the input v, writes the outputs that it completes to dst and
// returns their number. With a dst shorter than MaxOut it writes nothing,
// does not take v and returns 0.
func (p *Poly[T]) Push(v T, dst []float64) int {
	if p.taps == 0 || len(dst) < p.MaxOut() {
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
