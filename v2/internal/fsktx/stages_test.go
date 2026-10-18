//go:build !tinygo

package fsktx

import "math"

// The interpolations of upStage.
const (
	stageHold = iota
	stageLinear
	stageFIR
	stageCIC
)

// upStage raises the rate of the samples of a shaper by k: by holding each
// sample, by linear interpolation, by a polyphase FIR with taps coefficients
// per phase (Kaiser windowed sinc in Q15), or by a CIC filter of the given order.
type upStage struct {
	in    shaper
	buf   []int16
	k     int
	mode  int
	prev  int32
	taps  [][]int32
	ring  []int32
	pos   int
	order int
	integ [4]int64
	comb  [4]int64
	recip int64
}

func newUpStage(in shaper, k, mode, taps, order int) *upStage {
	u := &upStage{in: in, k: k, mode: mode, order: order}
	switch mode {
	case stageFIR:
		n := k * taps
		half := float64(n-1) / 2
		u.taps = make([][]int32, k)
		for p := range k {
			u.taps[p] = make([]int32, taps)
			for j := range taps {
				t := float64(j*k+p) - half
				x := t / float64(k)
				v := 1.0
				if x != 0 {
					v = math.Sin(math.Pi*x) / (math.Pi * x)
				}
				v *= taper(winKaiser, 7, t/(half+1))
				u.taps[p][j] = int32(math.Round(v * (1 << 15)))
			}
		}
		u.ring = make([]int32, 2*taps)
	case stageCIC:
		u.recip = int64(math.Round((1 << 24) / math.Pow(float64(k), float64(order-1))))
	}
	return u
}

func (u *upStage) appendSymbol(dst []int16, s int8) []int16 {
	u.buf = u.in.appendSymbol(u.buf[:0], s)
	for _, y := range u.buf {
		v := int32(y)
		switch u.mode {
		case stageHold:
			for range u.k {
				dst = append(dst, y)
			}
		case stageLinear:
			for j := 1; j <= u.k; j++ {
				dst = append(dst, int16(u.prev+(v-u.prev)*int32(j)/int32(u.k)))
			}
		case stageFIR:
			n := len(u.taps[0])
			u.pos--
			if u.pos < 0 {
				u.pos = n - 1
			}
			u.ring[u.pos] = v
			u.ring[u.pos+n] = v
			w := u.ring[u.pos : u.pos+n]
			for p := range u.k {
				var acc int32
				for j, c := range u.taps[p] {
					acc += c * w[j]
				}
				dst = append(dst, sat16i((acc+1<<14)>>15))
			}
		case stageCIC:
			x := int64(v)
			for i := range u.order {
				x, u.comb[i] = x-u.comb[i], x
			}
			for j := range u.k {
				z := int64(0)
				if j == 0 {
					z = x
				}
				for i := range u.order {
					u.integ[i] += z
					z = u.integ[i]
				}
				dst = append(dst, sat16i(int32((z*u.recip+1<<23)>>24)))
			}
		}
		u.prev = v
	}
	return dst
}
