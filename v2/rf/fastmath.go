package rf

import "math"

func atan2f(y, x float32) float32 {
	ax, ay := x, y
	if ax < 0 {
		ax = -ax
	}
	if ay < 0 {
		ay = -ay
	}
	if ax == 0 && ay == 0 {
		return 0
	}
	swap := ay > ax
	n, d := ay, ax
	if swap {
		n, d = ax, ay
	}
	a := n / d
	s := a * a
	r := a * (0.99986633 + s*(-0.3303048+s*(0.18015937+s*(-0.08515649+s*0.020845189))))
	if swap {
		r = math.Pi/2 - r
	}
	if x < 0 {
		r = math.Pi - r
	}
	if y < 0 {
		r = -r
	}
	return r
}

func sqrtf(x float32) float32 {
	return float32(math.Sqrt(float64(x)))
}

func roundf(x float32) float32 {
	if x < 0 {
		return float32(int32(x - 0.5))
	}
	return float32(int32(x + 0.5))
}

func log32(x float32) float32 {
	b := math.Float32bits(x)
	e := float32(int32(b>>23&0xff) - 127)
	m := math.Float32frombits(b&0x7fffff | 0x3f800000)
	t := (m - 1) / (m + 1)
	t2 := t * t
	return e*math.Ln2 + 2*t*(1+t2*(1.0/3+t2*(1.0/5+t2*(1.0/7))))
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func pow2(e int32) float32 {
	b := e + 127
	if b <= 0 || b > 254 {
		return 0
	}
	return math.Float32frombits(uint32(b) << 23)
}

func sat8(x float32) int8 {
	switch {
	case x >= math.MaxInt8:
		return math.MaxInt8
	case x <= math.MinInt8:
		return math.MinInt8
	case x < 0:
		return int8(x - 0.5)
	}
	return int8(x + 0.5)
}

func sincosf(x float32) (sin, cos float32) {
	const (
		twoOverPi = 0.63661977
		pio2Hi    = 1.5707963705062866
		pio2Lo    = -4.371139000186241e-08
	)
	k := x * twoOverPi
	if k < 0 {
		k -= 0.5
	} else {
		k += 0.5
	}
	q := int32(k)
	r := (x - float32(q)*pio2Hi) - float32(q)*pio2Lo
	r2 := r * r
	sr := r + r*r2*(-1.6666654611e-1+r2*(8.3321608736e-3+r2*-1.9515295891e-4))
	cr := 1 - 0.5*r2 + r2*r2*(4.166664568298827e-2+r2*(-1.388731625493765e-3+r2*2.443315711809948e-5))
	switch q & 3 {
	case 1:
		return cr, -sr
	case 2:
		return -sr, -cr
	case 3:
		return -cr, sr
	}
	return sr, cr
}
