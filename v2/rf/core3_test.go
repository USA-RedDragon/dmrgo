//go:build !tinygo

package rf

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

func TestCoreTier3(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 4)
	x := noisyBaseband(slots, 1400, 12, 1)
	iq := mustNew(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8})
	var got []Burst
	for k := 0; k < len(x); k += 4096 {
		got = append(got, must(iq.writeBB(x[k:min(k+4096, len(x))]))...)
	}
	got = append(got, iq.flushAll()...)
	if exact, total := exactFrames(slots, got); exact != total || total == 0 {
		t.Errorf("I/Q: %d of %d voice frames exact", exact, total)
	}
	a := discAudio(x, 9600)
	ar := coreAudio3(t, AudioRxConfig{Rate: 9600, Tier: Tier3})
	got = ar.AppendFlush(ar.AppendBursts(nil, a))
	if exact, total := exactFrames(slots, got); exact != total || total == 0 {
		t.Errorf("audio: %d of %d voice frames exact", exact, total)
	}
	if e, _ := ar.ext.(*softExt); e == nil || e.coh == nil || e.coh.ex == nil || !e.ph.useHeld {
		t.Error("audio core Tier 3 runs without the exact stage and the held unit")
	}
}

func coreAudio3(t testing.TB, cfg AudioRxConfig) *AudioReceiver {
	t.Helper()
	cfg.Tier = Tier3
	r, err := NewAudioReceiver(&cfg)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestCohSlotFit(t *testing.T) {
	t.Parallel()
	opt := tierOptions(Tier3)
	if !opt.CohSlotFit {
		t.Fatal("core Tier 3 I/Q runs without the slot timing fit")
	}
	s := newCohStageOf(&opt)
	rng := rand.New(rand.NewPCG(1, 2))
	period := float64(slotSymbols) * s.sps * (1 + 500e-6)
	var q float64
	n := 0
	for k := int64(0); k < 400; k += 2 {
		truth := 1e6 + period*float64(k)
		pt, ok := s.slotPredict(k)
		s.t0 = truth + 0.2*rng.NormFloat64()
		s.slotMeasure(k, pt, ok)
		if k >= 100 {
			q += (s.t0 - truth) * (s.t0 - truth)
			n++
		}
	}
	if rms := math.Sqrt(q / float64(n)); rms > 0.08 {
		t.Errorf("slot fit timing rms %.3f samples from measurements of rms 0.2", rms)
	}
	pt, ok := s.slotPredict(400)
	if !ok {
		t.Fatal("no slot timing prediction after 200 measurements")
	}
	s.t0 = pt + 3
	s.slotMeasure(400, pt, ok)
	if math.Abs(s.t0-pt-3) > 1e-9 {
		t.Errorf("a 3 sample timing jump moved the fit to %.3f instead of restarting it", s.t0-pt)
	}
}

func TestCoreTier3EarlySlots(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	sig := clockSignal{clockBaseband(slots, 5000, 0), 0}
	if rate, _ := earlyExact(truth, 1, sig); rate < 0.78 {
		t.Errorf("core Tier 3 at +5000 Hz, 1 dB: %.3f of the AMBE frames in the first 12 slots exact", rate)
	}
	if rate, _ := earlyExact(truth, 0, sig); rate < 0.6 {
		t.Errorf("core Tier 3 at +5000 Hz, 0 dB: %.3f of the AMBE frames in the first 12 slots exact", rate)
	}
}

func TestCoreTier3EarlySlotsClock(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	rate, miss := earlyExact(truth, 3, clockSignal{clockBaseband(slots, 5000, -500), -500}, clockSignal{clockBaseband(slots, 5000, 500), 500})
	if rate < 0.9 {
		t.Errorf("core Tier 3 at +5000 Hz, +-500 ppm, 3 dB: %.3f of the AMBE frames in the first 12 slots exact", rate)
	}
	if miss > 0.6 {
		t.Errorf("core Tier 3 at +-500 ppm: the oldest replayed slot sits up to %.2f symbols off", miss)
	}
}

type clockSignal struct {
	sig benchSignal
	ppm float64
}

func clockBaseband(slots []rftx.BSSlot, offsetHz, ppm float64) benchSignal {
	fm, env := rftx.ModulateBS(slots)
	if ppm != 0 {
		fm, env = resampleFM(fm, env, ppm)
	}
	pad := int(benchPad * basebandRate)
	s := benchSignal{make([]float64, len(fm)+2*pad), make([]float64, len(fm)+2*pad)}
	var ph float64
	for k := range fm {
		ph += 2 * math.Pi * (fm[k] + offsetHz) / basebandRate
		s.re[pad+k], s.im[pad+k] = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
	}
	return s
}

func earlyExact(truth []benchSlot, cn float64, sigs ...clockSignal) (rate, miss float64) {
	start := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	n := int((start + 16*slotSeconds) * basebandRate)
	lookback := float64(tierOptions(Tier3).ExtLookback * slotSymbols)
	var exact, total atomic.Int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, c := range sigs {
		sig := c.sig
		sig.re, sig.im = sig.re[:n], sig.im[:n]
		for seed := range uint64(24) {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8})
				x := sig.noisy(cn, 7000+seed)
				var got []Burst
				for k := 0; k < len(x); k += 4096 {
					got = append(got, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
				}
				got = append(got, r.flushAll()...)
				e, n := ambeInSlots(truth, got, 12)
				exact.Add(int64(e))
				total.Add(int64(n))
				var stretch float64
				if h, ok := r.full.(*rxHost); ok && h.iqd != nil {
					if x, ok := h.iqd.d.ext.(*softExt); ok {
						stretch = float64(x.stretch)
					}
				}
				mu.Lock()
				miss = max(miss, math.Abs(stretch+c.ppm*1e-6)*lookback)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	return float64(exact.Load()) / float64(total.Load()), miss
}

func ambeInSlots(truth []benchSlot, got []Burst, n int) (exact, total int) {
	start := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	byIndex := map[int]Burst{}
	for _, r := range got {
		if i := int(math.Round((r.Seconds - start) / slotSeconds)); i >= 0 && i < n && math.Abs(r.Seconds-start-float64(i)*slotSeconds) < 0.002 {
			byIndex[i] = r
		}
	}
	for i, ts := range truth[:n] {
		if ts.kind != "voice" {
			continue
		}
		r, have := byIndex[i]
		data, _ := r.Decoded()
		want, _ := ts.burst.AMBEFrames()
		gotF, _ := data.AMBEFrames()
		for j := range want {
			total++
			if have && gotF[j] == want[j] {
				exact++
			}
		}
	}
	return exact, total
}

func TestCoreTier3Ring(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier3, Format: IQFormatCU8})
	x := noisyBaseband(slots, 1400, 40, 1)
	for k := 0; k < len(x); k += 4096 {
		must(r.writeBB(x[k:min(k+4096, len(x))]))
	}
	h, _ := r.full.(*rxHost)
	if h == nil || h.iqd == nil || h.iqd.cHist == nil {
		t.Fatal("core Tier 3 I/Q runs without the coherent ring")
	}
	q := h.iqd
	m := int64(len(q.cHist) - 1)
	end := q.yN - q.cLag
	best, bestLag := 0.0, 0
	for lag := -2; lag <= 2; lag++ {
		var c complex128
		var ey, ec float64
		for k := end - 4096; k < end; k++ {
			y, v := complex128(q.yHist[k&m]), complex128(q.cHist[(k+int64(lag))&m])
			c += y * cmplx.Conj(v)
			ey += real(y)*real(y) + imag(y)*imag(y)
			ec += real(v)*real(v) + imag(v)*imag(v)
		}
		if rho := cmplx.Abs(c) / math.Sqrt(ey*ec); rho > best {
			best, bestLag = rho, lag
		}
	}
	if bestLag != 0 || best < 0.9 {
		t.Errorf("coherent ring best matches the narrow ring at lag %d with correlation %.3f", bestLag, best)
	}
}

