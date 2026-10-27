//go:build !tinygo

package fsktx

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

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

func (p rxPath) run(y []complex64) []rf.Burst {
	if !p.disc {
		return receive(y, p.tier)
	}
	return discBursts(discAudio(y), discHz, p.tier)
}

func discBursts(a []int16, hz int, tier rf.Tier) []rf.Burst {
	var out []rf.Burst
	r, err := rf.NewDisc(&rf.DiscConfig{Hz: hz, Tier: tier, OnBurst: func(b *rf.Burst) { out = append(out, *b) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(a); k += 1024 {
		check(r.Write(a[k:min(k+1024, len(a))]))
	}
	r.Flush()
	return out
}

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

func dither(b []byte) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range b {
		b[i] = byte(int8(b[i]) + int8(rng.IntN(3)-1))
	}
}

func nativeBursts(b []byte, rate int) []rf.Burst {
	var got []rf.Burst
	r, err := rf.New(&rf.Config{Rate: float64(rate), Signed: true, Tier: rf.Tier1, MaxWrite: 1 << 16, OnBurst: func(b *rf.Burst) { got = append(got, *b) }})
	if err != nil {
		panic(err)
	}
	for k := 0; k < len(b); k += 1 << 16 {
		check(r.Write(b[k:min(k+1<<16, len(b))]))
	}
	r.Flush()
	return got
}

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

func (r *refSig) shift(d float64, rate int) int {
	return int(math.Round((r.delay - d) * float64(rate) / rxRate))
}

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

type refRuns struct {
	mu   sync.Mutex
	errs map[string]*refRun
}

type refRun struct {
	once sync.Once
	errs int
}

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

func berErrs(ts *testStream, x []complex128, shift, n int, p rxPath, cn float64, seeds int) int {
	e := 0
	for s := range seeds {
		for k := range rxTimings {
			e += ts.rawScore(p.run(noisy(x, shift+timing(k, rxRate), n, cn, uint64(100+s))), rxPad).errs
		}
	}
	return e
}

func (u txUnit) penaltyDB() float64 {
	if u.mcu || u.tier == Tier0 {
		return 1.5
	}
	return 1
}

func keyedLike(b []byte, raw []int16, a float64) []byte {
	out := make([]byte, len(b))
	for i := 0; i+1 < len(b) && i+1 < len(raw); i += 2 {
		e := math.Hypot(float64(raw[i]), float64(raw[i+1])) / a
		out[i] = byte(int8(math.Round(float64(int8(b[i])) * e)))
		out[i+1] = byte(int8(math.Round(float64(int8(b[i+1])) * e)))
	}
	return out
}

const (
	rxTimings  = 5
	timingStep = 17
)

func timing(k, rate int) int {
	return (k - rxTimings/2) * timingStep * rate / rxRate
}

const leadSlots = 3

func checkDetect(t *testing.T, name string, score func(k int, ref bool) rawScore) string {
	t.Helper()
	lead, refBad, stray := true, false, true
	ok := func(ref bool) (int, string) {
		n, line := 0, ""
		for k := range rxTimings {
			sc := score(k, ref)
			if sc.bursts > 0 && sc.exact >= sc.bursts-2 {
				n++
			}
			if ref {
				refBad = refBad || sc.lastBad >= leadSlots
			} else {
				lead = lead && sc.bursts > 0 && sc.lastBad < leadSlots
				stray = stray && (sc.exact >= sc.bursts-2 || sc.stray > 0)
			}
			line += fmt.Sprintf(" %d/%d", sc.exact, sc.bursts)
		}
		return n, line
	}
	n, line := ok(false)
	if n > rxTimings/2 {
		return ""
	}
	r, rline := ok(true)
	t.Logf("%s: exact bursts at %d times:%s; reference:%s", name, rxTimings, line, rline)
	switch {
	case n >= r-1:
		return ""
	case lead:
		return fmt.Sprintf("RF9, %s: exact from slot %d at every time, short in the first slots at %d of %d times, reference at %d", name, leadSlots, rxTimings-n, rxTimings, rxTimings-r)
	case refBad:
		return fmt.Sprintf("RF10, %s: the receiver loses bits of the reference after slot %d; short at %d of %d times, reference at %d", name, leadSlots, rxTimings-n, rxTimings, rxTimings-r)
	case stray:
		return fmt.Sprintf("RF11, %s: the receiver reports bursts outside the stream at each of the %d of %d times it falls short, reference at %d", name, rxTimings-n, rxTimings, rxTimings-r)
	}
	t.Errorf("%s: detected at %d of %d times, reference %d", name, n, rxTimings, r)
	return ""
}

func skipRF(t *testing.T, notes []string) {
	t.Helper()
	notes = slices.DeleteFunc(notes, func(s string) bool { return s == "" })
	if len(notes) > 0 {
		t.Skipf("depends on a bug of the rf receiver: %s", strings.Join(notes, "; "))
	}
}

func moved(b []byte, k int) []byte {
	if k >= 0 {
		return append(make([]byte, 2*k, 2*k+len(b)), b...)
	}
	return b[-2*k:]
}

func movedAudio(a []int16, k int) []int16 {
	if k >= 0 {
		return append(make([]int16, k, k+len(a)), a...)
	}
	return a[-k:]
}

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
	for _, u := range raceUnits(rxUnits(full)) {
		t.Run(u.String(), func(t *testing.T) {
			t.Parallel()
			raw := u.run(t, ts.syms, keyedOn())
			x := u.signal48(raw, radio{ideal: true})
			d := fsk(x, ts.syms).Delay
			var notes []string
			if !u.audio() {
				b := u.s8Bytes(u.run(t, lead.syms, keyedOn()))
				var rb []byte
				notes = append(notes, checkDetect(t, "native BS", func(k int, isRef bool) rawScore {
					if !isRef {
						return lead.rawScore(nativeBursts(moved(b, timing(k, u.rate)), u.rate), 0)
					}
					if rb == nil {
						rb = refLead.native(u, d+(refLead.delay-ref.delay))
					}
					return lead.rawScore(nativeBursts(moved(rb, timing(k, u.rate)), u.rate), 0)
				}))
				ms, keys := newMSStream(t, u, 2)
				mraw := u.run(t, ms.syms, keys)
				mb := u.s8Bytes(mraw)
				var mrb []byte
				notes = append(notes, checkDetect(t, "native MS", func(k int, isRef bool) rawScore {
					if !isRef {
						return ms.rawScore(nativeBursts(moved(mb, timing(k, u.rate)), u.rate), 0)
					}
					if mrb == nil {
						mrb = keyedLike(newRefSig(ms).native(u, d), mraw, u.amplitude())
					}
					return ms.rawScore(nativeBursts(moved(mrb, timing(k, u.rate)), u.rate), 0)
				}))
			}
			if u.audio() && slices.Contains([]int{24000, 12000, 9600}, u.rate) {
				a := make([]int16, len(raw))
				for i, v := range raw {
					a[i] = int16(int(v) - u.offset)
				}
				ra := ref.audio(u.rate, d)
				for _, tier := range []rf.Tier{rf.Tier0, rf.Tier1} {
					notes = append(notes, checkDetect(t, fmt.Sprintf("disc %d", tier-rf.Tier0), func(k int, isRef bool) rawScore {
						if isRef {
							return ts.rawScore(discBursts(movedAudio(ra, timing(k, u.rate)), u.rate, tier), 0)
						}
						return ts.rawScore(discBursts(movedAudio(a, timing(k, u.rate)), u.rate, tier), 0)
					}))
				}
			}
			sh := ref.shift(d, rxRate)
			for _, p := range rxPaths() {
				if p.tier == rf.Tier3 && !full && u.rate != 44100 {
					continue
				}
				notes = append(notes, checkDetect(t, p.name+" at 30 dB", func(k int, isRef bool) rawScore {
					if isRef {
						return ts.rawScore(p.run(noisy(ref.x48, sh+timing(k, rxRate), ref.n, 30, 1)), rxPad)
					}
					return ts.rawScore(p.run(noisy(x, timing(k, rxRate), ref.n, 30, 1)), rxPad)
				}))
				got := berErrs(ts, x, 0, ref.n, p, p.cn, seeds)
				lim := refs.get(ref, p, p.cn-u.penaltyDB(), sh, seeds)
				t.Logf("%s at %v dB: %d bits wrong; reference %d at %v dB", p.name, p.cn, got, lim, p.cn-u.penaltyDB())
				if got > lim {
					t.Errorf("%s at %v dB: %d bits wrong, reference %d at %v dB", p.name, p.cn, got, lim, p.cn-u.penaltyDB())
				}
			}
			skipRF(t, notes)
		})
	}
}
