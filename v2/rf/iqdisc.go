package rf

import "math"

// Layout of the discriminator of iqRx.
const (
	// iqTaps is the length of the channel filter of iqDisc.
	iqTaps = 31
	// iqUnitsPerHz is the scale of the discriminator samples that iqRx
	// hands the DiscReceiver.
	iqUnitsPerHz = 2
	// iqAFCBlock is the number of baseband samples between the moves of
	// the oscillator, and iqAFCLockHz the least carrier offset that moves
	// it while the core is locked.
	iqAFCBlock  = basebandRate / 20
	iqAFCLockHz = 100
	// While the core searches, an offset of at least iqAFCPullHz in one
	// block moves the oscillator. The symbols of a block spread its mean
	// frequency by about 100 Hz, so below that the oscillator moves when
	// the mean over the blocks, by iqAFCAvgGain, is iqAFCFineHz off.
	iqAFCPullHz  = 300
	iqAFCFineHz  = 60
	iqAFCAvgGain = 0.25
	// iqAFCCoherence is the least magnitude of the sum of the products of
	// a block, over the power of its channel filter samples, at which it
	// moves the oscillator while the core searches. Noise alone gives 0.33
	// to 0.47, and its mean frequency is noise that would walk the
	// oscillator away from where the next carrier comes; a DMR carrier
	// gives 0.73 to 0.82, and 0.49 to 0.62 at a C/N of -3 dB.
	iqAFCCoherence = 0.48
)

// iqDisc is the FM discriminator of the I/Q tiers: an oscillator that
// removes the carrier, a channel filter to discCutHz that decimates the
// baseband by decim, and the frequency of each output sample, from the
// phase of its product with the conjugate of the one before. With a
// half-band stage, the channel filter runs at half the baseband rate.
// With options.IQFront above 0 it is the lean front of iq0Front in place
// of the channel filter, and taps is nil.
type iqDisc struct {
	decim int
	// step is the decimation of the channel filter, decim without the
	// half-band stage and decim/2 with it.
	step    int
	taps    []float32
	hist    []complex64
	head    int
	phase   int
	half    iqHalf
	prev    complex64
	ncoStep complex64
	ncoRot  complex64
	ncoN    int
	hz      float32
	// acc sums the products of the output samples with the conjugate of
	// the ones before since the last move of the oscillator, and pw the
	// power of the output samples.
	acc complex64
	pw  float32
	// With afc set, accAvg sums the products of the sums of the last
	// len(afc) output samples, a moving average that keeps out of the
	// estimate the edges of the channel, where another channel leaks in,
	// and pwAvg their power; afcSum is the sum, afcPrev the one before,
	// afcHead the oldest.
	accAvg  complex64
	pwAvg   float32
	afc     []complex64
	afcSum  complex64
	afcPrev complex64
	afcHead int
	iq0Front
}

// iqFilter is the design of the filters of iqDisc: the length, Kaiser
// window and cutoff of the channel filter, and the length of the
// half-band stage. The zero value is the Hamming window of iqTaps taps to
// discCutHz at the baseband rate.
type iqFilter struct {
	taps  int
	beta  float64
	cutHz float64
	half  int
	afc   int
}

// iqHalf is a half-band filter that decimates by 2: every other tap but
// the center one is 0, so it holds only the center tap and the others of
// one side.
type iqHalf struct {
	center float32
	side   []float32
	hist   []complex64
	head   int
	phase  int
}

// init sets up h with n taps, n+1 a multiple of 4; n 0 leaves it off.
func (h *iqHalf) init(n int) {
	*h = iqHalf{}
	if n == 0 {
		return
	}
	h.side = make([]float32, (n+1)/4)
	h.hist = make([]complex64, 2*n)
	w := windowTaps(n, 0.25, 7)
	var sum float64
	for _, v := range w {
		sum += v
	}
	h.center = float32(w[n/2] / sum)
	for j := range h.side {
		h.side[j] = float32(w[n/2+2*j+1] / sum)
	}
}