func TestCoreTier3Lag(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("long stream")
	}
	long, _ := benchStream(t, 16)
	x := noisyBaseband(long, 1400, 12, 3)
	x = x[int(0.9*basebandRate):]
	const chunk = 16
	for _, hz := range []int{9600, 48000} {
		a := discAudio(x, hz)
		fed, worst, n := 0, 0.0, 0
		r := coreAudio3(t, AudioRxConfig{Rate: hz, Tier: Tier3, OnBursts: func(bs []Burst) {
			for _, b := range bs {
				worst, n = max(worst, float64(fed)-(b.Seconds+burstSeconds)*float64(hz)), n+1
			}
		}})
		for k := 0; k < len(a); k += chunk {
			fed = min(k+chunk, len(a))
			_, _ = r.Write(a[k:fed])
		}
		_ = r.Flush()
		if n == 0 || worst > float64(r.Lag()+chunk) {
			t.Errorf("audio at %d Hz: %d bursts, a lag of %.0f samples, Lag %d", hz, n, worst, r.Lag())
		}
	}
	const rate = 48000
	in := cf32(dgUpsample(x, rate))
	fed, worst, n := 0, 0.0, 0
	r := mustNew(RxConfig{Rate: rate, Tier: Tier3, Format: IQFormatCF32, OnBursts: func(bs []Burst) {
		for _, b := range bs {
			worst, n = max(worst, float64(fed/8)-(b.Seconds+burstSeconds)*float64(rate)), n+1
		}
	}})
	step := 8 * max(chunk, rate/3000)
	for k := 0; k < len(in); k += step {
		fed = min(k+step, len(in))
		_, _ = r.Write(in[k:fed])
	}
	_ = r.Flush()
	if n == 0 || worst > float64(r.Lag()+step/8) {
		t.Errorf("I/Q at %d Hz: %d bursts, a lag of %.0f samples, Lag %d", rate, n, worst, r.Lag())
	}
}

