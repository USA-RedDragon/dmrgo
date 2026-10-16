package main

import (
	"math"
	"math/bits"
)

// Limits of the channel stage.
const (
	// chanTaps is the length of the channel filter.
	chanTaps = 95
	// chanPassHz is the edge of the passband of the channel filter.
	chanPassHz = 7500.0
	// chanStopHz is the edge of its stopband.
	chanStopHz = 11000.0
	// discTaps is the length of the discriminator prefilter.
	discTaps = 31
	// discSpan is the number of symbols on each side of the matched
	// filter of the discriminator.
	discSpan = 4
)

// ring is a ring of complex samples addressed by absolute sample number,
// one sample every dec channel samples. It holds float samples in a
// power-of-two buffer, or, when compact, an exact number of 8-bit samples
// in blocks of qBlock with one exponent each, and the newest partial block
// as floats.
type ring struct {
	buf  []complex64
	mask int64
	n    int64
	dec  int
	// q holds the real and imaginary parts of each compact sample, ex the
	// exponent of each block, stage the samples of the partial block and w
	// the position in q of the first sample of the partial block.
	q     []int8
	ex    []int8
	stage [qBlock]complex64
	w     int
	// tab holds the interpolator taps for phases+1 fractional delays.
	tab    []float32
	phases int
}

// Layout of the rings.
const (
	// interpPhases is the number of fractional delays the interpolator of
	// a float ring has taps for, and compactPhases that of a compact ring.
	interpPhases  = 256
	compactPhases = 64
	// qBlock is the number of samples of a compact ring that share an
	// exponent.
	qBlock = 8
)

// newRing returns a float ring of at least size samples, or a compact ring
// of size rounded up to whole blocks, one sample every dec channel samples.
func newRing(size, dec int, compact bool) ring {
	r := ring{dec: dec, phases: interpPhases}
	if compact {
		size = (size + qBlock - 1) / qBlock * qBlock
		r.q, r.ex, r.phases = make([]int8, 2*size), make([]int8, size/qBlock), compactPhases
	} else {
		n := 1
		for n < size {
			n <<= 1
		}
		r.buf, r.mask = make([]complex64, n), int64(n-1)
	}
	r.tab = make([]float32, (r.phases+1)*sincTaps)
	for p := range r.phases + 1 {
		fr := float64(p) / float64(r.phases)
		for k := range sincTaps {
			u := float64(k-sincTaps/2+1) - fr
			w := 0.42 + 0.5*math.Cos(math.Pi*u/(sincTaps/2)) + 0.08*math.Cos(2*math.Pi*u/(sincTaps/2))
			if u != 0 {
				w *= math.Sin(math.Pi*u) / (math.Pi * u)
			}
			r.tab[p*sincTaps+k] = float32(w)
		}
	}
	return r
}

// size returns the number of samples the ring holds when full.
func (r *ring) size() int {
	if r.q != nil {
		return len(r.q) / 2
	}
	return len(r.buf)
}

// push appends sample v.
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

// commit quantizes the block b into the compact block at position pos:
// the largest part of a sample gets 7 significant bits.
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

// pos returns the position in q of committed sample i, or -1 when the ring
// does not hold it.
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

// sample returns the compact sample at position k of q.
func (r *ring) sample(k int) complex64 {
	g := pow2(int32(r.ex[k/qBlock]))
	return complex(float32(r.q[2*k])*g, float32(r.q[2*k+1])*g)
}

// at returns sample i, or 0 outside the samples the ring holds.
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

// first returns the oldest sample the ring holds when full, negative before
// it fills: a compact ring holds whole blocks.
func (r *ring) first() int64 {
	if r.q != nil {
		return r.n&^(qBlock-1) - int64(r.size())
	}
	return r.n - int64(r.size())
}

// rotate multiplies every sample the ring holds, sample i by
// rot*step^(i-first) where first is max(r.first(), 0).
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

// ringAt returns the ring position of channel sample t as base+u, with u
// from 0 to 1 at t and base pad ring samples lower, so that the positions
// of a run of samples from t and up to pad ring samples before it have a
// non-negative u.
func (r *ring) ringAt(t float64, pad int) (base int64, u float32) {
	t /= float64(r.dec)
	i0 := int64(math.Floor(t))
	return i0 - int64(pad), float32(t-float64(i0)) + float32(pad)
}

