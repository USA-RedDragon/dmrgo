//go:build !tinygo

package fsktx

type lut8 struct {
	tab  []int16
	zero []int16
	l    int
	reg  uint32
	zm   uint32
}

func newLUT8(pu pulse, l int, gain float64) *lut8 {
	tab, zero := lut8Tables(pu, l, gain)
	return &lut8{tab: tab, zero: zero, l: l, reg: 0xAAAA, zm: 0xFF}
}

func lut8Tables(pu pulse, l int, gain float64) (tab, zero []int16) {
	h := pu.taps(l, 1)
	tab = make([]int16, l*2*256)
	zero = make([]int16, l*2*16)
	for p := range l {
		for g := range 2 {
			for idx := range 256 {
				var v float64
				for j := range 4 {
					s := float64(2*((idx>>(2*j))&3) - 3)
					v += h[p*8+4*g+j] * s
				}
				tab[(p*2+g)*256+idx] = sat16(v * gain)
			}
			for m := range 16 {
				var v float64
				for j := range 4 {
					if m>>j&1 != 0 {
						v += h[p*8+4*g+j]
					}
				}
				zero[(p*2+g)*16+m] = sat16(v * gain)
			}
		}
	}
	return tab, zero
}

func (k *lut8) push(s int8) {
	c, z := uint32(s+3)>>1, uint32(0)
	if s == 0 {
		c, z = 2, 1
	}
	k.reg = (k.reg<<2 | c) & 0xFFFF
	k.zm = (k.zm<<1 | z) & 0xFF
}

func (k *lut8) symbol(dst []int16, s int8) int {
	k.push(s)
	dst = dst[:k.l]
	lo, hi := int(k.reg&255), int(k.reg>>8)
	for p := range dst {
		t := k.tab[p*512 : p*512+512]
		y := int32(t[lo]) + int32(t[256+hi])
		if k.zm != 0 {
			z := k.zero[p*32 : p*32+32]
			y -= int32(z[k.zm&15]) + int32(z[16+k.zm>>4])
		}
		dst[p] = int16(y)
	}
	return k.l
}

func (k *lut8) appendSymbol(dst []int16, s int8) []int16 {
	n := len(dst)
	for range k.l {
		dst = append(dst, 0)
	}
	k.symbol(dst[n:], s)
	return dst
}