func TestCoreTier3WideTune(t *testing.T) {
	t.Parallel()
	run := func(tuneAt int) []complex64 {
		var d iqDisc
		d.init(2, iqFilter{})
		d.coh = newIQCohFIR(cohWideTaps, cohWideKaiser, cohWideCutHz)
		if tuneAt < 0 {
			d.tune(1500)
		}
		var out []complex64
		for i := range 4800 {
			if i == tuneAt {
				d.tune(1500)
			}
			p := 2 * math.Pi * 1700 * float64(i) / basebandRate
			if _, _, ok := d.push(complex64(cmplx.Rect(1, p))); ok {
				out = append(out, d.coh.y)
			}
		}
		return out
	}
	ref, got := run(-1), run(2001)
	rot := got[len(got)-1] / ref[len(ref)-1]
	worst := 0.0
	for i := 1001; i < len(got); i++ {
		worst = max(worst, cmplx.Abs(complex128(got[i]-rot*ref[i])))
	}
	if worst > 1e-3 {
		t.Errorf("wide filter output after a tune differs from a filter tuned from the start by up to %.4f", worst)
	}
}

func TestCoreTier3Codec2Call(t *testing.T) {
	t.Parallel()
	for _, late := range []bool{false, true} {
		x, want, start := codec2Call(t, 4, late, 30)
		r := mustNew(RxConfig{Tier: Tier3, Rate: basebandRate, Format: IQFormatCU8})
		var got []Burst
		for k := 0; k < len(x); k += 4096 {
			got = append(got, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
		}
		got = append(got, r.flushAll()...)
		if bad := codec2Bad(got, want, start, late); bad != 0 {
			t.Errorf("late entry %v: %d of %d Codec2 frames differ", late, bad, len(want))
		}
	}
}

func TestCoreTier3SlowClock(t *testing.T) {
	t.Parallel()
	cfg := rftx.SeqConfig{ColorCode: 1, Source: 3191868, Dest: 9990, BS: 310123, Wakeups: 2, Message: "DMRGO TEST MESSAGE", Superframes: 4}
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm0, env0 := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	for _, lead := range []float64{0, 0.2} {
		pad := int(lead * basebandRate)
		tx := rftx.ToIQ(append(make([]float64, pad), fm0...), append(make([]float64, pad), env0...), 2e6, 0, 0.9)
		for _, off := range []float64{0, 3000} {
			for _, ppm := range []float64{500, -500} {
				iq := rftx.Channel(tx, 2e6/(1+ppm*1e-6), 2.048e6, off, 25, 5)
				r := mustNew(RxConfig{Rate: 2.048e6, Tier: Tier3, Format: IQFormatCU8})
				got := r.AppendFlush(r.AppendBursts(nil, iq))
				if len(got) != len(built.Bursts) {
					t.Errorf("%.1f s lead %+.0f Hz %+.0f ppm: %d bursts, want %d", lead, off, ppm, len(got), len(built.Bursts))
					continue
				}
				bad := 0
				for i := range got {
					if got[i].Data != built.Bursts[i] && (lead > 0 || i >= 3) {
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("%.1f s lead %+.0f Hz %+.0f ppm: raw bits differ in %d of %d bursts", lead, off, ppm, bad, len(got))
				}
			}
		}
	}
}

func mxReceiveCore(t testing.TB, in mxInput) []Burst {
	t.Helper()
	if in.audio != nil {
		r := coreAudio3(t, AudioRxConfig{Rate: in.hz, Tier: Tier3})
		var got []Burst
		for k := 0; k < len(in.audio); k += 1024 {
			got = r.AppendBursts(got, in.audio[k:min(k+1024, len(in.audio))])
		}
		return r.AppendFlush(got)
	}
	r, err := newReceiver(RxConfig{Rate: in.rate, Tier: Tier3, Format: in.iqFmt}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []Burst
	for k := 0; k < len(in.iq); k += 8192 {
		got = r.AppendBursts(got, in.iq[k:min(k+8192, len(in.iq))])
	}
	return r.AppendFlush(got)
}

func TestCoreTier3ReplayTime(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"agc.noise.+4000.ms.data12.+500ppm.low.cu8-2048k.40dB",
		"gap.none.-4000.ms2.data1.+500ppm.nom.cs16-48k.inf",
	} {
		row := mxRowOf(t, name)
		s, err := mxBuild(row)
		if err != nil {
			t.Fatal(err)
		}
		in := mxInputOf(s, row)
		res := &mxResult{}
		mxScore(s, in, Tier3, mxReceiveCore(t, in), mxOuterHz, row.val(mxCN) != "inf", res)
		if res.fail["seconds"] || res.fail["missing"] {
			t.Errorf("%s: %s", name, strings.Join(res.detail, " | "))
		}
	}
}

func TestCoreTier3BurstContinuity(t *testing.T) {
	t.Parallel()
	if o := tierOptions(Tier3); o.CohNoCross || o.CohNoNext {
		t.Error("core Tier 3 I/Q detects each burst without the trellis of the burst before and after it")
	}
}

func TestDiscTier3WriteFlush(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 12, 7)
	for _, hz := range []int{2 * symbolRate, discRate} {
		a := discAudio(x, hz)
		a = a[:len(a)*2/3]
		var want []Burst
		w, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: Tier3, OnBursts: each(func(b *Burst) { want = append(want, *b) })})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(a); k += 4096 {
			must(w.writeAll(a[k:min(k+4096, len(a))]))
		}
		w.flushAll()
		r, err := NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: Tier3})
		if err != nil {
			t.Fatal(err)
		}
		var got []Burst
		for k := 0; k < len(a); k += 4096 {
			got = append(got, must(r.writeAll(a[k:min(k+4096, len(a))]))...)
		}
		got = append(got, r.flushAll()...)
		if len(got) != len(want) {
			t.Fatalf("at %d Hz Write and Flush return %d bursts, OnBurst takes %d", hz, len(got), len(want))
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("at %d Hz burst %d of Write and Flush differs from that of OnBurst", hz, i)
			}
		}
		if n := r.flushBursts(); n > 1050 {
			t.Errorf("at %d Hz Tier3 Flush has room for %d bursts, the docs say up to 1050", hz, n)
		}
	}
}

func TestCoreTier3WeakDataPlaced(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	sig := cleanBaseband(slots, 1400)
	first := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
	drops, n := 0, 0
	for _, cn := range []float64{-3, -2.5} {
		for s := range uint64(3) {
			have := map[int]bool{}
			for _, b := range benchReceive(t, sig.noisy(cn, 60+s), benchEntry{benchIQ, Tier3}) {
				if i := int(math.Round((b.Seconds - first) / slotSeconds)); math.Abs(b.Seconds-first-float64(i)*slotSeconds) < 0.002 {
					have[i] = true
				}
			}
			for i, ts := range truth {
				if ts.kind == "data" && have[i-2] && have[i-1] {
					n++
					if !have[i] {
						drops++
					}
				}
			}
		}
	}
	if 20*drops > n {
		t.Errorf("%d of %d data bursts inside a stream not placed", drops, n)
	}
}