// interpIn returns x, samples of the ring, at the fractional index u by a
// Blackman windowed sinc of sincTaps samples, at the nearest of the phases
// fractional delays. Samples outside x count as 0.
func (r *ring) interpIn(x []complex64, u float32) complex64 {
	iu := int(u)
	if u < 0 {
		iu--
	}
	p := int((u-float32(iu))*float32(r.phases) + 0.5)
	taps := r.tab[p*sincTaps : (p+1)*sincTaps]
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

// gather writes the samples from first into x, 0 outside the samples the
// ring holds.
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

// fring is a ring of real samples addressed by absolute sample number.
type fring struct {
	buf []float32
	n   int64
	w   int
}

// newFring returns a ring of size samples.
func newFring(size int) fring {
	return fring{buf: make([]float32, size)}
}

// push appends sample v.
func (r *fring) push(v float32) {
	r.buf[r.w] = v
	r.n++
	if r.w++; r.w == len(r.buf) {
		r.w = 0
	}
}

// at returns sample i, or 0 outside the samples the ring holds.
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

// qring is a ring of real samples addressed by absolute sample number, each
// held as an int8 multiple of step, saturated.
type qring struct {
	buf  []int8
	n    int64
	w    int
	step float32
	inv  float32
	// off is added to every sample the ring holds.
	off float32
}

// newQring returns a ring of size samples of resolution step.
func newQring(size int, step float32) qring {
	return qring{buf: make([]int8, size), step: step, inv: 1 / step}
}

// quantize returns v in units of step, rounded and saturated.
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

// push appends sample v.
func (r *qring) push(v float32) {
	r.buf[r.w] = r.quantize(v - r.off)
	r.n++
	if r.w++; r.w == len(r.buf) {
		r.w = 0
	}
}

// at returns sample i, or 0 outside the samples the ring holds.
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

// gather writes samples base, base+stride, ... into x, 0 outside the
// samples the ring holds.
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

// add adds v to every sample the ring holds: the whole steps go into the
// samples and the rest into the offset of the ring, so that each sample
// keeps its value to within its own rounding.
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

// lerp returns the ring at the fractional sample t by linear interpolation.
func (r *qring) lerp(t float64) float32 {
	base := int64(math.Floor(t))
	return r.lerpFrom(base, float32(t-float64(base)))
}

// lerpFrom returns the ring at the fractional sample base+u, u at least 0,
// by linear interpolation, in single precision.
func (r *qring) lerpFrom(base int64, u float32) float32 {
	iu := int32(u)
	fr := u - float32(iu)
	i := base + int64(iu)
	return r.at(i)*(1-fr) + r.at(i+1)*fr
}

// Layout of the discriminator path.
const (
	// energyBin is the number of channel samples whose power the
	// discriminator path sums into one bin of its energy ring.
	energyBin = 40
	// discStepHz is the resolution of the discriminator samples that the
	// ring holds.
	discStepHz = 64
)

// channelizer moves the carrier to 0 Hz with an oscillator and filters the
// channel. For coherent detection it runs a wide filter into ch. For the
// discriminator tiers it runs a narrower prefilter at basebandRate/discDec,
// sums the power of its output into energy, and runs the FM discriminator
// and the RRC matched filter into sym (Hz of deviation), discDec times
// decimated. Every output has the delay of its causal filters: channel
// sample t, the time base of the receiver, is input sample t-delayCh, and
// symIndex maps it to sym.
type channelizer struct {
	disc    bool
	n       int64
	ncoHz   float64
	ncoStep complex64
	ncoRot  complex64
	delayCh int

	wide    ring
	hist    []complex64
	head    int
	chTaps  []float32
	ch      ring
	chDec   int
	compact bool

	discDec int
	dTaps   []float32
	dHist   []complex64
	dHead   int
	prev    complex64
	rrc     []float32
	fHist   []float32
	fHead   int
	sym     qring
	energy  fring
	eAcc    float32
	discHz  float32
	// decLeft and binLeft count the input samples to the next prefilter
	// output and to the end of the next energy bin.
	decLeft int
	binLeft int
}

// newChannelizer returns a channelizer whose rings hold size channel
// samples, with the discriminator path at basebandRate/dec when disc, its
// prefilter cut off at cutHz, and the coherent channel ring at
// basebandRate/dec otherwise, compact when compact.
func newChannelizer(size int, disc bool, cutHz float64, dec int, compact bool) *channelizer {
	c := &channelizer{disc: disc, ncoRot: 1, ncoStep: 1, delayCh: chanTaps / 2}
	if !disc {
		c.chDec, c.compact = dec, compact
		if !compact {
			c.wide = newRing(size, 1, false)
		}
		c.hist = make([]complex64, 2*chanTaps)
		c.chTaps = kaiserLowpass(chanTaps, (chanPassHz+chanStopHz)/2/basebandRate, 7)
		c.ch = newRing((size+dec-1)/dec, dec, compact)
		return c
	}
	c.discDec = dec
	c.dHist = make([]complex64, 2*discTaps)
	c.dTaps = kaiserLowpass(discTaps, cutHz/basebandRate, 5)
	r := rrcTapsSpan(float64(samplesPerSy)/float64(dec), discSpan)
	c.rrc = make([]float32, len(r))
	for i, v := range r {
		c.rrc[i] = float32(v)
	}
	c.fHist = make([]float32, 2*len(r))
	c.sym = newQring(size/dec+1, discStepHz)
	c.energy = newFring(size/energyBin + 2)
	c.discHz = float32(basebandRate / float64(dec) / (2 * math.Pi))
	c.decLeft = dec
	c.binLeft = (energyBin-(1+c.delayCh-discTaps/2)%energyBin)%energyBin + 1
	return c
}

// count returns the number of channel samples so far.
func (c *channelizer) count() int64 {
	return c.n
}

// oldest returns the first channel sample that every ring still holds,
// negative before the rings fill.
func (c *channelizer) oldest() int64 {
	if c.disc {
		return c.n - int64(min(len(c.sym.buf)*c.discDec, (len(c.energy.buf)-1)*energyBin))
	}
	return c.ch.first() * int64(c.chDec)
}

// symAt returns the sym sample of channel sample t as base+u, u from 0 to
// 1, from the integers of symIndex: twice the index is q over 2*discDec, a
// power of two.
func (c *channelizer) symAt(t int64) (base int64, u float32) {
	d2 := 2 * c.discDec
	q := 2*(t-int64(c.delayCh)+discTaps/2+discSpan*samplesPerSy+1) - int64(c.discDec)
	sh := uint(bits.TrailingZeros(uint(d2)))
	return q >> sh, float32(q&int64(d2-1)) / float32(d2)
}

// symIndex returns the fractional sym sample of channel sample t: the
// prefilter output of input sample m*dec+dec-1 has its center discTaps/2
// samples earlier, the discriminator output half a step before that, and
// the matched filter output rrcSpan symbols before that.
func (c *channelizer) symIndex(t float64) float64 {
	d := float64(c.discDec)
	return (t - float64(c.delayCh) + discTaps/2 + discSpan*samplesPerSy + 1 - d/2) / d
}

// symStep returns the number of sym samples per symbol.
func (c *channelizer) symStep() float32 {
	return float32(samplesPerSy) / float32(c.discDec)
}

// slotEnergyAt returns the mean power of the discriminator prefilter output
// over the n channel samples from a, to the nearest bins of the energy ring.
func (c *channelizer) slotEnergyAt(a float64, n int) float64 {
	b0 := int64(math.Round(a / energyBin))
	b1 := int64(math.Round((a + float64(n)) / energyBin))
	var e float64
	for b := b0; b < b1; b++ {
		e += float64(c.energy.at(b))
	}
	return e / float64(max(b1-b0, 1)*energyBin/int64(c.discDec))
}

// kaiserLowpass returns a Kaiser windowed sinc low-pass of n taps with
// cutoff fc (a fraction of the sample rate) and unity DC gain.
func kaiserLowpass(n int, fc, beta float64) []float32 {
	h := make([]float32, n)
	var sum float64
	v := make([]float64, n)
	for i := range n {
		m := float64(i) - float64(n-1)/2
		x := 2 * fc
		if m != 0 {
			x = math.Sin(2*math.Pi*fc*m) / (math.Pi * m)
		}
		r := 2*float64(i)/float64(n-1) - 1
		v[i] = x * bessel0(beta*math.Sqrt(max(0, 1-r*r))) / bessel0(beta)
		sum += v[i]
	}
	for i := range h {
		h[i] = float32(v[i] / sum)
	}
	return h
}

// tune sets the oscillator to hz, the carrier offset to remove. The coherent
// path filters again the wide samples the ring still holds, so that every
// sample of the channel ring has the same carrier; a compact one instead
// rotates the samples of the channel ring and the filter history by the
// change of frequency. The discriminator path keeps the phase of the
// oscillator and shifts the discriminator samples it holds by the change of
// frequency.
func (c *channelizer) tune(hz float64) {
	p := -2 * math.Pi * hz / basebandRate
	c.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	if c.disc {
		d := float32(c.ncoHz - hz)
		c.ncoHz = hz
		c.sym.add(d)
		for i := range c.fHist {
			c.fHist[i] += d
		}
		return
	}
	if c.compact {
		c.retune(hz)
		return
	}
	c.ncoHz = hz
	n := c.wide.n
	from := max(n-int64(len(c.wide.buf)), 0)
	c.ch.n = (from + int64(c.chDec) - 1) / int64(c.chDec)
	clear(c.hist)
	for i := from; i < n; i++ {
		c.filter(c.wide.at(i), i)
	}
}

// retune moves the compact coherent path to the oscillator at hz: input
// sample i, mixed at the old frequency, turns by -2*pi*(hz-ncoHz)*i, and a
// channel sample by the turn at the center of its filter.
func (c *channelizer) retune(hz float64) {
	w := -2 * math.Pi * (hz - c.ncoHz) / basebandRate
	turn := func(i int64) complex64 {
		p := math.Mod(w*float64(i), 2*math.Pi)
		return complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	first := max(c.ch.first(), 0)
	c.ch.rotate(turn(first*int64(c.chDec)-int64(c.delayCh)), turn(int64(c.chDec)))
	for k := range chanTaps {
		j := (c.head + k) % chanTaps
		r := turn(c.n - chanTaps + int64(k))
		c.hist[j] *= r
		c.hist[j+chanTaps] = c.hist[j]
	}
	c.ncoHz = hz
	p := math.Mod(-2*math.Pi*hz*float64(c.n)/basebandRate, 2*math.Pi)
	c.ncoRot = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

// push filters one wide baseband sample.
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

// filter runs the wide sample v, number n, through the oscillator and, for
// every chDec samples, the channel filter.
func (c *channelizer) filter(v complex64, n int64) {
	if n&1023 == 0 {
		p := -2 * math.Pi * c.ncoHz * float64(n) / basebandRate
		c.ncoRot = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	}
	v *= c.ncoRot
	c.ncoRot *= c.ncoStep
	c.hist[c.head] = v
	c.hist[c.head+chanTaps] = v
	c.head = (c.head + 1) % chanTaps
	if n&int64(c.chDec-1) == 0 {
		c.ch.push(fir(c.hist[c.head:c.head+chanTaps], c.chTaps))
	}
}

// discFilter runs the wide sample v through the oscillator, and every
// discDec samples through the prefilter, the discriminator and the matched
// filter.
func (c *channelizer) discFilter(v complex64) {
	n := c.n
	c.n++
	c.binLeft--
	if n&1023 == 0 {
		g := (3 - real(c.ncoRot)*real(c.ncoRot) - imag(c.ncoRot)*imag(c.ncoRot)) / 2
		c.ncoRot *= complex(g, 0)
	}
	v *= c.ncoRot
	c.ncoRot *= c.ncoStep
	c.dHist[c.dHead] = v
	c.dHist[c.dHead+discTaps] = v
	if c.dHead++; c.dHead == discTaps {
		c.dHead = 0
	}
	if c.decLeft--; c.decLeft > 0 {
		return
	}
	c.decLeft = c.discDec
	y := fir(c.dHist[c.dHead:c.dHead+discTaps], c.dTaps)
	c.eAcc += real(y)*real(y) + imag(y)*imag(y)
	if c.binLeft <= 0 {
		c.energy.push(c.eAcc)
		c.eAcc = 0
		c.binLeft += energyBin
	}
	d := y * complex(real(c.prev), -imag(c.prev))
	c.prev = y
	f := atan2f(imag(d), real(d)) * c.discHz
	nr := len(c.rrc)
	c.fHist[c.fHead] = f
	c.fHist[c.fHead+nr] = f
	if c.fHead++; c.fHead == nr {
		c.fHead = 0
	}
	var s float32
	for k, h := range c.rrc {
		s += c.fHist[c.fHead+k] * h
	}
	c.sym.push(s)
}

// fir returns the dot product of the history x (oldest first) and the
// symmetric taps h.
func fir(x []complex64, h []float32) complex64 {
	var re, im float32
	for k, t := range h {
		re += real(x[k]) * t
		im += imag(x[k]) * t
	}
	return complex(re, im)
}
