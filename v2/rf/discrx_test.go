package rf

import (
	"math"
	"testing"
)

const (
	audioGain   = 0.6
	audioOffset = 300
)

func audioIFTaps() []float64 {
	const n = 31
	h := make([]float64, n)
	fc := 6250.0 / basebandRate
	for i := range h {
		k := float64(i - n/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		h[i] = s * (0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/(n-1)))
	}
	return h
}

func discAudio(x []complex64, hz int) []int16 {
	h := audioIFTaps()
	step := basebandRate / hz
	aa := lowpassTaps(63, min(6250, 0.42*float64(hz))/basebandRate)
	disc := make([]float64, 0, len(x))
	var prev complex128
	for i := 0; i+len(h) <= len(x); i++ {
		var y complex128
		for k, t := range h {
			y += complex128(x[i+k]) * complex(t, 0)
		}
		d := y * complex(real(prev), -imag(prev))
		prev = y
		disc = append(disc, math.Atan2(imag(d), real(d))*basebandRate/(2*math.Pi))
	}
	out := make([]int16, 0, len(disc)/step)
	for i := 0; i+len(aa) <= len(disc); i += step {
		var hz float64
		for k, t := range aa {
			hz += disc[i+k] * t
		}
		out = append(out, int16(max(min(math.Round(hz*audioGain+audioOffset), math.MaxInt16), math.MinInt16)))
	}
	return out
}

func lowpassTaps(n int, fc float64) []float64 {
	h := make([]float64, n)
	var sum float64
	for i := range h {
		k := float64(i - n/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		h[i] = s * (0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1)))
		sum += h[i]
	}
	for i := range h {
		h[i] /= sum
	}
	return h
}

func receiveAudio(t testing.TB, a []int16, hz int, opt *options) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: hz, OnBurst: func(b *Burst) { out = append(out, *b) }, opt: opt})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		must(r.Write(a[k:min(k+1024, len(a))]))
	}
	r.Flush()
	return out
}

func TestDiscReceiverLoopback(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 6)
	x := noisyBaseband(slots, 1400, 30, 1)
	for _, hz := range []int{24000, 12000, 9600} {
		sc := scoreBench(truth, receiveAudio(t, discAudio(x, hz), hz, nil))
		for _, k := range []string{"ambe", "csbk", "rate12", "rate34", "sync", "emb", "slottype"} {
			miss := 0
			if k == "sync" || k == "slottype" {
				miss = 1
			}
			if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth-miss {
				t.Errorf("%d Hz, %s: %d of %d recovered at 30 dB", hz, k, s.Exact, s.Truth)
			}
		}
	}
}

func TestNewDiscConfig(t *testing.T) {
	t.Parallel()
	if _, err := NewDisc(DiscConfig{Hz: 48000}); err == nil {
		t.Error("NewDisc took 48 kHz")
	}
	if _, err := NewDisc(DiscConfig{Hz: 16000}); err == nil {
		t.Error("NewDisc took 16 kHz")
	}
	if _, err := NewDisc(DiscConfig{MaxWrite: -1}); err == nil {
		t.Error("NewDisc took a negative MaxWrite")
	}
	for _, hz := range []int{24000, 12000, 9600} {
		r, err := NewDisc(DiscConfig{Hz: hz})
		if err != nil {
			t.Fatal(err)
		}
		if sps := 2 * r.tm.step; float32(r.mfStep)*sps != samplesPerSy {
			t.Errorf("%d Hz: %d baseband samples per matched filter sample at %v samples per symbol", hz, r.mfStep, sps)
		}
	}
}

func TestDiscReceiverInitAppend(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 3)
	a := discAudio(noisyBaseband(slots, 1400, 12, 1), 9600)
	r, err := NewDisc(DiscConfig{Hz: 9600, Tier: Tier0, MaxWrite: 1024})
	if err != nil {
		t.Fatal(err)
	}
	var want []Burst
	for k := 0; k < len(a); k += 1024 {
		want = append(want, must(r.Write(a[k:min(k+1024, len(a))]))...)
	}
	want = append(want, r.Flush()...)
	var s DiscReceiver
	if err := s.Init(DiscConfig{Hz: 9600}); err != nil {
		t.Fatal(err)
	}
	var got []Burst
	for k := 0; k < len(a); k += 1024 {
		got = s.AppendBursts(got, a[k:min(k+1024, len(a))])
	}
	got = s.AppendFlush(got)
	if len(want) == 0 || len(got) != len(want) {
		t.Fatalf("AppendBursts returned %d bursts, Write %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("burst %d differs", i)
		}
	}
}

