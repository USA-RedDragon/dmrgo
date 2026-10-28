package rf

import (
	"math"
	"math/bits"
	"unsafe"
)

const (
	iqLowBuf      = 16
	iqLowPack     = 16
	iqLowScale    = 4096
	iqLowISI      = 0.04
	iqLowAtanNear = false
	iqLowRecip    = true
)

var iqLowTab = [128]uint16{
	81, 244, 407, 570, 732, 894, 1056, 1217, 1377, 1537, 1696, 1854, 2012, 2168, 2324, 2478,
	2632, 2784, 2935, 3085, 3233, 3380, 3526, 3670, 3813, 3955, 4095, 4233, 4370, 4505, 4639, 4771,
	4901, 5030, 5157, 5282, 5406, 5528, 5649, 5768, 5885, 6000, 6114, 6227, 6337, 6446, 6554, 6660,
	6764, 6867, 6968, 7068, 7166, 7262, 7358, 7451, 7544, 7635, 7724, 7812, 7899, 7984, 8068, 8151,
	32263, 31775, 31300, 30840, 30393, 29959, 29537, 29127, 28728, 28339, 27962, 27594, 27235, 26886, 26546, 26214,
	25890, 25575, 25266, 24966, 24672, 24385, 24105, 23831, 23563, 23301, 23045, 22795, 22550, 22310, 22075, 21845,
	21620, 21399, 21183, 20971, 20763, 20560, 20360, 20164, 19972, 19784, 19599, 19418, 19239, 19065, 18893, 18724,
	18558, 18396, 18236, 18078, 17924, 17772, 17623, 17476, 17331, 17189, 17050, 16912, 16777, 16644, 16513, 16384,
}

var iqLowAtan = [16]uint32{
	0x028B0000, 0x0286028B, 0x027C0511, 0x026E078D, 0x025C09FB, 0x02470C57, 0x02300E9E, 0x021610CE,
	0x01FC12E4, 0x01E214E0, 0x01C816C2, 0x01AE188A, 0x01951A38, 0x017D1BCD, 0x01661D4A, 0x01501EB0,
}

type iqLowRx struct {
	d           discRx
	q           uint32
	ar, ai, pow int32
	afcAvg      complex64
	step        int32
	hz          float32
	afcN        int16
	n           uint8
	buf         [iqLowBuf]int16
}

func (r *iqLowRx) center(hz float32) {
	r.hz = hz
	r.step = roundBits(hz/float32(r.d.hz())*(1<<16)) << 16
}

func (r *iqLowRx) init(hz int, on func(*Burst), opt *options) {
	*r = iqLowRx{}
	r.d.setup(discSetup{Hz: hz, OnBurst: on, opt: opt, lowIQ: true}, false)
	r.d.iqLow = hz == iq0LowHz
}

func (r *iqLowRx) write(iq []byte, f IQFormat, words *[2]uint32) {
	size := f.bytes()
	if f == IQFormatCS16 && littleEndian() && uintptr(unsafe.Pointer(unsafe.SliceData(iq)))%4 == 0 {
		if n := len(iq) / 4; n > 0 {
			r.run(unsafe.Slice((*uint32)(unsafe.Pointer(unsafe.SliceData(iq))), n))
		}
		return
	}
	for len(iq) >= size {
		m := min(len(iq)/size, len(words))
		for i := range m {
			words[i] = iqLowWord(iq[i*size:], f)
		}
		r.run(words[:m])
		iq = iq[m*size:]
	}
}

func littleEndian() bool {
	v := uint16(1)
	return *(*byte)(unsafe.Pointer(&v)) == 1
}

func iqLowWord(s []byte, f IQFormat) uint32 {
	switch f {
	case IQFormatCU8:
		return uint32(uint16((2*int32(s[0])-255)<<4)) | uint32((2*int32(s[1])-255)<<4)<<16
	case IQFormatCS8:
		return uint32(uint16((2*int32(int8(s[0]))+1)<<4)) | uint32((2*int32(int8(s[1]))+1)<<4)<<16
	case IQFormatCF32:
		re := math.Float32frombits(uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24)
		im := math.Float32frombits(uint32(s[4]) | uint32(s[5])<<8 | uint32(s[6])<<16 | uint32(s[7])<<24)
		return uint32(uint16(q15(re))) | uint32(q15(im))<<16
	case IQFormatDefault, IQFormatCS16:
	}
	return uint32(s[0]) | uint32(s[1])<<8 | uint32(s[2])<<16 | uint32(s[3])<<24
}

func (r *iqLowRx) block() int { return iqAFCBlock * r.d.hz() / basebandRate }

func (r *iqLowRx) writeBaseband(x []complex64) {
	var b [iqLowPack]uint32
	for len(x) > 0 {
		m := min(len(x), len(b))
		for i, v := range x[:m] {
			b[i] = uint32(uint16(q15(real(v)))) | uint32(q15(imag(v)))<<16
		}
		r.run(b[:m])
		x = x[m:]
	}
}

func q15(v float32) int32 {
	return roundBits(min(max(v*iqLowScale, -32767), 32767))
}

