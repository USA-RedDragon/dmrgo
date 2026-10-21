package rf

import (
	"errors"
	"math"
	"math/bits"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2"
)

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

func TestLowIQISI(t *testing.T) {
	t.Parallel()
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

func TestLowIQConfig(t *testing.T) {
	t.Parallel()
	var r LowIQReceiver
	on := func(*Burst) {}
	for _, c := range []struct {
		cfg   LowIQConfig
		field string
	}{
		{LowIQConfig{Hz: 48000, OnBurst: on}, "LowIQConfig.Hz"},
		{LowIQConfig{Hz: 24000, OnBurst: on}, "LowIQConfig.Hz"},
		{LowIQConfig{}, "LowIQConfig.OnBurst"},
	} {
		err := r.Init(c.cfg)
		if e := (*dmrgo.ConfigError)(nil); !errors.As(err, &e) || e.Field != c.field {
			t.Errorf("%+v: got %v, want a ConfigError for %s", c.cfg, err, c.field)
		}
	}
	if err := r.Init(LowIQConfig{OnBurst: on}); err != nil {
		t.Errorf("default Hz: %v", err)
	}
}

func TestLowIQWeakNoiseKeepsTune(t *testing.T) {
	t.Parallel()
	for _, sigma := range []float64{4, 12, 24} {
		var r LowIQReceiver
		if err := r.Init(LowIQConfig{OnBurst: func(*Burst) {}}); err != nil {
			t.Fatal(err)
		}
		rng := rand.New(rand.NewPCG(1, uint64(sigma)))
		x := make([]uint32, 4096)
		for range 5 {
			for i := range x {
				re, im := int16(math.Round(rng.NormFloat64()*sigma)), int16(math.Round(rng.NormFloat64()*sigma))
				x[i] = uint32(uint16(re)) | uint32(uint16(im))<<16
			}
			r.WriteIQ16(x)
		}
		if r.r.hz != 0 {
			t.Errorf("noise of %v LSB tuned the AFC to %.0f Hz", sigma, r.r.hz)
		}
	}
}

func TestIQLowHalfRun(t *testing.T) {
	t.Parallel()
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
				tol := 2 + 6000/a
				if iqLowRecip {
					tol += 30
				}
				for i, v := range r.buf[1:] {
					if d := float64(v) - hz*iqUnitsPerHz; math.Abs(d) > tol {
						t.Fatalf("%v Hz, amplitude %v, phase %v: sample %d is %d", hz, a, p, i+1, v)
					}
				}
			}
		}
	}
}

func TestLagrangeAt(t *testing.T) {
	t.Parallel()
	p := [6]float32{0.3, -1.2, 2.5, 0.7, -3, 1.1}
	for i := 0; i <= lagrangeTabN; i++ {
		mu := float32(i) / lagrangeTabN
		want := lagrange6[site0](p[0], p[1], p[2], p[3], p[4], p[5], mu)
		if got := lagrangeAt(p[0], p[1], p[2], p[3], p[4], p[5], mu); math.Abs(float64(got-want)) > 1e-5 {
			t.Errorf("mu %v: %v, want %v", mu, got, want)
		}
	}
}

func TestMod64(t *testing.T) {
	t.Parallel()
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

func TestIQLowRecip(t *testing.T) {
	t.Parallel()
	seed := uint32(1)
	for k := 0; k < 200000; k++ {
		seed = seed*1664525 + 1013904223
		ax := seed>>1 | 1
		seed = seed*1664525 + 1013904223
		ay := uint32(uint64(seed) * uint64(ax) >> 32)
		z := uint(bits.LeadingZeros32(ax))
		x := ax << z
		r := ((ay << z) >> 16 * uint32(iqLowTab[x>>25])) >> 24
		if r > 63 {
			t.Fatalf("ax %d ay %d: step %d", ax, ay, r)
		}
		got := float64(iqLowTab[r]) * (2 * math.Pi / (1 << 16))
		if d := math.Abs(got - math.Atan2(float64(ay), float64(ax))); d > 0.02 {
			t.Fatalf("ax %d ay %d: angle off by %.4f rad", ax, ay, d)
		}
	}
}
