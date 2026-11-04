package rf

const disc48Out = 32

type disc48 struct {
	phase uint8
	head  uint8
	hist  [halfTaps]int16
	buf   [disc48Out]int16
}

func (d *disc48) push(v int16) (int16, bool) {
	if d.phase == 0 {
		d.phase = 2
		for i := range d.hist {
			d.hist[i] = v
		}
	}
	h := int(d.head)
	d.hist[h] = v
	if h++; h == halfTaps {
		h = 0
	}
	d.head = uint8(h)
	if d.phase ^= 3; d.phase == 1 {
		return 0, false
	}
	const c = halfTaps / 2
	acc := int32(discHalfQ15[0]) * int32(d.hist[ringAt(h, c)])
	for k, t := range discHalfQ15[1:] {
		acc += int32(t) * (int32(d.hist[ringAt(h, c-2*k-1)]) + int32(d.hist[ringAt(h, c+2*k+1)]))
	}
	return int16(max(min((acc+1<<14)>>15, 32767), -32768)), true
}

func (r *AudioReceiver) input(x []int16) {
	d := &r.d48
	if !r.f.dec {
		if r.pe != nil {
			r.emitCopy(x)
			return
		}
		r.write(x)
		return
	}
	n := 0
	for _, v := range x {
		y, ok := d.push(v)
		if !ok {
			continue
		}
		d.buf[n] = y
		if n++; n == disc48Out {
			r.emit(d.buf[:])
			n = 0
		}
	}
	if n > 0 {
		r.emit(d.buf[:n])
	}
}

func (r *discRx) inputSamples(n int) int {
	if r.f.dec {
		return (n + 1) / 2
	}
	return n
}
