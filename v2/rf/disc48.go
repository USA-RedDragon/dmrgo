package rf

const disc48Out = 32

type disc48 struct {
	on   bool
	odd  bool
	head int
	hist [halfTaps]int16
	buf  [disc48Out]int16
}

func (d *disc48) push(v int16) (int16, bool) {
	h := d.head
	d.hist[h] = v
	if h++; h == halfTaps {
		h = 0
	}
	d.head = h
	if d.odd = !d.odd; d.odd {
		return 0, false
	}
	const c = halfTaps / 2
	acc := int32(discHalfQ15[0]) * int32(d.hist[ringAt(h, c)])
	for k, t := range discHalfQ15[1:] {
		acc += int32(t) * (int32(d.hist[ringAt(h, c-2*k-1)]) + int32(d.hist[ringAt(h, c+2*k+1)]))
	}
	return int16(max(min((acc+1<<14)>>15, 32767), -32768)), true
}

func (r *DiscReceiver) input(x []int16) {
	d := &r.d48
	if !d.on {
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
			r.write(d.buf[:])
			n = 0
		}
	}
	if n > 0 {
		r.write(d.buf[:n])
	}
}

func (r *DiscReceiver) inputSamples(n int) int {
	if r.d48.on {
		return (n + 1) / 2
	}
	return n
}