func TestRescaleSigns(t *testing.T) {
	t.Parallel()
	var r DiscReceiver
	if err := r.Init(DiscConfig{Hz: 9600}); err != nil {
		t.Fatal(err)
	}
	for range leanRing {
		r.symbol(120*discStepHz, 0)
	}
	r.rescale(1.0 / 64)
	voice, _, _ := sourceAt(0).SyncPatterns()
	var syms [syncSymbols]int8
	syncSyms(voice, &syms)
	for _, s := range syms {
		r.symbol(float32(s)*deviationHz, 0)
	}
	if got := r.signs & (1<<syncSymbols - 1); got != syncSigns[0] {
		t.Errorf("signs %06x, want %06x", got, syncSigns[0])
	}
}

func TestRescaleTiming(t *testing.T) {
	t.Parallel()
	var r DiscReceiver
	if err := r.Init(DiscConfig{Hz: 9600}); err != nil {
		t.Fatal(err)
	}
	const g = 1.0 / 16
	x := func(i int) float32 { return 3 * deviationHz * float32(math.Sin(math.Pi*float64(i)/2+0.3)) }
	for i := range 2000 {
		r.push(x(i))
	}
	want := g * g * r.tm.pow
	r.rescale(g)
	for i := 2000; i < 2010; i++ {
		r.push(g * x(i))
	}
	if p := r.tm.pow; p > 2*want || p < want/2 {
		t.Errorf("timing power %.0f, want about %.0f", p, want)
	}
}

func TestRingPosWrap(t *testing.T) {
	t.Parallel()
	for k := int64(1<<32 - 3*leanRing); k < 1<<32+3*leanRing; k++ {
		if got, want := ringPos(k), uint32(k%leanRing); got != want {
			t.Fatalf("ringPos(%d) = %d, want %d", k, got, want)
		}
	}
	if got, want := ringPos(1<<40+5), uint32((1<<40+5)%leanRing); got != want {
		t.Errorf("ringPos(1<<40+5) = %d, want %d", got, want)
	}
}

func TestDiscReceiverRingWrap(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	a := discAudio(noisyBaseband(slots, 1400, 12, 3), benchAudioHz)
	for _, tier := range []Tier{Tier0, Tier1, Tier2} {
		want := receiveAudioTier(t, a, tier)
		var got []Burst
		r, err := NewDisc(DiscConfig{Hz: benchAudioHz, Tier: tier, OnBurst: func(b *Burst) { got = append(got, *b) }})
		if err != nil {
			t.Fatal(err)
		}
		r.ns = 1<<32 - int64(len(a)/benchAudioHz*symbolRate/2)
		for k := 0; k < len(a); k += 1024 {
			must(r.Write(a[k:min(k+1024, len(a))]))
		}
		r.Flush()
		if len(got) != len(want) || len(want) < 50 {
			t.Fatalf("tier %d: %d bursts, want %d", tier-Tier0, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("tier %d: burst %d at %.3f s differs", tier-Tier0, i, want[i].Seconds)
			}
		}
	}
}

func TestLevelUp(t *testing.T) {
	t.Parallel()
	x := make([]int16, 8*frontBlock)
	for i := range x {
		x[i] = int16(math.Round(1200 * math.Sqrt2 * math.Sin(float64(i)*0.7)))
	}
	for _, up := range []bool{false, true} {
		opt := discOptions()
		opt.LevelUp = up
		var r DiscReceiver
		if err := r.Init(DiscConfig{Hz: discRate / 2, opt: &opt}); err != nil {
			t.Fatal(err)
		}
		_ = r.AppendBursts(nil, x)
		if g := r.f.gain; up != (g > 1.5) {
			t.Errorf("LevelUp %v: gain %.3f", up, g)
		}
	}
}