func (r *iqLowRx) run(x []uint32) {
	block := r.block()
	if r.d.iqLow && r.d.front96 {
		for k := 0; k < len(x); {
			m := min(len(x)-k, block-int(r.afcN))
			r.feed(x[k : k+m])
			k += m
			r.afcN += int16(m)
			if int(r.afcN) == block {
				r.afcN = 0
				r.follow()
			}
		}
		return
	}
	for k := 0; k < len(x); {
		m := min(len(x)-k, iqLowBuf-int(r.n), block-int(r.afcN))
		r.halfRun(x[k : k+m])
		k += m
		r.n += uint8(m)
		r.afcN += int16(m)
		if r.n == iqLowBuf || int(r.afcN) == block {
			r.d.write(r.buf[:r.n])
			r.n = 0
		}
		if int(r.afcN) == block {
			r.afcN = 0
			r.follow()
		}
	}
}

func (r *iqLowRx) feed(x []uint32) {
	r.n = 0
	j, k := 0, 0
	for {
		i, m := r.d.run96(r.buf[:r.n], j, x[k:])
		if !r.d.slotDue {
			r.n = 0
			return
		}
		r.d.slotDue = false
		r.d.endSlot()
		k, j = k+m, i
	}
}

func iqLowOf(r *discRx) *iqLowRx { return (*iqLowRx)(unsafe.Pointer(r)) }

//go:noinline
//go:section .text.halfRun
func (r *iqLowRx) halfRunGo(x []uint32) {
	out := r.buf[r.n : int(r.n)+len(x)]
	q, step := r.q, r.step
	ar, ai, pow := r.ar, r.ai, r.pow
	k := 2 * int32(r.d.hz())
	if len(x) == 0 {
		return
	}
	for i := 0; ; {
		y := x[i]
		d := smuad(y, q)
		ar = addRound10(ar, d)
		c := smusdx(q, y)
		ai = addRound10(ai, c)
		pow = addRound10(pow, smuad(y, y))
		q = y
		ax, ay := uint32((d^d>>31)-d>>31), uint32((c^c>>31)-c>>31)
		m := (c ^ d) >> 31
		sw := ay > ax
		if sw {
			ax, ay = ay, ax
		}
		z := uint(bits.LeadingZeros32(ax | 1))
		var a int32
		if iqLowRecip {
			x := ax << z
			t := ((ay << z) >> 16 * uint32(iqLowTab[x>>25])) >> 24
			a = int32(iqLowTab[t&63])
		} else if t := (ay << z) / ((ax<<z)>>16 + 1); iqLowAtanNear {
			a = int32(iqLowTab[t>>10&63])
		} else {
			w := iqLowAtan[t>>12&15]
			a = int32(w&0xFFFF) + int32(w>>16)*int32(t&0xFFF)>>12
		}
		if sw {
			a = 1<<14 - a
		}
		a = (a ^ m) - m
		out[i] = int16(smmulr((a<<16-step)^d&math.MinInt32, k))
		if i++; i == len(x) {
			break
		}
	}
	r.q = q
	r.ar, r.ai, r.pow = ar, ai, pow
}

//go:noinline
func (r *iqLowRx) follow() {
	s, c := sincosf(float32(r.step) * (-2 * math.Pi / (1 << 32)))
	acc := complex(float32(r.ar), float32(r.ai)) * complex(c, s)
	p := float32(r.pow)
	r.ar, r.ai, r.pow = 0, 0, 0
	rate := float32(r.d.hz())
	est := atan2f(imag(acc), real(acc)) * (rate / (2 * math.Pi))
	quiet := real(acc)*real(acc)+imag(acc)*imag(acc) < iq0Coherence*iq0Coherence*p*p
	switch {
	case quiet:
	case r.afcAvg == 0:
		r.afcAvg = acc
	default:
		r.afcAvg += complex(iqAFCAvgGain, 0) * (acc - r.afcAvg)
	}
	avg := atan2f(imag(r.afcAvg), real(r.afcAvg)) * (rate / (2 * math.Pi))
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	scale := f.gain * iqUnitsPerHz
	if r.d.locked {
		hz := (f.dc + r.d.dcHz) / scale
		switch {
		case abs32(avg) >= iqAFCPullHz && abs32(hz) >= iqAFCLockHz && hz*avg > 0:
			r.afcAvg = 0
			r.tune(avg, f.dc-avg*scale)
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < iqAFCPullHz:
			r.tune(hz, f.dc-hz*scale)
		}
		return
	}
	switch {
	case quiet:
	case abs32(est) >= iqAFCPullHz:
		r.afcAvg = 0
		r.tune(est, 0)
	case abs32(avg) >= iqAFCFineHz:
		r.tune(avg, 0)
	}
}

func (r *iqLowRx) tune(hz, dc float32) {
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	r.hz += hz
	rate := float32(r.d.hz())
	r.step = roundBits(r.hz/rate*(1<<16)) << 16
	s, c := sincosf(-2 * math.Pi * hz / rate)
	r.afcAvg *= complex(c, s)
	d := hz*scale + dc - f.dc
	if r.d.locked {
		f.mean += dc - f.dc
	} else {
		f.mean = dc
	}
	f.dc, r.d.tuneHz = dc, dc
	f.refresh()
	if !r.d.locked {
		f.shiftHist(d)
		r.d.shift(d)
	}
}

func (r *iqLowRx) flush() {
	if !r.d.iqLow || !r.d.front96 {
		r.d.write(r.buf[:r.n])
	}
	r.n = 0
	r.d.flush()
}
