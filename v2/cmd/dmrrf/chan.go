package main

import "math"

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

// ring is a power-of-two ring of complex samples addressed by absolute
// sample number.
type ring struct {
	buf  []complex64
	mask int64
	n    int64
	// tab holds the interpolator taps for interpPhases fractional delays.
	tab []float32
}

// interpPhases is the number of fractional delays the interpolator has
// taps for.
const interpPhases = 256

// newRing returns a ring of at least size samples.
func newRing(size int) ring {
	n := 1
	for n < size {
		n <<= 1
	}
	r := ring{buf: make([]complex64, n), mask: int64(n - 1), tab: make([]float32, (interpPhases+1)*sincTaps)}
	for p := range interpPhases + 1 {
		fr := float64(p) / interpPhases
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

// push appends sample v.
func (r *ring) push(v complex64) {
	r.buf[r.n&r.mask] = v
	r.n++
}

// at returns sample i, or 0 outside the samples the ring holds.
func (r *ring) at(i int64) complex64 {
	if i < 0 || i >= r.n || i < r.n-int64(len(r.buf)) {
		return 0
	}
	return r.buf[i&r.mask]
}

// interp returns the ring at the fractional sample t by a Blackman windowed
// sinc of sincTaps samples, at the nearest of interpPhases fractional
// delays.
func (r *ring) interp(t float64) complex64 {
	i0 := int64(math.Floor(t))
	p := int((t-float64(i0))*interpPhases + 0.5)
	taps := r.tab[p*sincTaps : (p+1)*sincTaps]
	lo := i0 - sincTaps/2 + 1
	var re, im float32
	if lo >= 0 && lo >= r.n-int64(len(r.buf)) && lo+sincTaps <= r.n {
		for k, w := range taps {
			v := r.buf[(lo+int64(k))&r.mask]
			re += real(v) * w
			im += imag(v) * w
		}
		return complex(re, im)
	}
	for k, w := range taps {
		v := r.at(lo + int64(k))
		re += real(v) * w
		im += imag(v) * w
	}
	return complex(re, im)
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

// add adds v to every sample the ring holds.
func (r *fring) add(v float32) {
	for i := range r.buf {
		r.buf[i] += v
	}
}

// lerp returns the ring at the fractional sample t by linear interpolation.
func (r *fring) lerp(t float64) float32 {
	i0 := int64(math.Floor(t))
	fr := float32(t - float64(i0))
	return r.at(i0)*(1-fr) + r.at(i0+1)*fr
}

// Layout of the discriminator path.
const (
	// energyBin is the number of channel samples whose power the
	// discriminator path sums into one bin of its energy ring.
	energyBin = 40
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

	wide   ring
	hist   []complex64
	head   int
	chTaps []float32
	ch     ring

	discDec int
	dTaps   []float32
	dHist   []complex64
	dHead   int
	prev    complex64
	rrc     []float32
	fHist   []float32
	fHead   int
	sym     fring
	energy  fring
	eAcc    float32
	discHz  float32
}

// newChannelizer returns a channelizer whose rings hold size channel
// samples, with the discriminator path at basebandRate/dec when disc, its
// prefilter cut off at cutHz.
func newChannelizer(size int, disc bool, cutHz float64, dec int) *channelizer {
	c := &channelizer{disc: disc, ncoRot: 1, ncoStep: 1, delayCh: chanTaps / 2}
	if !disc {
		c.wide = newRing(size)
		c.hist = make([]complex64, 2*chanTaps)
		c.chTaps = kaiserLowpass(chanTaps, (chanPassHz+chanStopHz)/2/basebandRate, 7)
		c.ch = newRing(size)
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
	c.sym = newFring(size/dec + 1)
	c.energy = newFring(size/energyBin + 2)
	c.discHz = float32(basebandRate / float64(dec) / (2 * math.Pi))
	return c
}

// count returns the number of channel samples so far.
func (c *channelizer) count() int64 {
	if c.disc {
		return c.n
	}
	return c.ch.n
}

// oldest returns the first channel sample that every ring still holds,
// negative before the rings fill.
func (c *channelizer) oldest() int64 {
	if c.disc {
		return c.n - int64(min(len(c.sym.buf)*c.discDec, (len(c.energy.buf)-1)*energyBin))
	}
	return c.ch.n - int64(len(c.ch.buf))
}

// symIndex returns the fractional sym sample of channel sample t: the
// prefilter output of input sample m*dec+dec-1 has its center discTaps/2
// samples earlier, the discriminator output half a step before that, and
// the matched filter output rrcSpan symbols before that.
func (c *channelizer) symIndex(t float64) float64 {
	d := float64(c.discDec)
	return (t - float64(c.delayCh) + discTaps/2 + discSpan*samplesPerSy + 1 - d/2) / d
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
// sample of the channel ring has the same carrier. The discriminator path
// keeps the phase of the oscillator and shifts the discriminator samples it
// holds by the change of frequency.
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
	c.ncoHz = hz
	n := c.wide.n
	from := max(n-int64(len(c.wide.buf)), 0)
	c.ch.n = from
	clear(c.hist)
	for i := from; i < n; i++ {
		c.filter(c.wide.at(i), i)
	}
}

// push filters one wide baseband sample.
func (c *channelizer) push(v complex64) {
	if c.disc {
		c.discFilter(v)
		return
	}
	n := c.wide.n
	c.wide.push(v)
	c.filter(v, n)
}

// filter runs the wide sample v, number n, through the oscillator and the
// channel filter.
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
	c.ch.push(fir(c.hist[c.head:c.head+chanTaps], c.chTaps))
}

// discFilter runs the wide sample v through the oscillator, and every
// discDec samples through the prefilter, the discriminator and the matched
// filter.
func (c *channelizer) discFilter(v complex64) {
	n := c.n
	c.n++
	if n&1023 == 0 {
		g := (3 - real(c.ncoRot)*real(c.ncoRot) - imag(c.ncoRot)*imag(c.ncoRot)) / 2
		c.ncoRot *= complex(g, 0)
	}
	v *= c.ncoRot
	c.ncoRot *= c.ncoStep
	c.dHist[c.dHead] = v
	c.dHist[c.dHead+discTaps] = v
	c.dHead = (c.dHead + 1) % discTaps
	if (n+1)%int64(c.discDec) != 0 {
		return
	}
	y := fir(c.dHist[c.dHead:c.dHead+discTaps], c.dTaps)
	c.eAcc += real(y)*real(y) + imag(y)*imag(y)
	if (n+1+int64(c.delayCh)-discTaps/2)%energyBin == 0 {
		c.energy.push(c.eAcc)
		c.eAcc = 0
	}
	d := y * complex(real(c.prev), -imag(c.prev))
	c.prev = y
	f := float32(math.Atan2(float64(imag(d)), float64(real(d)))) * c.discHz
	nr := len(c.rrc)
	c.fHist[c.fHead] = f
	c.fHist[c.fHead+nr] = f
	c.fHead = (c.fHead + 1) % nr
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
