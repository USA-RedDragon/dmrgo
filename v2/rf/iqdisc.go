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
	// the oscillator, and iqAFCSearchHz and iqAFCLockHz the least carrier
	// offset that moves it while the core searches and while it is
	// locked.
	iqAFCBlock    = basebandRate / 20
	iqAFCSearchHz = 30
	iqAFCLockHz   = 100
)

// iqDisc is the FM discriminator of the I/Q tiers: an oscillator that
// removes the carrier, a channel filter to discCutHz that decimates the
// baseband by decim, and the frequency of each output sample, from the
// phase of its product with the conjugate of the one before.
type iqDisc struct {
	decim   int
	taps    [iqTaps]float32
	hist    [2 * iqTaps]complex64
	head    int
	phase   int
	prev    complex64
	ncoStep complex64
	ncoRot  complex64
	ncoN    int
	hz      float32
}

// init sets up d to decimate by decim, with the oscillator at 0 Hz.
func (d *iqDisc) init(decim int) {
	*d = iqDisc{decim: decim, ncoRot: 1, ncoStep: 1}
	fc := discCutHz / basebandRate
	var sum float64
	for i := range d.taps {
		k := float64(i - iqTaps/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		w := 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(iqTaps-1))
		d.taps[i] = float32(s * w)
		sum += s * w
	}
	for i := range d.taps {
		d.taps[i] /= float32(sum)
	}
}

// tune moves the oscillator to remove a carrier at hz.
func (d *iqDisc) tune(hz float32) {
	d.hz = hz
	p := -2 * math.Pi * float64(hz) / basebandRate
	d.ncoStep = complex(float32(math.Cos(p)), float32(math.Sin(p)))
}

// push takes baseband sample v and reports whether it completed an output
// sample, whose frequency in Hz and channel filter sample it returns.
func (d *iqDisc) push(v complex64) (float32, complex64, bool) {
	if d.ncoN++; d.ncoN == 512 {
		d.ncoN = 0
		g := (3 - real(d.ncoRot)*real(d.ncoRot) - imag(d.ncoRot)*imag(d.ncoRot)) / 2
		d.ncoRot *= complex(g, 0)
	}
	w := v * d.ncoRot
	d.ncoRot *= d.ncoStep
	// The history is held twice, so that the taps read one run.
	d.hist[d.head], d.hist[d.head+iqTaps] = w, w
	if d.head++; d.head == iqTaps {
		d.head = 0
	}
	if d.phase++; d.phase < d.decim {
		return 0, 0, false
	}
	d.phase = 0
	h := d.hist[d.head : d.head+iqTaps]
	var re, im float32
	for k, t := range d.taps {
		re += t * real(h[k])
		im += t * imag(h[k])
	}
	y := complex(re, im)
	p := y * complex(real(d.prev), -imag(d.prev))
	d.prev = y
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
// oscillator follows the carrier offset that the DiscReceiver measures,
// its DC offset, so that the channel filter holds the carrier, and the
// DC offset that the DiscReceiver removes moves with it.
type iqRx struct {
	fe      *frontEnd
	wide    []complex64
	inChunk int
	dc      complex64
	dcN     int32
	signed  bool
	conv    iqDisc
	d       DiscReceiver
	audio   []int16
	afcN    int
	out     []Burst
	// yHist holds the last cohHist channel filter samples for the
	// coherent stage, yN of them so far; nil without it.
	yHist []complex64
	yN    int64
	// tunes holds the last moves of the oscillator, the channel filter
	// sample from which each holds and its frequency, for the coherent
	// stage; nTunes counts them.
	tunes  [iqTunes]iqTune
	nTunes int
}

// newIQRx returns the receiver of cfg with the settings opt.
func newIQRx(cfg Config, opt options) *iqRx {
	r := &iqRx{signed: cfg.Signed}
	hz := opt.IQHz
	if hz == 0 {
		hz = discRate
	}
	r.conv.init(basebandRate / hz)
	r.audio = make([]int16, 0, 1024)
	on := cfg.OnBurst
	if on == nil {
		r.out = make([]Burst, 0, 4*cfg.MaxWrite/(int(cfg.Rate*slotSeconds)*2)+16)
		on = func(b *Burst) {
			if len(r.out) < cap(r.out) {
				r.out = append(r.out, *b)
			}
		}
	}
	if err := r.d.Init(DiscConfig{Hz: hz, OnBurst: on, opt: &opt}); err != nil {
		panic(err)
	}
	if opt.SoftExt {
		x := softExtOf(&opt, 2*r.d.tm.step)
		if opt.CohExt && hz == discRate {
			r.yHist = make([]complex64, cohHist)
			x.coh = newCohStage(r, &opt)
			r.d.signMiss = leanExtSignMiss
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
		r.yHist[r.yN&(cohHist-1)] = y
		r.yN++
	}
	r.audio = append(r.audio, int16(max(min(roundf(f*iqUnitsPerHz), math.MaxInt16), math.MinInt16)))
	if len(r.audio) == cap(r.audio) {
		r.d.write(r.audio)
		r.audio = r.audio[:0]
	}
}

// follow moves the oscillator to the carrier offset that the DiscReceiver
// measures, and the DC offset that it removes by as much.
func (r *iqRx) follow() {
	f := &r.d.f
	if f.gain <= 0 {
		return
	}
	dc, lim := f.dc, float32(iqAFCSearchHz)
	if r.d.locked {
		dc, lim = dc+r.d.dcHz, iqAFCLockHz
	}
	// The DiscReceiver counts f.gain of its Hz per input unit.
	scale := f.gain * iqUnitsPerHz
	hz := dc / scale
	if abs32(hz) < lim {
		return
	}
	// The samples that the DiscReceiver has not taken yet carry the old
	// offset.
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	r.conv.tune(r.conv.hz + hz)
	r.tunes[r.nTunes%iqTunes] = iqTune{n: r.yN, hz: r.conv.hz}
	r.nTunes++
	f.dc -= hz * scale
	f.mean -= hz * scale
	r.d.tuneHz -= hz * scale
	f.refresh()
}

// writeBaseband processes the samples of Receiver.WriteBaseband.
func (r *iqRx) writeBaseband(x []complex64) []Burst {
	r.out = r.out[:0]
	for _, v := range x {
		r.sample(v)
	}
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	return r.out
}

// write processes the I/Q bytes of Receiver.Write.
func (r *iqRx) write(iq []byte) []Burst {
	r.out = r.out[:0]
	if r.fe == nil {
		for i := 0; i+1 < len(iq); i += 2 {
			r.sample(r.byteSample(iq[i], iq[i+1]))
		}
	} else {
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
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	return r.out
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
	if r.dcN < math.MaxInt32 {
		r.dcN++
	}
	g := max(1/(dcSeconds*discRate*2), 1/(float32(r.dcN)+dcStartSeconds*discRate*2))
	r.dc += (v - r.dc) * complex(g, 0)
	return v - r.dc
}

// flush finishes the stream at its end and returns the remaining bursts.
func (r *iqRx) flush() []Burst {
	r.out = r.out[:0]
	r.d.write(r.audio)
	r.audio = r.audio[:0]
	r.d.flush()
	return r.out
}
