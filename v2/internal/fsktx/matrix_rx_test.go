//go:build !tinygo

package fsktx

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

// rxPath is a receiver of the matrix on the baseband at rxRate: an
// rf.Receiver of a tier, or an rf.DiscReceiver of a tier behind the model
// of an FM receiver of discAudio. cn is the C/N of its penalty test, where
// the reference modulator loses about 2 percent of the bits.
type rxPath struct {
	name string
	tier rf.Tier
	disc bool
	cn   float64
}

func rxPaths() []rxPath {
	return []rxPath{
		{"rx0", rf.Tier0, false, 6}, {"rx1", rf.Tier1, false, 6}, {"rx2", rf.Tier2, false, 2}, {"rx3", rf.Tier3, false, 2},
		{"disc0", rf.Tier0, true, 6}, {"disc1", rf.Tier1, true, 6},
	}
}

// run returns the bursts that p recovers from the baseband y.
func (p rxPath) run(y []complex64) []rf.Burst {
	if !p.disc {
		return receive(y, p.tier)
	}
	return discBursts(discAudio(y), discHz, p.tier)
}

// discBursts returns the bursts that an rf.DiscReceiver of tier recovers
// from the discriminator samples a at hz.
func discBursts(a []int16, hz int, tier rf.Tier) []rf.Burst {
	var out []rf.Burst
	r, err := rf.NewDisc(rf.DiscConfig{Hz: hz, Tier: tier, OnBurst: func(b *rf.Burst) { out = append(out, *b) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(a); k += 1024 {
		r.Write(a[k:min(k+1024, len(a))])
	}
	r.Flush()
	return out
}

// s8Bytes returns the I/Q raw of u as signed 8-bit I/Q, with a dither
// of one unit.
func (u txUnit) s8Bytes(raw []int16) []byte {
	b := make([]byte, len(raw))
	for i, v := range raw {
		if u.format == fmtS16 {
			v = (v + 1<<7) >> 8
		}
		b[i] = byte(int8(v))
	}
	dither(b)
	return b
}

// dither adds a dither of one unit to the signed 8-bit I/Q b.
func dither(b []byte) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range b {
		b[i] = byte(int8(b[i]) + int8(rng.IntN(3)-1))
	}
}

// nativeBursts returns the bursts that an rf.Receiver of Tier1 recovers
// from the signed 8-bit I/Q b at rate.
func nativeBursts(b []byte, rate int) []rf.Burst {
	var got []rf.Burst
	r, err := rf.New(rf.Config{Rate: float64(rate), Signed: true, Tier: rf.Tier1, MaxWrite: 1 << 16, OnBurst: func(b *rf.Burst) { got = append(got, *b) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(b); k += 1 << 16 {
		r.Write(b[k:min(k+1<<16, len(b))])
	}
	r.Flush()
	return got
}

// refSig is the signal of the reference modulator for a stream, at
// rxRate, and the time of its symbol 0 in samples. The tests compare a
// modulator with it moved to the time of the modulator, so that both meet
// a receiver at the same time.
type refSig struct {
	ts    *testStream
	dev   []float64
	x48   []complex128
	delay float64
	n     int
}

func newRefSig(ts *testStream) *refSig {
	r := &refSig{ts: ts, dev: refDeviation(ts.syms)}
	r.x48 = resample(fm(r.dev), dacRate, rxRate)
	r.delay = fsk(r.x48, ts.syms).Delay
	r.n = len(r.x48) + int(2*rxPad*rxRate)
	return r
}

// shift returns the shift of the reference at rate that moves it to the
// time d, in samples at rxRate.
func (r *refSig) shift(d float64, rate int) int {
	return int(math.Round((r.delay - d) * float64(rate) / rxRate))
}

// native returns the reference as dithered signed 8-bit I/Q at the rate
// and carrier of u, moved to the time d: its deviation at dacRate taken
// at each sample by a cubic interpolation, which is exact to about 1e-6
// for the 2.9 kHz of the band of the pulse, and integrated into the phase.
func (r *refSig) native(u txUnit, d float64) []byte {
	n := int(float64(len(r.dev)) * float64(u.rate) / dacRate)
	s := r.shift(d, u.rate)
	a := u.amplitude()
	if u.format == fmtS16 {
		a /= 256
	}
	at := func(k int) float64 { return r.dev[min(max(k, 0), len(r.dev)-1)] }
	b := make([]byte, 2*n)
	var ph float64
	for i := range n {
		k := i + s
		if k < 0 || k >= n {
			continue
		}
		t := float64(k) * dacRate / float64(u.rate)
		j := int(t)
		x := t - float64(j)
		p0, p1, p2, p3 := at(j-1), at(j), at(j+1), at(j+2)
		f := p1 + 0.5*x*(p2-p0+x*(2*p0-5*p1+4*p2-p3+x*(3*(p1-p2)+p3-p0)))
		ph += 2 * math.Pi * (f + float64(u.offHz)) / float64(u.rate)
		ph = math.Remainder(ph, 2*math.Pi)
		y, c := math.Sincos(ph)
		b[2*i], b[2*i+1] = byte(int8(math.Round(a*c))), byte(int8(math.Round(a*y)))
	}
	dither(b)
	return b
}

// audio returns the reference as discriminator samples at hz, 15000 units
// for the 1944 Hz of symbol +3, moved to the time d.
func (r *refSig) audio(hz int, d float64) []int16 {
	c := make([]complex128, len(r.dev))
	for i, v := range r.dev {
		c[i] = complex(v*tableUnits/(3*DeviationHz), 0)
	}
	y := resample(c, dacRate, hz)
	s := r.shift(d, hz)
	out := make([]int16, len(y))
	for i := range out {
		if k := i + s; k >= 0 && k < len(y) {
			out[i] = int16(math.Round(real(y[k])))
		}
	}
	return out
}

// rxUnits returns the units of the receive tests.
func rxUnits(full bool) []txUnit {
	if full {
		units := nominal(true)
		out := make([]txUnit, 0, len(units))
		for _, u := range units {
			if !u.audio() {
				u.offHz = 1200
			}
			out = append(out, u)
		}
		return out
	}
	a := func(mcu bool, tier Tier, rate int) txUnit {
		return txUnit{mcu: mcu, tier: tier, format: fmtAudio, rate: rate, level: tableUnits}
	}
	q := func(mcu bool, tier Tier, f txFormat, rate int) txUnit {
		return txUnit{mcu: mcu, tier: tier, format: f, rate: rate, offHz: 1200, ramp: rate / 1000}
	}
	return []txUnit{
		a(true, Tier0, 24000), a(false, Tier0, 48000), a(false, Tier1, 9600), a(false, Tier1, 44100), a(false, Tier2, 48000),
		q(true, Tier0, fmtS16, 48000), q(false, Tier0, fmtS8, 96000), q(false, Tier1, fmtS16, 48001), q(false, Tier1, fmtS8, 2400000),
		q(false, Tier2, fmtS16, 96000),
	}
}

// refRuns holds the bit errors of the reference modulator by receiver,
// C/N and shift, for the penalty test.
type refRuns struct {
	mu   sync.Mutex
	errs map[string]*refRun
}

type refRun struct {
	once sync.Once
	errs int
}

// get returns the bit errors of the reference r through p at C/N cn,
// moved by shift samples, over seeds, run once.
func (rs *refRuns) get(r *refSig, p rxPath, cn float64, shift, seeds int) int {
	k := fmt.Sprintf("%s/%v/%d", p.name, cn, shift)
	rs.mu.Lock()
	e, ok := rs.errs[k]
	if !ok {
		e = &refRun{}
		rs.errs[k] = e
	}
	rs.mu.Unlock()
	e.once.Do(func() { e.errs = berErrs(r.ts, r.x48, shift, r.n, p, cn, seeds) })
	return e.errs
}

// berErrs returns the bit errors of the bursts of ts that p detects from
// the baseband x moved by shift samples, n long, at C/N cn, summed over
// seeds and over the times of rxTimings.
func berErrs(ts *testStream, x []complex128, shift, n int, p rxPath, cn float64, seeds int) int {
	e := 0
	for s := range seeds {
		for k := range rxTimings {
			e += ts.rawScore(p.run(noisy(x, shift+timing(k, rxRate), n, cn, uint64(100+s))), rxPad).errs
		}
	}
	return e
}

// penaltyDB returns the most penalty of u at the receivers, in dB of C/N
// against the reference modulator: twice the spread of the bit errors of
// the receivers over the seeds and times of the test.
func (u txUnit) penaltyDB() float64 {
	if u.mcu || u.tier == Tier0 {
		return 1.5
	}
	return 1
}

// keyedLike returns the I/Q b, signed 8-bit, keyed as the I/Q raw of
// amplitude a: each sample scaled by the amplitude of raw, so that a
// reference has the power ramps of the modulator.
func keyedLike(b []byte, raw []int16, a float64) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b) && i+1 < len(raw); i += 2 {
		e := math.Hypot(float64(raw[i]), float64(raw[i+1])) / a
		out[i] = byte(int8(math.Round(float64(int8(b[i])) * e)))
		out[i+1] = byte(int8(math.Round(float64(int8(b[i+1])) * e)))
	}
	return out
}

// rxTimings is the number of times of the stream, timingStep samples
// apart at rxRate, at which the receive tests detect it. The receivers
// lose a stream at some times and not at the next sample, for the
// reference modulator too, so a test at one time tests the receiver more
// than the modulator.
const (
	rxTimings  = 5
	timingStep = 17
)

// timing returns the shift of the time k of rxTimings, in samples at
// rate.
func timing(k, rate int) int {
	return (k - rxTimings/2) * timingStep * rate / rxRate
}

// checkDetect fails unless a receiver detects every burst of the stream
// but the first two bit for bit at a majority of rxTimings times, or at as
// many of them as from the reference modulator less one, for a receiver
// that loses the reference at some of them. score returns the
// detection of the modulator, or with ref set of the reference, at the
// time k.
func checkDetect(t *testing.T, name string, score func(k int, ref bool) rawScore) {
	t.Helper()
	ok := func(ref bool) (int, string) {
		n, line := 0, ""
		for k := range rxTimings {
			sc := score(k, ref)
			if sc.bursts > 0 && sc.exact >= sc.bursts-2 {
				n++
			}
			line += fmt.Sprintf(" %d/%d", sc.exact, sc.bursts)
		}
		return n, line
	}
	n, line := ok(false)
	if n > rxTimings/2 {
		return
	}
	r, rline := ok(true)
	t.Logf("%s: exact bursts at %d times:%s; reference:%s", name, rxTimings, line, rline)
	if n < r-1 {
		t.Errorf("%s: detected at %d of %d times, reference %d", name, n, rxTimings, r)
	}
}

// moved returns b, interleaved I/Q, moved later by k samples, or earlier
// for k below 0.
func moved(b []byte, k int) []byte {
	if k >= 0 {
		return append(make([]byte, 2*k, 2*k+len(b)), b...)
	}
	return b[-2*k:]
}

// movedAudio is moved for audio samples.
func movedAudio(a []int16, k int) []int16 {
	if k >= 0 {
		return append(make([]int16, k, k+len(a)), a...)
	}
	return a[-k:]
}

// TestTxMatrixReceive runs each modulator through the rf receivers: as
// 8-bit I/Q at its rate into an rf.Receiver of Tier1 for an I/Q
// modulator, a BS outbound stream after 250 ms of carrier, which that
// receiver takes to settle, and MS bursts keyed in their slots; as
// discriminator samples into an rf.DiscReceiver of Tier0 and Tier1 for an
// audio modulator at a rate that it takes; and as baseband at a C/N of 30
// dB into every receiver of rxPaths. Each detects the stream as
// checkDetect says. At the C/N of the penalty test of each receiver, the
// bit errors stay within those of the reference modulator, at the time of
// the modulator, at a C/N lower by penaltyDB.
func TestTxMatrixReceive(t *testing.T) {
	t.Parallel()
	full := txFull()
	seeds := 1
	if full {
		seeds = 8
	}
	ts := newTestStream(t, 2, 40)
	lead := ts.withLead(1200)
	ref, refLead := newRefSig(ts), newRefSig(lead)
	refs := &refRuns{errs: map[string]*refRun{}}
	for _, u := range rxUnits(full) {
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			raw := u.run(t, ts.syms, keyedOn())
			x := u.signal48(raw, radio{ideal: true})
			d := fsk(x, ts.syms).Delay
			if !u.audio() {
				b := u.s8Bytes(u.run(t, lead.syms, keyedOn()))
				var rb []byte
				checkDetect(t, "native BS", func(k int, isRef bool) rawScore {
					if !isRef {
						return lead.rawScore(nativeBursts(moved(b, timing(k, u.rate)), u.rate), 0)
					}
					if rb == nil {
						rb = refLead.native(u, d+(refLead.delay-ref.delay))
					}
					return lead.rawScore(nativeBursts(moved(rb, timing(k, u.rate)), u.rate), 0)
				})
				ms, keys := newMSStream(t, u, 2)
				mraw := u.run(t, ms.syms, keys)
				mb := u.s8Bytes(mraw)
				var mrb []byte
				checkDetect(t, "native MS", func(k int, isRef bool) rawScore {
					if !isRef {
						return ms.rawScore(nativeBursts(moved(mb, timing(k, u.rate)), u.rate), 0)
					}
					if mrb == nil {
						mrb = keyedLike(newRefSig(ms).native(u, d), mraw, u.amplitude())
					}
					return ms.rawScore(nativeBursts(moved(mrb, timing(k, u.rate)), u.rate), 0)
				})
			}
			if u.audio() && slices.Contains([]int{24000, 12000, 9600}, u.rate) {
				a := make([]int16, len(raw))
				for i, v := range raw {
					a[i] = int16(int(v) - u.offset)
				}
				ra := ref.audio(u.rate, d)
				for _, tier := range []rf.Tier{rf.Tier0, rf.Tier1} {
					checkDetect(t, fmt.Sprintf("disc %d", tier-rf.Tier0), func(k int, isRef bool) rawScore {
						if isRef {
							return ts.rawScore(discBursts(movedAudio(ra, timing(k, u.rate)), u.rate, tier), 0)
						}
						return ts.rawScore(discBursts(movedAudio(a, timing(k, u.rate)), u.rate, tier), 0)
					})
				}
			}
			sh := ref.shift(d, rxRate)
			for _, p := range rxPaths() {
				if p.tier == rf.Tier3 && !full && u.rate != 44100 {
					continue
				}
				checkDetect(t, p.name+" at 30 dB", func(k int, isRef bool) rawScore {
					if isRef {
						return ts.rawScore(p.run(noisy(ref.x48, sh+timing(k, rxRate), ref.n, 30, 1)), rxPad)
					}
					return ts.rawScore(p.run(noisy(x, timing(k, rxRate), ref.n, 30, 1)), rxPad)
				})
				got := berErrs(ts, x, 0, ref.n, p, p.cn, seeds)
				lim := refs.get(ref, p, p.cn-u.penaltyDB(), sh, seeds)
				t.Logf("%s at %v dB: %d bits wrong; reference %d at %v dB", p.name, p.cn, got, lim, p.cn-u.penaltyDB())
				if got > lim {
					t.Errorf("%s at %v dB: %d bits wrong, reference %d at %v dB", p.name, p.cn, got, lim, p.cn-u.penaltyDB())
				}
			}
		})
	}
}
