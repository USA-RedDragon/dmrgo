package rf

import (
	"math"
	"testing"
)

// lowIQ returns the baseband x at basebandRate as a radio chip delivers it
// at hz: a 255-tap Kaiser-windowed sinc (beta 8) with its -6 dB point at
// 0.45 hz, centered, and every basebandRate/hz-th output.
func lowIQ(x []complex64, hz int) []complex64 {
	const n, beta = 255, 8.0
	fc := 0.45 * float64(hz) / basebandRate
	i0 := func(v float64) float64 {
		s, t := 1.0, 1.0
		for k := 1; k < 60; k++ {
			t *= v / (2 * float64(k))
			s += t * t
		}
		return s
	}
	h := make([]float64, n)
	var sum float64
	for i := range h {
		k := float64(i - n/2)
		s := 2 * fc
		if k != 0 {
			s = math.Sin(2*math.Pi*fc*k) / (math.Pi * k)
		}
		r := 2*float64(i)/float64(n-1) - 1
		h[i] = s * i0(beta*math.Sqrt(max(0, 1-r*r))) / i0(beta)
		sum += h[i]
	}
	step := basebandRate / hz
	y := make([]complex64, len(x)/step)
	for o := range y {
		var re, im float64
		for k, t := range h {
			if j := o*step + k - n/2; j >= 0 && j < len(x) {
				re += float64(real(x[j])) * t / sum
				im += float64(imag(x[j])) * t / sum
			}
		}
		y[o] = complex(float32(re), float32(im))
	}
	return y
}

// TestLowIQReceiver checks that a LowIQReceiver in caller memory allocates
// nothing in Init and WriteBaseband and, with the settings of the audio
// Tier 0 core, decodes every voice frame of a BS outbound call at 1400 Hz
// offset at 30 dB, at 9.6 and 12 kHz.
func TestLowIQReceiver(t *testing.T) {
	slots, _ := benchStream(t, 3)
	x := noisyBaseband(slots, 1400, 30, 1)
	o := discOptions()
	o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
	for _, hz := range []int{9600, 12000} {
		y := lowIQ(x, hz)
		var r LowIQReceiver
		var got []Burst
		on := func(b *Burst) { got = append(got, *b) }
		if n := testing.AllocsPerRun(1, func() {
			if err := r.Init(LowIQConfig{Hz: hz, OnBurst: on, opt: &o}); err != nil {
				t.Fatal(err)
			}
		}); n != 0 {
			t.Errorf("%d Hz: Init: %v allocations", hz, n)
		}
		got = make([]Burst, 0, 4*len(slots)+16)
		const chunk = 1 << 9
		pos := 0
		if n := testing.AllocsPerRun(len(y)/chunk-2, func() {
			r.WriteBaseband(y[pos : pos+chunk])
			pos += chunk
		}); n != 0 {
			t.Errorf("%d Hz: %v allocations per WriteBaseband", hz, n)
		}
		r.WriteBaseband(y[pos:])
		r.Flush()
		if exact, total := exactFrames(slots, got); exact != total || total == 0 {
			t.Errorf("%d Hz: %d of %d voice frames exact", hz, exact, total)
		}
	}
}

// TestLowIQISI checks that a LowIQReceiver at 9.6 kHz decodes every voice
// frame of a long BS outbound call at 1400 Hz offset at 25 dB. Without
// cancelISI, a +1 between two +3 symbols, which the phase step at 9.6 kHz
// and the filter of the chip near the signal spread, lost a frame or more
// per call.
func TestLowIQISI(t *testing.T) {
	if testing.Short() {
		t.Skip("long call")
	}
	slots, _ := benchStream(t, 30)
	o := discOptions()
	o.MidTiming, o.Front96, o.BSSuspectRun, o.RawSearch = true, true, 3, 2
	for seed := uint64(1); seed <= 3; seed++ {
		y := lowIQ(noisyBaseband(slots, 1400, 25, seed), iq0LowHz)
		var r LowIQReceiver
		var got []Burst
		if err := r.Init(LowIQConfig{OnBurst: func(b *Burst) { got = append(got, *b) }, opt: &o}); err != nil {
			t.Fatal(err)
		}
		r.WriteBaseband(y)
		r.Flush()
		if exact, total := exactFrames(slots, got); exact != total || total == 0 {
			t.Errorf("seed %d: %d of %d voice frames exact", seed, exact, total)
		}
	}
}

// TestLowIQConfig checks the errors of LowIQReceiver.Init.
func TestLowIQConfig(t *testing.T) {
	var r LowIQReceiver
	on := func(*Burst) {}
	for _, c := range []struct {
		cfg   LowIQConfig
		field string
	}{
		{LowIQConfig{Hz: 48000, OnBurst: on}, "Hz"},
		{LowIQConfig{Hz: 24000, OnBurst: on}, "Hz"},
		{LowIQConfig{}, "OnBurst"},
	} {
		err := r.Init(c.cfg)
		if e, ok := err.(*ConfigError); !ok || e.Field != c.field {
			t.Errorf("%+v: got %v, want a ConfigError for %s", c.cfg, err, c.field)
		}
	}
	if err := r.Init(LowIQConfig{OnBurst: on}); err != nil {
		t.Errorf("default Hz: %v", err)
	}
}

// TestIQLowHalfRun checks that iqLowRx.halfRun gives the frequency of a
// tone within 2 units of iqUnitsPerHz and the error of the rounding of the
// input, at frequencies up to 0.49 of the rate, at any phase and at
// amplitudes from 64 to full scale.
func TestIQLowHalfRun(t *testing.T) {
	var r iqLowRx
	if err := r.init(9600, func(*Burst) {}, nil); err != nil {
		t.Fatal(err)
	}
	var x [iqLowBuf]uint32
	for hz := -4700.0; hz <= 4700; hz += 37 {
		for _, a := range []float64{64, 1000, 32767} {
			for p := 0.0; p < 6.3; p += 0.7 {
				for i := range x {
					v := p + 2*math.Pi*hz/9600*float64(i)
					x[i] = uint32(uint16(int16(math.Round(a*math.Cos(v))))) | uint32(int16(math.Round(a*math.Sin(v))))<<16
				}
				r.q, r.n = 0, 0
				r.halfRun(x[:])
				for i, v := range r.buf[1:] {
					if d := float64(v) - hz*iqUnitsPerHz; math.Abs(d) > 2+6000/a {
						t.Fatalf("%v Hz, amplitude %v, phase %v: sample %d is %d", hz, a, p, i+1, v)
					}
				}
			}
		}
	}
}

func TestMod64(t *testing.T) {
	for _, n := range []uint32{slotSymbols, 2 * slotSymbols} {
		for _, k := range []int64{0, 1, -1, 143, 144, -144, -145, 287, 288, 1 << 32, -1 << 32, 1<<40 + 77, -(1<<40 + 77), 1<<62 + 5, -1 << 63, 1<<63 - 1} {
			want := k % int64(n)
			if want < 0 {
				want += int64(n)
			}
			if got := mod64(k, n); int64(got) != want {
				t.Errorf("mod64(%d, %d) = %d, want %d", k, n, got, want)
			}
		}
	}
}
