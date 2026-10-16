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
	discTaps = 63
)

// ring is a power-of-two ring of complex samples addressed by absolute
// sample number.
type ring struct {
	buf  []complex64
	mask int64
	n    int64
}

// newRing returns a ring of at least size samples.
func newRing(size int) ring {
	n := 1
	for n < size {
		n <<= 1
	}
	return ring{buf: make([]complex64, n), mask: int64(n - 1)}
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
// sinc of sincTaps samples.
func (r *ring) interp(t float64) complex64 {
	i0 := int64(math.Floor(t))
	fr := t - float64(i0)
	var s complex64
	for k := -sincTaps/2 + 1; k <= sincTaps/2; k++ {
		u := float64(k) - fr
		w := 0.42 + 0.5*math.Cos(math.Pi*u/(sincTaps/2)) + 0.08*math.Cos(2*math.Pi*u/(sincTaps/2))
		if u != 0 {
			w *= math.Sin(math.Pi*u) / (math.Pi * u)
		}
		s += r.at(i0+int64(k)) * complex(float32(w), 0)
	}
	return s
}

// fring is a power-of-two ring of real samples addressed by absolute sample
// number.
type fring struct {
	buf  []float32
	mask int64
	n    int64
}

// newFring returns a ring of at least size samples.
func newFring(size int) fring {
	n := 1
	for n < size {
		n <<= 1
	}
	return fring{buf: make([]float32, n), mask: int64(n - 1)}
}

// push appends sample v.
func (r *fring) push(v float32) {
	r.buf[r.n&r.mask] = v
	r.n++
}

// at returns sample i, or 0 outside the samples the ring holds.
func (r *fring) at(i int64) float32 {
	if i < 0 || i >= r.n || i < r.n-int64(len(r.buf)) {
		return 0
	}
	return r.buf[i&r.mask]
}

// lerp returns the ring at the fractional sample t by linear interpolation.
func (r *fring) lerp(t float64) float32 {
	i0 := int64(math.Floor(t))
	fr := float32(t - float64(i0))
	return r.at(i0)*(1-fr) + r.at(i0+1)*fr
}

// channelizer moves the carrier to 0 Hz with an oscillator and filters the
// channel: a wide filter for coherent detection into ch, and for the
// discriminator tiers a narrower prefilter, the FM discriminator and the
// RRC matched filter into sym (Hz of deviation). Every output has the delay
// of its causal filters, which delay compensates.
type channelizer struct {
	wide     ring
	ncoHz    float64
	ncoStep  complex64
	ncoRot   complex64
	hist     []complex64
	head     int
	chTaps   []float32
	dTaps    []float32
	rrc      []float32
	dHist    []complex64
	dHead    int
	fHist    []float32
	fHead    int
	prev     complex64
	ch       ring
	sym      fring
	disc     bool
	delayCh  int
	delaySym int
}

// newChannelizer returns a channelizer whose rings hold size samples, with
// the discriminator path when disc, its prefilter cut off at cutHz.
func newChannelizer(size int, disc bool, cutHz float64) *channelizer {
	c := &channelizer{disc: disc, ncoRot: 1, ncoStep: 1}
	c.wide = newRing(size)
	c.hist = make([]complex64, 2*chanTaps)
	c.chTaps = kaiserLowpass(chanTaps, (chanPassHz+chanStopHz)/2/basebandRate, 7)
	c.ch = newRing(size)
	c.delayCh = chanTaps / 2
	if disc {
		c.dHist = make([]complex64, 2*discTaps)
		c.dTaps = kaiserLowpass(discTaps, cutHz/basebandRate, 5)
		r := rrcTapsAt(samplesPerSy)
		c.rrc = make([]float32, len(r))
		for i, v := range r {
			c.rrc[i] = float32(v)
		}
		c.fHist = make([]float32, 2*len(r))
		c.sym = newFring(size)
		c.delaySym = discTaps/2 + len(r)/2
	}
	return c
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

// tune sets the oscillator to hz, the carrier offset to remove, and
// filters again the wide samples the ring still holds, so that every
// sample of the channel rings has the same carrier.
func (c *channelizer) tune(hz float64) {
	c.ncoHz = hz
	p := -2 * math.Pi * hz / basebandRate
	c.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
	n := c.wide.n
	from := max(n-int64(len(c.wide.buf)), 0)
	c.ch.n, c.sym.n = from, from
	clear(c.hist)
	clear(c.dHist)
	clear(c.fHist)
	c.prev = 0
	for i := from; i < n; i++ {
		c.filter(c.wide.at(i), i)
	}
}

// push filters one wide baseband sample.
func (c *channelizer) push(v complex64) {
	n := c.wide.n
	c.wide.push(v)
	c.filter(v, n)
}

// filter runs the wide sample v, number n, through the oscillator and the
// filters.
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
	if !c.disc {
		return
	}
	nd := len(c.dTaps)
	c.dHist[c.dHead] = v
	c.dHist[c.dHead+nd] = v
	c.dHead = (c.dHead + 1) % nd
	y := fir(c.dHist[c.dHead:c.dHead+nd], c.dTaps)
	d := y * complex(real(c.prev), -imag(c.prev))
	c.prev = y
	f := float32(math.Atan2(float64(imag(d)), float64(real(d))) * basebandRate / (2 * math.Pi))
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
