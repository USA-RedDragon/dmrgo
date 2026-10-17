package rf

import (
	"math"
	"testing"
)

// Settings of discAudio: the gain of its converter, in units per Hz, and
// its offset in units.
const (
	audioGain   = 0.6
	audioOffset = 300
)

// audioIFTaps returns the taps of the IF filter of discAudio at basebandRate:
// a Hamming-windowed low pass with its cutoff at 6.25 kHz, half a 12.5 kHz
// channel.
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

// discAudio returns the output of an FM receiver for the baseband x at
// basebandRate, sampled at hz: the IF filter of audioIFTaps, a limiter
// and discriminator at basebandRate, the anti-alias filter of a converter
// at hz, with its cutoff at 0.42 hz, and the converter with gain audioGain
// and offset audioOffset. Below the FM threshold its output has the clicks
// of a real one.
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

// lowpassTaps returns n taps of a Hamming-windowed low pass with cutoff
// fc, a fraction of the sample rate, and unit gain at DC.
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

// receiveAudio runs the discriminator samples a at hz through a
// DiscReceiver.
func receiveAudio(t testing.TB, a []int16, hz int, opt *options) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: hz, OnBurst: func(b Burst) { out = append(out, b) }, opt: opt})
	if err != nil {
		t.Fatal(err)
	}
	for k := 0; k < len(a); k += 1024 {
		r.Write(a[k:min(k+1024, len(a))])
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
			// The receiver locks to the SYNC of the first slot, which it
			// does not decode.
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
	r, err := NewDisc(DiscConfig{Hz: 9600, MaxWrite: 1024})
	if err != nil {
		t.Fatal(err)
	}
	var want []Burst
	for k := 0; k < len(a); k += 1024 {
		want = append(want, r.Write(a[k:min(k+1024, len(a))])...)
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