// push takes sample v and reports whether it completed an output sample,
// which it returns.
func (h *iqHalf) push(v complex64) (complex64, bool) {
	n := len(h.hist) / 2
	h.hist[h.head], h.hist[h.head+n] = v, v
	if h.head++; h.head == n {
		h.head = 0
	}
	if h.phase ^= 1; h.phase == 1 {
		return 0, false
	}
	x := h.hist[h.head : h.head+n]
	c := n / 2
	y := complex(h.center, 0) * x[c]
	for j, t := range h.side {
		o := 2*j + 1
		y += complex(t, 0) * (x[c-o] + x[c+o])
	}
	return y, true
}

// windowTaps returns n taps of a windowed sinc low pass with cutoff fc, a
// fraction of the sample rate: a Kaiser window of beta, or Hamming with
// beta 0. They are not normalized.
func windowTaps(n int, fc, beta float64) []float64 {
	h := make([]float64, n)
	for i := range h {
		k := float64(i - n/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
		if beta > 0 {
			r := 2*float64(i)/float64(n-1) - 1
			w = bessel0(beta*math.Sqrt(max(0, 1-r*r))) / bessel0(beta)
		}
		h[i] = s * w
	}
	return h
}

// init sets up d to decimate by decim with the filters of f, with the
// oscillator at 0 Hz.
func (d *iqDisc) init(decim int, f iqFilter) {
	*d = iqDisc{decim: decim, step: decim, ncoRot: 1, ncoStep: 1}
	n, fc, rate := iqTaps, discCutHz, float64(basebandRate)
	if f.taps > 0 {
		n = f.taps
	}
	if f.cutHz > 0 {
		fc = f.cutHz
	}
	if f.half > 0 && decim%2 == 0 {
		d.half.init(f.half)
		d.step, rate = decim/2, rate/2
	}
	if f.afc > 1 {
		d.afc = make([]complex64, f.afc)
	}
	d.taps = make([]float32, n)
	d.hist = make([]complex64, 2*n)
	w := windowTaps(n, fc/rate, f.beta)
	var sum float64
	for _, v := range w {
		sum += v
	}
	for i, v := range w {
		d.taps[i] = float32(v / sum)
	}
}

// tune moves the oscillator to remove a carrier at hz.
func (d *iqDisc) tune(hz float32) {
	if d.taps == nil {
		d.tuneLean(hz)
		return
	}
	d.hz = hz
	p := -2 * math.Pi * float64(hz) / basebandRate
	d.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

// push takes baseband sample v and reports whether it completed an output
// sample, whose frequency in Hz and channel filter sample it returns.
func (d *iqDisc) push(v complex64) (float32, complex64, bool) {
	if d.taps == nil {
		return d.pushLean(v)
	}
	if d.ncoN++; d.ncoN == 512 {
		d.ncoN = 0
		g := (3 - real(d.ncoRot)*real(d.ncoRot) - imag(d.ncoRot)*imag(d.ncoRot)) / 2
		d.ncoRot *= complex(g, 0)
	}
	w := v * d.ncoRot
	d.ncoRot *= d.ncoStep
	if d.half.side != nil {
		var ok bool
		if w, ok = d.half.push(w); !ok {
			return 0, 0, false
		}
	}
	// The history is held twice, so that the taps read one run.
	n := len(d.taps)
	d.hist[d.head], d.hist[d.head+n] = w, w
	if d.head++; d.head == n {
		d.head = 0
	}
	if d.phase++; d.phase < d.step {
		return 0, 0, false
	}
	d.phase = 0
	var re, im float32
	if n == iqTaps {
		// The taps and the history are read through pointers: a range
		// over an array copies it, and TinyGo copies it again on every
		// pass of the loop.
		h := (*[iqTaps]complex64)(d.hist[d.head : d.head+iqTaps])
		for k, t := range (*[iqTaps]float32)(d.taps) {
			re += t * real(h[k])
			im += t * imag(h[k])
		}
	} else {
		h := d.hist[d.head : d.head+n]
		h = h[:n]
		for k, t := range d.taps {
			re += t * real(h[k])
			im += t * imag(h[k])
		}
	}
	y := complex(re, im)
	p := y * complex(real(d.prev), -imag(d.prev))
	d.prev = y
	d.acc += p
	d.pw += re*re + im*im
	if d.afc != nil {
		d.afcSum += y - d.afc[d.afcHead]
		d.afc[d.afcHead] = y
		if d.afcHead++; d.afcHead == len(d.afc) {
			// The sum starts again from the samples, so that the rounding
			// of its updates does not add up.
			d.afcHead, d.afcSum = 0, 0
			for _, a := range d.afc {
				d.afcSum += a
			}
		}
		d.accAvg += d.afcSum * complex(real(d.afcPrev), -imag(d.afcPrev))
		d.afcPrev = d.afcSum
		d.pwAvg += real(d.afcSum)*real(d.afcSum) + imag(d.afcSum)*imag(d.afcSum)
	}
	return atan2f(imag(p), real(p)) * (float32(basebandRate/d.decim) / (2 * math.Pi)), y, true
}

// iqTunes is the number of moves of the oscillator that iqRx keeps.
const iqTunes = 16

// iqTune is a move of the oscillator of iqRx: the channel filter sample
// from which it holds, and the frequency it removes.
type iqTune struct {
	n  int64
	hz float32
}

// ncoAt returns the frequency that the oscillator removed at channel
// filter sample n, as far back as the moves that r keeps.
func (r *iqRx) ncoAt(n int64) float32 {
	hz := r.conv.hz
	for i := 0; i < min(r.nTunes, iqTunes); i++ {
		t := r.tunes[(r.nTunes-1-i)%iqTunes]
		if t.n <= n {
			return hz
		}
		// Before this move, the oscillator was at the frequency of the
		// move before it, or 0.
		hz = 0
		if j := r.nTunes - 2 - i; j >= 0 && i+1 < iqTunes {
			hz = r.tunes[j%iqTunes].hz
		}
	}
	return hz
}

// iqRx is the receiver of Tier 1 on I/Q: iqDisc before the DiscReceiver
// of Tier 0, so that I/Q runs the same core as discriminator audio. The
// oscillator follows the carrier, so that the channel filter holds it, and
// the DC offset that the DiscReceiver removes moves with it.
type iqRx struct {
	fe      *frontEnd
	wide    []complex64
	inChunk int
	carry   byteCarry
	dc      dcBlock
	signed  bool
	conv    iqDisc
	d       DiscReceiver
	audio   []int16
	afcN    int
	// afcAvg is the mean of the sums of iqDisc over the blocks since the
	// oscillator last moved, while the core searches.
	afcAvg complex64
	out    []Burst
	// yHist holds the last options.cohRing channel filter samples for the
	// coherent stage, yN of them so far; nil without it.
	yHist []complex64
	yN    int64
	// tunes holds the last moves of the oscillator, the channel filter
	// sample from which each holds and its frequency, for the coherent
	// stage, nil without it; nTunes counts them.
	tunes  *[iqTunes]iqTune
	nTunes int
	// maxHz, when above 0, bounds the oscillator to the carrier offsets
	// within it, so that it does not follow a carrier of another channel.
	maxHz float32
	// syncOnly moves the oscillator while the core is locked only by the
	// carrier offset of its SYNC fits, not by the mean frequency.
	syncOnly bool
}

// newIQRx returns the receiver of cfg with the settings opt.
func newIQRx(cfg Config, opt options) *iqRx {
	r := &iqRx{signed: cfg.Signed}
	r.dc.init(float64(cfg.BasebandHz))
	hz := opt.IQHz
	if hz == 0 {
		hz = discRate
	}
	if opt.IQFront > 0 {
		r.conv.initLean(basebandRate/hz, &opt)
	} else {
		r.conv.init(basebandRate/hz, iqFilter{taps: opt.IQTaps, beta: opt.IQKaiser, cutHz: opt.IQCutHz, half: opt.IQHalfband, afc: opt.IQAFCAvg})
	}
	r.maxHz, r.syncOnly = opt.IQTuneMaxHz, opt.IQTuneSyncOnly
	n := opt.IQBuf
	if n == 0 {
		n = 1024
	}
	r.audio = make([]int16, 0, n)
	on := cfg.OnBurst
	if on == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+16)
		on = func(b *Burst) {
			if n := len(r.out); n < cap(r.out) {
				r.out = r.out[:n+1]
				r.out[n] = *b
			}
		}
	}
	if err := r.d.Init(DiscConfig{Hz: hz, OnBurst: on, opt: &opt}); err != nil {
		panic(err)
	}
	if opt.SoftExt {
		x := softExtOf(&opt, 2*r.d.tm.step)
		if opt.CohExt {
			r.yHist = make([]complex64, opt.cohRing())
			r.tunes = new([iqTunes]iqTune)
			x.coh = newCohStage(r, &opt)
			if !opt.CohLocked {
				r.d.signMiss = opt.signMiss()
			}
		}
		r.d.ext = x
	}
	if cfg.Rate != float64(cfg.BasebandHz) {
		r.fe = newFrontEnd(cfg.Rate, cfg.Signed, float64(cfg.BasebandHz))
		r.wide = make([]complex64, leanChunk)
		r.inChunk = max(2*int(float64(leanChunk-4)*cfg.Rate/float64(cfg.BasebandHz)), 2)
	}
	return r
}

