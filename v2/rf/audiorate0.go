package rf

type rate0 struct {
	hist [audioUpTaps]int16
	rem  uint16
	num  uint16
	den  uint16
	head uint8
	lead int8
}

func (s *rate0) init(rate, native int) {
	*s = rate0{num: uint16(rate), den: uint16(native), lead: -1}
}

func (s *rate0) on() bool { return s.den != 0 }

//go:noinline
func (s *rate0) at() int16 {
	p := uint32(s.rem) << 15 / uint32(s.den)
	h := (*[2 * audioUpTaps]int16)(audioUpQ15[p>>10*audioUpTaps:])
	x, k := &s.hist, uint(s.head)
	x0, x1, x2, x3 := int32(x[k&7]), int32(x[(k+1)&7]), int32(x[(k+2)&7]), int32(x[(k+3)&7])
	x4, x5, x6, x7 := int32(x[(k+4)&7]), int32(x[(k+5)&7]), int32(x[(k+6)&7]), int32(x[(k+7)&7])
	a0 := int32(h[0])*x0 + int32(h[1])*x1 + int32(h[2])*x2 + int32(h[3])*x3 +
		int32(h[4])*x4 + int32(h[5])*x5 + int32(h[6])*x6 + int32(h[7])*x7
	a1 := int32(h[8])*x0 + int32(h[9])*x1 + int32(h[10])*x2 + int32(h[11])*x3 +
		int32(h[12])*x4 + int32(h[13])*x5 + int32(h[14])*x6 + int32(h[15])*x7
	a0, a1 = (a0+1<<14)>>15, (a1+1<<14)>>15
	y := a0 + ((a1-a0)*int32(p&1023)+512)>>10
	return int16(max(min(y, 32767), -32768))
}

func (s *rate0) step() {
	rem := uint32(s.rem) + uint32(s.num)
	if rem >= uint32(s.den) {
		rem -= uint32(s.den)
		s.lead--
	}
	s.rem = uint16(rem)
}

//go:noinline
func (r *AudioReceiver) resample0(x []int16) {
	s, d := &r.rs0, &r.d48
	n := 0
	for _, v := range x {
		s.hist[s.head] = v
		s.head = (s.head + 1) & (audioUpTaps - 1)
		s.lead++
		for s.lead >= audioUpTaps/2 {
			y := s.at()
			s.step()
			if r.f.dec {
				if y, ok := d.push(y); ok {
					d.buf[n] = y
					n++
				}
			} else {
				d.buf[n] = y
				n++
			}
			if n == disc48Out {
				r.write(d.buf[:])
				n = 0
			}
		}
	}
	if n > 0 {
		r.write(d.buf[:n])
	}
}
