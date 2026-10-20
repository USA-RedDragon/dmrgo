package rf

import (
	"math"
)

const (
	chanTaps   = 95
	chanPassHz = 7500.0
	chanStopHz = 11000.0
	discCutHz  = 4500.0
)

type ring struct {
	buf          []complex64
	mask         int64
	n            int64
	dec          int
	q            []int8
	ex           []int8
	stage        [qBlock]complex64
	w            int
	phases, rows int
}

const (
	interpPhases  = 256
	compactPhases = 64
	qBlock        = 8
)

func newRing(size, dec int, compact bool) ring {
	r := ring{dec: dec, phases: interpPhases, rows: 1}
	if compact {
		size = (size + qBlock - 1) / qBlock * qBlock
		r.q, r.ex = make([]int8, 2*size), make([]int8, size/qBlock)
		r.phases, r.rows = compactPhases, interpPhases/compactPhases
	} else {
		n := 1
		for n < size {
			n <<= 1
		}
		r.buf, r.mask = make([]complex64, n), int64(n-1)
	}
	return r
}

func (r *ring) size() int {
	if r.q != nil {
		return len(r.q) / 2
	}
	return len(r.buf)
}

func (r *ring) push(v complex64) {
	if r.q == nil {
		r.buf[r.n&r.mask] = v
		r.n++
		return
	}
	r.stage[r.n&(qBlock-1)] = v
	r.n++
	if r.n&(qBlock-1) == 0 {
		r.commit(r.w, &r.stage)
		if r.w += qBlock; r.w == len(r.ex)*qBlock {
			r.w = 0
		}
	}
}

func (r *ring) commit(pos int, b *[qBlock]complex64) {
	var m float32
	for _, v := range b {
		m = max(m, abs32(real(v)), abs32(imag(v)))
	}
	e := int32(-64)
	if m > 0 {
		e = max(int32(math.Float32bits(m)>>23&0xff)-127-6, -64)
	}
	inv := pow2(-e)
	r.ex[pos/qBlock] = int8(e)
	for k, v := range b {
		r.q[2*(pos+k)] = sat8(real(v) * inv)
		r.q[2*(pos+k)+1] = sat8(imag(v) * inv)
	}
}

func (r *ring) pos(i int64) int {
	end := r.n &^ (qBlock - 1)
	d := end - i
	if i < 0 || d <= 0 || d > int64(len(r.ex)*qBlock) {
		return -1
	}
	k := r.w - int(d)
	if k < 0 {
		k += len(r.ex) * qBlock
	}
	return k
}

func (r *ring) sample(k int) complex64 {
	g := pow2(int32(r.ex[k/qBlock]))
	return complex(float32(r.q[2*k])*g, float32(r.q[2*k+1])*g)
}

func (r *ring) at(i int64) complex64 {
	if r.q != nil {
		if i >= r.n&^(qBlock-1) && i < r.n {
			return r.stage[i&(qBlock-1)]
		}
		if k := r.pos(i); k >= 0 {
			return r.sample(k)
		}
		return 0
	}
	if i < 0 || i >= r.n || i < r.n-int64(len(r.buf)) {
		return 0
	}
	return r.buf[i&r.mask]
}

func (r *ring) first() int64 {
	if r.q != nil {
		return r.n&^(qBlock-1) - int64(r.size())
	}
	return r.n - int64(r.size())
}

func (r *ring) rotate(rot, step complex64) {
	first := max(r.first(), 0)
	end := r.n &^ (qBlock - 1)
	if r.q == nil {
		for i := first; i < r.n; i++ {
			r.buf[i&r.mask] *= rot
			rot *= step
		}
		return
	}
	var b [qBlock]complex64
	for i := first; i < end; i += qBlock {
		k := r.pos(i)
		for j := range b {
			b[j] = r.sample(k+j) * rot
			rot *= step
		}
		r.commit(k, &b)
		g := (3 - real(rot)*real(rot) - imag(rot)*imag(rot)) / 2
		rot *= complex(g, 0)
	}
	for i := end; i < r.n; i++ {
		r.stage[i&(qBlock-1)] *= rot
		rot *= step
	}
}