// sample runs one baseband sample through the discriminator, and hands
// the core a buffer of its output when it fills.
func (r *iqRx) sample(v complex64) {
	if r.afcN++; r.afcN == iqAFCBlock {
		r.afcN = 0
		r.follow()
	}
	f, y, ok := r.conv.push(v)
	if !ok {
		return
	}
	if r.yHist != nil {
		r.yHist[r.yN&int64(len(r.yHist)-1)] = y
		r.yN++
	}
	n := len(r.audio)
	r.audio = r.audio[:n+1]
	r.audio[n] = int16(max(min(roundf(f*iqUnitsPerHz), math.MaxInt16), math.MinInt16))
	if n+1 == cap(r.audio) {
		r.d.write(r.audio)
		r.audio = r.audio[:0]
	}
}

// follow moves the oscillator to the carrier offset: the mean frequency
// of the channel filter samples while the core searches, and the DC
// offset that the DiscReceiver measures while it is locked.
func (r *iqRx) follow() {
	acc, pw := r.conv.acc, r.conv.pw
	if r.conv.afc != nil && !r.d.locked {
		// While the core searches, the moving average keeps the edges of
		// the channel out of the estimate. It pulls the estimate of an
		// offset toward 0, so the SYNC fits of a lock check it against
		// the mean of the samples themselves.
		acc, pw = r.conv.accAvg, r.conv.pwAvg
	}
	r.conv.acc, r.conv.pw, r.conv.accAvg, r.conv.pwAvg = 0, 0, 0, 0
	// Below the FM threshold the clicks pull the mean of the
	// discriminator toward 0 Hz. The phase of the sum of the products is
	// the mean frequency of the carrier, which noise pulls toward the
	// oscillator only by its share of the power. The symbols of a block
	// do not average to 0 Hz, so its mean over the blocks, avg, spreads
	// less.
	est := r.conv.hzOf(acc)
	// The lean front passes white noise, whose mean frequency is
	// anywhere in its band: a block without a carrier moves nothing.
	quiet := r.conv.taps == nil && r.conv.quiet(acc)
	switch {
	case quiet:
	case r.afcAvg == 0:
		r.afcAvg = acc
	default:
		r.afcAvg += complex(iqAFCAvgGain, 0) * (acc - r.afcAvg)
	}
	avg := r.conv.hzOf(r.afcAvg)
	// The samples that the DiscReceiver has not taken yet can scale its
	// input, move its offset or lock it, and the move must see that. The
	// bursts that they complete see the mean of their block.
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	// The DiscReceiver counts f.gain of its Hz per input unit.
	scale := f.gain * iqUnitsPerHz
	if r.d.locked {
		// A lock can come while the oscillator still pulls in, and the
		// channel filter then pulls both the mean frequency and the SYNC
		// fits toward the oscillator, by different shares, so that they
		// disagree: the mean, which the symbols do not move, pulls in.
		// Otherwise the SYNC fits of a false lock measure any offset, so
		// the mean frequency must agree with them.
		hz := (f.dc + r.d.dcHz) / scale
		switch {
		case abs32(avg) >= iqAFCPullHz && !r.syncOnly:
			// The channel filter pulled the mean of the blocks before
			// toward the oscillator by more than it pulls the mean after
			// the move: the mean starts again, as while the core searches.
			r.afcAvg = 0
			r.tune(avg, f.dc-avg*scale)
		case abs32(hz) >= iqAFCLockHz && abs32(hz-avg) < iqAFCPullHz:
			r.tune(hz, f.dc-hz*scale)
		}
		return
	}
	if real(acc)*real(acc)+imag(acc)*imag(acc) < iqAFCCoherence*iqAFCCoherence*pw*pw {
		// No carrier: the mean starts again with the next one.
		r.afcAvg = 0
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

// hzOf returns the frequency of the phase of a sum of the products of
// output samples with the conjugate of the ones before.
func (d *iqDisc) hzOf(acc complex64) float32 {
	return atan2f(imag(acc), real(acc)) * (float32(basebandRate/d.decim) / (2 * math.Pi))
}

// tune moves the oscillator by hz, and sets the DC offset that the
// DiscReceiver removes to dc, in its units. The caller hands the
// DiscReceiver the samples before the move first.
func (r *iqRx) tune(hz, dc float32) {
	f := &r.d.f
	scale := f.gain * iqUnitsPerHz
	if r.maxHz > 0 && abs32(r.conv.hz+hz) > r.maxHz {
		return
	}
	r.conv.tune(r.conv.hz + hz)
	if r.tunes != nil {
		r.tunes[r.nTunes%iqTunes] = iqTune{n: r.yN, hz: r.conv.hz}
		r.nTunes++
	}
	// The mean of the blocks moves with the oscillator.
	p := -2 * math.Pi * float64(hz) / float64(basebandRate/r.conv.decim)
	r.afcAvg *= complex(float32(math.Cos(p)), float32(math.Sin(p)))
	// A move while locked keeps the input of the core. A move while it
	// searches moves the input by -d, and the values that the core holds
	// move with it, so that the front end removes tuneHz from all of
	// them. The mean frequency of the block measured the carrier, which
	// the move takes out, so that the mean input is then dc: the mean of
	// the blocks before does not move with the oscillator, as most of
	// them are noise, whose mean does not follow the oscillator.
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

// writeBaseband processes the samples of Receiver.WriteBaseband.
func (r *iqRx) writeBaseband(x []complex64) []Burst {
	if r.conv.taps == nil {
		return r.writeLean(x)
	}
	r.out = r.out[:0]
	for _, v := range x {
		r.sample(v)
	}
	r.drain()
	return r.out
}

// drain hands the core the samples that it has not taken yet, at the end
// of a Write. With the coherent stage it keeps them until the buffer
// fills or the oscillator moves, so that the samples that the stage sees
// ahead of the core do not depend on how the input splits into Writes.
func (r *iqRx) drain() {
	if r.yHist != nil {
		return
	}
	r.d.write(r.audio)
	r.audio = r.audio[:0]
}

// write processes the I/Q bytes of Receiver.Write.
func (r *iqRx) write(iq []byte) []Burst {
	r.out = r.out[:0]
	pair, iq := r.carry.split(iq)
	r.bytes(pair)
	r.bytes(iq)
	r.drain()
	return r.out
}

// bytes processes whole I/Q pairs of bytes.
func (r *iqRx) bytes(iq []byte) {
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
		return
	}
	for len(iq) > 0 {
		n := min(len(iq), r.inChunk) &^ 1
		if n == 0 {
			break
		}
		w := r.fe.process(iq[:n], r.wide)
		iq = iq[n:]
		for _, v := range r.wide[:w] {
			r.sample(v)
		}
	}
}

// byteSample returns the I/Q bytes i and q at the baseband rate as a
// complex sample, less their DC offset, as leanRx.byteSample does.
func (r *iqRx) byteSample(i, q byte) complex64 {
	var v complex64
	if r.signed {
		v = complex(float32(int8(i))/127.5, float32(int8(q))/127.5)
	} else {
		v = complex((float32(i)-127.5)/127.5, (float32(q)-127.5)/127.5)
	}
	return r.dc.remove(v)
}

// flush finishes the stream at its end and returns the remaining bursts.
func (r *iqRx) flush() []Burst {
	r.out = r.out[:0]
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	r.d.flush()
	return r.out
}
