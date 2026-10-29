//go:build !tinygo

package fsktx

import "math"

type shaper interface {
	appendSymbol(dst []int16, s int8) []int16
}

const (
	upHold = iota
	upLinear
)

const (
	ncoTable = iota
	ncoTableInterp
	ncoCORDIC
	ncoF64
	ncoRotF32
)

type iqPipe struct {
	sh     shaper
	buf    []int16
	up     int
	l2, m2 int
	acc2   int
	prev   int64
	kq     int64
	off    int64
	phase  uint32
	nco    int
	tab    []int16
	bits   uint
	iters  int
	amp    float64
	ampQ   int32
	rot    complex64
	rotN   int
	hzUnit float64
}

func newIQPipe(sh shaper, hz1, gain float64, rate int, up, nco int, bits uint, iters int, offsetHz, amp float64) *iqPipe {
	g := gcd(rate, int(hz1))
	p := &iqPipe{sh: sh, up: up, l2: rate / g, m2: int(hz1) / g, nco: nco, bits: bits, iters: iters, amp: amp}
	p.hzUnit = DeviationHz / gain
	p.kq = int64(math.Round(p.hzUnit / float64(rate) * (1 << 32) * (1 << 16)))
	p.off = int64(math.Round(offsetHz / float64(rate) * (1 << 32) * (1 << 16)))
	p.ampQ = int32(math.Round(p.amp))
	p.rot = 1
	if nco == ncoTable || nco == ncoTableInterp {
		p.tab = make([]int16, 1<<bits+1)
		for i := range p.tab {
			p.tab[i] = int16(math.Round(32767 * math.Cos(2*math.Pi*float64(i)/float64(int(1)<<bits))))
		}
	}
	return p
}

func (p *iqPipe) appendSymbol(dst []int16, s int8) []int16 {
	p.buf = p.sh.appendSymbol(p.buf[:0], s)
	for _, y := range p.buf {
		w1 := int64(y)*p.kq + p.off
		var w, dw int64
		if p.up == upLinear {
			w = p.prev + (w1-p.prev)*int64(p.acc2)/int64(p.l2)
			dw = (w1 - p.prev) * int64(p.m2) / int64(p.l2)
		} else {
			w = w1
		}
		n := 0
		for a := p.acc2; a < p.l2; a += p.m2 {
			n++
		}
		dst = p.run(dst, w, dw, n)
		p.acc2 += n*p.m2 - p.l2
		p.prev = w1
	}
	return dst
}

func (p *iqPipe) run(dst []int16, w, dw int64, n int) []int16 {
	switch p.nco {
	case ncoTable:
		sh := 32 - p.bits
		q := uint32(1) << p.bits >> 2
		for range n {
			p.phase += uint32(w >> 16)
			w += dw
			i := p.phase >> sh
			c, s := p.tab[i], p.tab[(i-q)&(uint32(1)<<p.bits-1)]
			dst = append(dst, int16((int32(c)*p.ampQ+(1<<14))>>15), int16((int32(s)*p.ampQ+(1<<14))>>15))
		}
	case ncoTableInterp:
		sh := 32 - p.bits
		q := uint32(1) << p.bits >> 2
		mask := uint32(1)<<p.bits - 1
		for range n {
			p.phase += uint32(w >> 16)
			w += dw
			i := p.phase >> sh
			f := int32(p.phase << p.bits >> 17)
			c0, c1 := int32(p.tab[i]), int32(p.tab[i+1])
			j := (i - q) & mask
			s0, s1 := int32(p.tab[j]), int32(p.tab[j+1])
			c := c0 + (c1-c0)*f>>15
			s := s0 + (s1-s0)*f>>15
			dst = append(dst, int16((c*p.ampQ+(1<<14))>>15), int16((s*p.ampQ+(1<<14))>>15))
		}
	case ncoCORDIC:
		for range n {
			p.phase += uint32(w >> 16)
			w += dw
			c, s := cordic(p.phase, p.iters)
			dst = append(dst, int16((c*p.ampQ+(1<<14))>>15), int16((s*p.ampQ+(1<<14))>>15))
		}
	case ncoF64:
		for range n {
			p.phase += uint32(w >> 16)
			w += dw
			s, c := math.Sincos(float64(p.phase) * (2 * math.Pi / (1 << 32)))
			dst = append(dst, int16(math.Round(c*p.amp)), int16(math.Round(s*p.amp)))
		}
	case ncoRotF32:
		for range n {
			d := float32(float64(w>>16) * (2 * math.Pi / (1 << 32)))
			w += dw
			d2 := d * d
			r := complex(1-d2/2, d*(1-d2/6))
			p.rot *= r
			p.rotN++
			if p.rotN == 64 {
				p.rotN = 0
				m := real(p.rot)*real(p.rot) + imag(p.rot)*imag(p.rot)
				p.rot *= complex((3-m)/2, 0)
			}
			a := float32(p.amp)
			dst = append(dst, int16(roundf(real(p.rot)*a)), int16(roundf(imag(p.rot)*a)))
		}
	}
	return dst
}

func roundf(x float32) float32 {
	if x < 0 {
		return float32(int32(x - 0.5))
	}
	return float32(int32(x + 0.5))
}

var cordicAngles = [...]uint32{536870912, 316933406, 167458907, 85004756, 42667331, 21354465, 10679838, 5340245, 2670163, 1335087, 667544, 333772, 166886, 83443, 41722, 20861, 10430, 5215, 2608, 1304}

func cordic(phase uint32, iters int) (int32, int32) {
	const k = 19898
	q := phase >> 30
	z := int32(phase - q<<30)
	if z >= 1<<29 {
		z -= 1 << 30
		q = (q + 1) & 3
	}
	x, y := int32(k), int32(0)
	switch q {
	case 1:
		x, y = 0, k
	case 2:
		x, y = -k, 0
	case 3:
		x, y = 0, -k
	}
	for i := range min(iters, len(cordicAngles)) {
		if z >= 0 {
			x, y = x-y>>i, y+x>>i
			z -= int32(cordicAngles[i])
		} else {
			x, y = x+y>>i, y-x>>i
			z += int32(cordicAngles[i])
		}
	}
	return x, y
}