func (r *ring) ringAt(t float64, pad int) (base int64, u float32) {
	t /= float64(r.dec)
	i0 := int64(math.Floor(t))
	return i0 - int64(pad), float32(t-float64(i0)) + float32(pad)
}

func (r *ring) interpIn(x []complex64, u float32) complex64 {
	iu := int(u)
	if u < 0 {
		iu--
	}
	p := int((u-float32(iu))*float32(r.phases) + 0.5)
	taps := &interpTaps[p*r.rows]
	lo := iu - sincTaps/2 + 1
	var re, im float32
	if lo >= 0 && lo+sincTaps <= len(x) {
		w := x[lo : lo+sincTaps]
		for k, t := range taps {
			re += real(w[k]) * t
			im += imag(w[k]) * t
		}
		return complex(re, im)
	}
	for k, t := range taps {
		if j := lo + k; j >= 0 && j < len(x) {
			re += real(x[j]) * t
			im += imag(x[j]) * t
		}
	}
	return complex(re, im)
}

func (r *ring) gather(first int64, x []complex64) {
	n := int64(len(x))
	if r.q == nil {
		if first >= 0 && first >= r.n-int64(len(r.buf)) && first+n <= r.n {
			for j := range x {
				x[j] = r.buf[(first+int64(j))&r.mask]
			}
			return
		}
		for j := range x {
			x[j] = r.at(first + int64(j))
		}
		return
	}
	if first < 0 || first+n > r.n&^(qBlock-1) {
		for j := range x {
			x[j] = r.at(first + int64(j))
		}
		return
	}
	k := r.pos(first)
	if k < 0 || k+len(x) > len(r.ex)*qBlock {
		for j := range x {
			x[j] = r.at(first + int64(j))
		}
		return
	}
	for j := 0; j < len(x); {
		b := (k + j) / qBlock
		end := min(len(x), (b+1)*qBlock-k)
		g := pow2(int32(r.ex[b]))
		for ; j < end; j++ {
			x[j] = complex(float32(r.q[2*(k+j)])*g, float32(r.q[2*(k+j)+1])*g)
		}
	}
}

type fring struct {
	buf []float32
	n   int64
	w   int
}

func newFring(size int) fring {
	return fring{buf: make([]float32, size)}
}

func (r *fring) push(v float32) {
	r.buf[r.w] = v
	r.n++
	if r.w++; r.w == len(r.buf) {
		r.w = 0
	}
}

func (r *fring) at(i int64) float32 {
	d := r.n - i
	if i < 0 || d <= 0 || d > int64(len(r.buf)) {
		return 0
	}
	k := r.w - int(d)
	if k < 0 {
		k += len(r.buf)
	}
	return r.buf[k]
}

type qring struct {
	buf  []int8
	n    int64
	w    int
	step float32
	inv  float32
	off  float32
}

func newQring(size int, step float32) qring {
	return qring{buf: make([]int8, size), step: step, inv: 1 / step}
}

func (r *qring) quantize(v float32) int8 {
	q := v*r.inv + 0.5
	switch {
	case q >= math.MaxInt8:
		return math.MaxInt8
	case q <= math.MinInt8:
		return math.MinInt8
	}
	i := int8(q)
	if float32(i) > q {
		i--
	}
	return i
}

func (r *qring) push(v float32) {
	r.buf[r.w] = r.quantize(v - r.off)
	r.n++
	if r.w++; r.w == len(r.buf) {
		r.w = 0
	}
}

func (r *qring) at(i int64) float32 {
	d := r.n - i
	if i < 0 || d <= 0 || d > int64(len(r.buf)) {
		return 0
	}
	k := r.w - int(d)
	if k < 0 {
		k += len(r.buf)
	}
	return float32(r.buf[k])*r.step + r.off
}

