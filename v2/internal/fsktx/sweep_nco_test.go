//go:build !tinygo

package fsktx

import "math"

const ncoBits = 8

type nco struct {
	bits   uint
	tab    []int16
	phase  uint32
	step   int32
	off    int32
	up     int
	prev   int32
	interp bool
}

func newNCO(hzPerUnit float64, rate, up int, amp, offsetHz float64, bits uint, interp bool) *nco {
	tab := make([]int16, 1<<bits+1)
	for i := range tab {
		tab[i] = int16(math.Round(amp * math.Cos(2*math.Pi*float64(i)/float64(int(1)<<bits))))
	}
	return &nco{bits: bits, tab: tab, step: int32(math.Round(hzPerUnit / float64(rate) * (1 << 32))), off: int32(math.Round(offsetHz / float64(rate) * (1 << 32))), up: up, interp: interp}
}

func (o *nco) run(dst []int16, f []int16) {
	sh := 32 - o.bits
	quarter := uint32(1) << o.bits >> 2
	mask := uint32(1)<<o.bits - 1
	t := o.tab
	j := 0
	for _, y := range f {
		w := int32(y)*o.step + o.off
		var dw int32
		if o.up > 1 {
			dw = (w - o.prev) / int32(o.up)
			w, o.prev = o.prev+dw, w
		}
		for range o.up {
			o.phase += uint32(w)
			w += dw
			i := o.phase >> sh
			q := (i - quarter) & mask
			c, s := int32(t[i]), int32(t[q])
			if o.interp {
				fr := int32(o.phase << o.bits >> 17)
				c += (int32(t[i+1]) - c) * fr >> 15
				s += (int32(t[q+1]) - s) * fr >> 15
			}
			dst[j], dst[j+1] = int16(c), int16(s)
			j += 2
		}
	}
}
