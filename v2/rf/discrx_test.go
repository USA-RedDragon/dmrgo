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

// audioTaps returns the taps of the IF filter of discAudio at basebandRate:
// a Hamming-windowed low pass with its cutoff at 6.25 kHz, half a 12.5 kHz
// channel.
func audioTaps() []float64 {
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
// basebandRate: the IF filter of audioTaps, a limiter and discriminator,
// and a converter at discRate with gain audioGain and offset audioOffset.
// Below the FM threshold its output has the clicks of a real one.
func discAudio(x []complex64) []int16 {
	h := audioTaps()
	out := make([]int16, 0, len(x)/2)
	var prev complex128
	for i := 0; i+len(h) <= len(x); i += 2 {
		var y complex128
		for k, t := range h {
			y += complex128(x[i+k]) * complex(t, 0)
		}
		d := y * complex(real(prev), -imag(prev))
		prev = y
		hz := math.Atan2(imag(d), real(d)) * discRate / (2 * math.Pi)
		out = append(out, int16(max(min(math.Round(hz*audioGain+audioOffset), math.MaxInt16), math.MinInt16)))
	}
	return out
}

// receiveAudio runs the discriminator samples a through a DiscReceiver.
func receiveAudio(t testing.TB, a []int16, opt *options) []Burst {
	t.Helper()
	var out []Burst
	r, err := NewDisc(DiscConfig{OnBurst: func(b Burst) { out = append(out, b) }, opt: opt})
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
	got := receiveAudio(t, discAudio(noisyBaseband(slots, 1400, 30, 1)), nil)
	sc := scoreBench(truth, got)
	for _, k := range []string{"ambe", "csbk", "rate12", "rate34", "sync", "emb", "slottype"} {
		// The receiver locks to the SYNC of the first slot, which it does
		// not decode.
		miss := 0
		if k == "sync" || k == "slottype" {
			miss = 1
		}
		if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth-miss {
			t.Errorf("%s: %d of %d recovered at 30 dB", k, s.Exact, s.Truth)
		}
	}
}

func TestNewDiscConfig(t *testing.T) {
	t.Parallel()
	if _, err := NewDisc(DiscConfig{Hz: 48000}); err == nil {
		t.Error("NewDisc took 48 kHz")
	}
	if _, err := NewDisc(DiscConfig{MaxWrite: -1}); err == nil {
		t.Error("NewDisc took a negative MaxWrite")
	}
}
