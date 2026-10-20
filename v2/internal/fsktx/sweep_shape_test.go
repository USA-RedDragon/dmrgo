//go:build !tinygo

package fsktx

import "math"

func sat16f(x float32) int16 {
	switch {
	case x >= math.MaxInt16:
		return math.MaxInt16
	case x <= math.MinInt16:
		return math.MinInt16
	case x >= 0:
		return int16(x + 0.5)
	default:
		return int16(x - 0.5)
	}
}

func sat16i(x int32) int16 {
	switch {
	case x > math.MaxInt16:
		return math.MaxInt16
	case x < math.MinInt16:
		return math.MinInt16
	default:
		return int16(x)
	}
}

type phaser struct {
	l, m, acc int
}

type polyF64 struct {
	phaser
	taps []float64
	ring []float64
	n    int
	pos  int
	gain float64
}

func newPolyF64(pu pulse, hz int, gain float64) *polyF64 {
	l, m := ratio(hz)
	return &polyF64{phaser: phaser{l: l, m: m}, taps: pu.taps(l, m), ring: make([]float64, 4*pu.span), n: 2 * pu.span, gain: gain}
}

func (k *polyF64) appendSymbol(dst []int16, s int8) []int16 {
	k.pos--
	if k.pos < 0 {
		k.pos = k.n - 1
	}
	k.ring[k.pos] = float64(s)
	k.ring[k.pos+k.n] = float64(s)
	w := k.ring[k.pos : k.pos+k.n]
	for ; k.acc < k.l; k.acc += k.m {
		t := k.taps[k.acc*k.n : k.acc*k.n+k.n]
		var y float64
		for i, c := range t {
			y += c * w[i]
		}
		dst = append(dst, sat16(y*k.gain))
	}
	k.acc -= k.l
	return dst
}

type polyF32 struct {
	phaser
	taps []float32
	ring []float32
	n    int
	pos  int
}

func newPolyF32(pu pulse, hz int, gain float64) *polyF32 {
	l, m := ratio(hz)
	h := pu.taps(l, m)
	t := make([]float32, len(h))
	for i, v := range h {
		t[i] = float32(v * gain)
	}
	return &polyF32{phaser: phaser{l: l, m: m}, taps: t, ring: make([]float32, 4*pu.span), n: 2 * pu.span}
}

func (k *polyF32) appendSymbol(dst []int16, s int8) []int16 {
	k.pos--
	if k.pos < 0 {
		k.pos = k.n - 1
	}
	k.ring[k.pos] = float32(s)
	k.ring[k.pos+k.n] = float32(s)
	w := k.ring[k.pos : k.pos+k.n]
	for ; k.acc < k.l; k.acc += k.m {
		t := k.taps[k.acc*k.n : k.acc*k.n+k.n]
		var y float32
		for i, c := range t {
			y += c * w[i]
		}
		dst = append(dst, sat16f(y))
	}
	k.acc -= k.l
	return dst
}

type polyQ15 struct {
	phaser
	taps  []int32
	ring  []int32
	n     int
	pos   int
	shift uint
}

func newPolyQ15(pu pulse, hz int, gain float64, shift uint) *polyQ15 {
	l, m := ratio(hz)
	h := pu.taps(l, m)
	t := make([]int32, len(h))
	for i, v := range h {
		t[i] = int32(math.Round(v * gain * float64(int(1)<<shift)))
	}
	return &polyQ15{phaser: phaser{l: l, m: m}, taps: t, ring: make([]int32, 4*pu.span), n: 2 * pu.span, shift: shift}
}

func (k *polyQ15) appendSymbol(dst []int16, s int8) []int16 {
	k.push(s)
	return k.emit(dst)
}

func (k *polyQ15) push(s int8) {
	k.pos--
	if k.pos < 0 {
		k.pos = k.n - 1
	}
	k.ring[k.pos] = int32(s)
	k.ring[k.pos+k.n] = int32(s)
}

func (k *polyQ15) emit(dst []int16) []int16 {
	w := k.ring[k.pos : k.pos+k.n]
	half := int32(1) << k.shift >> 1
	for ; k.acc < k.l; k.acc += k.m {
		t := k.taps[k.acc*k.n : k.acc*k.n+k.n]
		var y int32
		for i, c := range t {
			y += c * w[i]
		}
		dst = append(dst, sat16i((y+half)>>k.shift))
	}
	k.acc -= k.l
	return dst
}

func symCode(s int8) uint64 { return uint64(s+3) >> 1 }

type lutQ15 struct {
	phaser
	tab    []int16
	g      uint
	groups int
	half   bool
	reg    uint64
	zeros  uint64
	slow   *polyQ15
}

func newLUTQ15(pu pulse, hz int, g uint, half bool) *lutQ15 {
	const gain = sweepGain
	l, m := ratio(hz)
	n := 2 * pu.span
	groups := (n + int(g) - 1) / int(g)
	h := pu.taps(l, m)
	size := 1 << (2 * g)
	if half {
		size /= 2
	}
	tab := make([]int16, l*groups*size)
	for p := range l {
		for gi := range groups {
			for idx := range size {
				code := idx
				if half {
					code |= 1 << (2*g - 1)
				}
				var v float64
				for j := range int(g) {
					i := gi*int(g) + j
					if i >= n {
						continue
					}
					s := float64(2*((code>>(2*j))&3) - 3)
					v += h[p*n+i] * s
				}
				tab[(p*groups+gi)*size+idx] = sat16(v * gain)
			}
		}
	}
	return &lutQ15{phaser: phaser{l: l, m: m}, zeros: uint64(1)<<n - 1, tab: tab, g: g, groups: groups, half: half, slow: newPolyQ15(pu, hz, gain, 14)}
}

func (k *lutQ15) appendSymbol(dst []int16, s int8) []int16 {
	n := uint(k.slow.n)
	mask := uint64(1)<<(2*n) - 1
	k.reg = (k.reg<<2 | symCode(s)) & mask
	z := uint64(0)
	if s == 0 {
		z = 1
	}
	k.zeros = (k.zeros<<1 | z) & (uint64(1)<<n - 1)
	k.slow.push(s)
	if k.zeros != 0 {
		k.slow.acc = k.acc
		dst = k.slow.emit(dst)
		k.acc = k.slow.acc
		return dst
	}
	gm := uint64(1)<<(2*k.g) - 1
	size := 1 << (2 * k.g)
	if k.half {
		size /= 2
	}
	top := uint64(1) << (2*k.g - 1)
	for ; k.acc < k.l; k.acc += k.m {
		base := k.acc * k.groups * size
		var y int32
		r := k.reg
		for gi := range k.groups {
			idx := r & gm
			r >>= 2 * k.g
			if k.half {
				if idx&top == 0 {
					y -= int32(k.tab[base+gi*size+int((idx^gm)&^top)])
				} else {
					y += int32(k.tab[base+gi*size+int(idx&^top)])
				}
				continue
			}
			y += int32(k.tab[base+gi*size+int(idx)])
		}
		dst = append(dst, sat16i(y))
	}
	k.acc -= k.l
	return dst
}
