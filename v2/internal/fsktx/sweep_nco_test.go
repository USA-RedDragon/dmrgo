//go:build !tinygo

package fsktx

import "math"

// ncoBits is the log2 of the default number of entries of the cosine
// table of nco.
const ncoBits = 8

// nco turns deviation samples into I/Q by a 32-bit phase accumulator and
// a cosine table of 2^bits entries, with linear interpolation between
// entries when interp is set. Each deviation sample steps the phase by
// step units per deviation unit plus off, up times, with the step
// interpolated linearly from the previous sample when up is above 1.
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

// newNCO returns an nco for deviation samples of hzPerUnit Hz per unit at
// rate/up, with output amplitude amp and a carrier offsetHz from 0 Hz.
func newNCO(hzPerUnit float64, rate, up int, amp, offsetHz float64, bits uint, interp bool) *nco {
	tab := make([]int16, 1<<bits+1)
	for i := range tab {
		tab[i] = int16(math.Round(amp * math.Cos(2*math.Pi*float64(i)/float64(int(1)<<bits))))
	}
	return &nco{bits: bits, tab: tab, step: int32(math.Round(hzPerUnit / float64(rate) * (1 << 32))), off: int32(math.Round(offsetHz / float64(rate) * (1 << 32))), up: up, interp: interp}
}

// run writes the I/Q of the deviation samples f to dst, interleaved, which
// has room for 2*up*len(f) values.
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