func (r *qring) gather(base int64, stride int, x []float32) {
	last := base + int64(stride*(len(x)-1))
	if base < 0 || base < r.n-int64(len(r.buf)) || last >= r.n {
		for k := range x {
			x[k] = r.at(base + int64(k*stride))
		}
		return
	}
	k0 := r.w - int(r.n-base)
	if k0 < 0 {
		k0 += len(r.buf)
	}
	for k := range x {
		x[k] = float32(r.buf[k0])*r.step + r.off
		if k0 += stride; k0 >= len(r.buf) {
			k0 -= len(r.buf)
		}
	}
}

func (r *qring) add(v float32) {
	r.off += v
	k := int32(r.quantize(r.off))
	if k == 0 {
		return
	}
	r.off -= float32(k) * r.step
	for i, q := range r.buf {
		r.buf[i] = int8(max(min(int32(q)+k, math.MaxInt8), math.MinInt8))
	}
}

func (r *qring) lerp(t float64) float32 {
	base := int64(math.Floor(t))
	return r.lerpFrom(base, float32(t-float64(base)))
}

func (r *qring) lerpFrom(base int64, u float32) float32 {
	iu := int32(u)
	fr := u - float32(iu)
	i := base + int64(iu)
	return r.at(i)*(1-fr) + r.at(i+1)*fr
}

const (
	energyBin  = 40
	discStepHz = 64
)

type channelizer struct {
	disc    bool
	n       int64
	ncoHz   float64
	ncoStep complex64
	ncoRot  complex64
	delayCh int

	wide    ring
	taps    []float32
	alt     []float32
	hist    []complex64
	head    int
	ch      ring
	chDec   int
	compact bool

	df     *discFront
	sym    qring
	energy fring
	step   int64
	eAcc   float32
	eBin   int64
}

func newChannelizer(size int, disc bool, dec int, compact, half bool) *channelizer {
	c := &channelizer{disc: disc, ncoRot: 1, ncoStep: 1, delayCh: chanTaps / 2, step: 1}
	if !disc {
		c.chDec, c.compact = dec, compact
		if !compact {
			c.wide = newRing(size, 1, false)
		}
		c.taps = chanFilterTaps[:]
		c.hist = make([]complex64, 2*chanTaps)
		c.ch = newRing((size+dec-1)/dec, dec, compact)
		return c
	}
	c.df = newDiscFront(half)
	if half {
		c.step = 2
	}
	c.sym = newQring(size/2+1, discStepHz)
	c.energy = newFring(size/energyBin + 2)
	return c
}

func (c *channelizer) setFilter(n int, beta, cutHz float64) {
	if n == 0 {
		n = chanTaps
	}
	if cutHz == 0 {
		cutHz = (chanPassHz + chanStopHz) / 2
	}
	w := windowTaps(n, cutHz/basebandRate, beta)
	var sum float64
	for _, v := range w {
		sum += v
	}
	c.taps = make([]float32, n)
	for i, v := range w {
		c.taps[i] = float32(v / sum)
	}
	c.hist = make([]complex64, 2*n)
	c.head, c.delayCh = 0, n/2
}

func (c *channelizer) useAlt(on bool) bool {
	t := chanFilterTaps[:]
	if on {
		t = c.alt
	}
	if &t[0] == &c.taps[0] {
		return false
	}
	c.taps = t
	if c.compact {
		return true
	}
	c.refilter()
	return true
}

func (c *channelizer) count() int64 {
	return c.n
}

func (c *channelizer) oldest() int64 {
	if c.disc {
		return c.n - int64(min(len(c.sym.buf)*2, (len(c.energy.buf)-1)*energyBin))
	}
	return c.ch.first() * int64(c.chDec)
}

func (c *channelizer) symAt(t int64) (base int64, u float32) {
	q := t - int64(c.delayCh) - c.df.center()
	return q >> 1, float32(q&1) / 2
}

func (c *channelizer) symIndex(t float64) float64 {
	return (t - float64(c.delayCh) - float64(c.df.center())) / 2
}

func (c *channelizer) symStep() float32 {
	return discSPS
}

func (c *channelizer) slotEnergyAt(a float64, n int) float64 {
	b0 := int64(math.Round(a / energyBin))
	b1 := int64(math.Round((a + float64(n)) / energyBin))
	var e float64
	for b := b0; b < b1; b++ {
		e += float64(c.energy.at(b))
	}
	return e / float64(max(b1-b0, 1)*energyBin/2)
}

func (c *channelizer) tune(hz float64) {
	p := -2 * math.Pi * hz / basebandRate
	c.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	if c.disc {
		d := float32(c.ncoHz - hz)
		c.ncoHz = hz
		c.df.tune(hz)
		c.sym.add(d)
		for i := range c.df.fHist {
			c.df.fHist[i] += d
		}
		return
	}
	if c.compact {
		c.retune(hz)
		return
	}
	c.ncoHz = hz
	c.refilter()
}

func (c *channelizer) refilter() {
	n := c.wide.n
	from := max(n-int64(len(c.wide.buf)), 0)
	c.ch.n = (from + int64(c.chDec) - 1) / int64(c.chDec)
	clear(c.hist)
	p := -2 * math.Pi * c.ncoHz * float64(from) / basebandRate
	c.ncoRot = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	for i := from; i < n; i++ {
		c.filter(c.wide.at(i), i)
	}
}

func (c *channelizer) retune(hz float64) {
	w := -2 * math.Pi * (hz - c.ncoHz) / basebandRate
	turn := func(i int64) complex64 {
		p := math.Mod(w*float64(i), 2*math.Pi)
		return complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	first := max(c.ch.first(), 0)
	c.ch.rotate(turn(first*int64(c.chDec)-int64(c.delayCh)), turn(int64(c.chDec)))
	nt := len(c.taps)
	for k := range nt {
		j := (c.head + k) % nt
		r := turn(c.n - int64(nt) + int64(k))
		c.hist[j] *= r
		c.hist[j+nt] = c.hist[j]
	}
	c.ncoHz = hz
	p := math.Mod(-2*math.Pi*hz*float64(c.n)/basebandRate, 2*math.Pi)
	c.ncoRot = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

func (c *channelizer) push(v complex64) {
	if c.disc {
		c.discFilter(v)
		return
	}
	n := c.n
	c.n++
	if !c.compact {
		c.wide.push(v)
	}
	c.filter(v, n)
}

func (c *channelizer) filter(v complex64, n int64) {
	if n&1023 == 0 {
		p := -2 * math.Pi * c.ncoHz * float64(n) / basebandRate
		c.ncoRot = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	v *= c.ncoRot
	c.ncoRot *= c.ncoStep
	nt := len(c.taps)
	c.hist[c.head] = v
	c.hist[c.head+nt] = v
	if c.head++; c.head == nt {
		c.head = 0
	}
	if n&int64(c.chDec-1) == 0 {
		c.ch.push(fir(c.hist[c.head:c.head+nt], c.taps))
	}
}

func (c *channelizer) discFilter(v complex64) {
	c.n += c.step
	s, p, ok := c.df.push(v)
	if !ok {
		return
	}
	t := 2*c.sym.n + c.df.center() + discSpan*samplesPerSy + 1 + int64(c.delayCh)
	if t >= 0 {
		for t >= (c.eBin+1)*energyBin {
			c.energy.push(c.eAcc)
			c.eAcc = 0
			c.eBin++
		}
		c.eAcc += p
	}
	c.sym.push(s)
}

func fir(x []complex64, h []float32) complex64 {
	var re, im float32
	for k, t := range h {
		re += real(x[k]) * t
		im += imag(x[k]) * t
	}
	return complex(re, im)
}
