//go:build !tinygo

package fsktx

import (
	"math"
	"math/cmplx"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/unpacked"
	"github.com/USA-RedDragon/dmrgo/v2/rf"
)

func audioTiers() []AudioConfig {
	return []AudioConfig{
		{Hz: 24000, Tier: Tier0}, {Hz: 48000, Tier: Tier0},
		{Hz: 24000, Tier: Tier1}, {Hz: 48000, Tier: Tier1}, {Hz: 44100, Tier: Tier1},
		{Hz: 48000, Tier: Tier2},
	}
}

func iqTiers() []IQConfig {
	return []IQConfig{
		{Rate: 48000, Tier: Tier0}, {Rate: 96000, Tier: Tier0},
		{Rate: 2400000, Tier: Tier1}, {Rate: 2000000, Tier: Tier1}, {Rate: 96000, Tier: Tier1},
		{Rate: 2400000, Tier: Tier2},
	}
}

func audioOf(t *testing.T, cfg AudioConfig, syms []int8) []int16 {
	t.Helper()
	a, err := NewAudio(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out []int16
	for _, s := range syms {
		out = a.AppendSymbol(out, s)
	}
	return out
}

func iqOf(t *testing.T, cfg IQConfig, syms []int8, on int) []complex128 {
	t.Helper()
	q, err := NewIQ(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var out8 []byte
	var out16 []int16
	for i, s := range syms {
		if i == on {
			q.Key(true)
		}
		if cfg.Rate >= 1e6 {
			out8 = q.AppendS8(out8, s)
		} else {
			out16 = q.AppendS16(out16, s)
		}
	}
	if cfg.Rate >= 1e6 {
		return s8IQ(out8)
	}
	return fromIQ(out16)
}

func toS8(x []complex128, rate int) []byte {
	g := 1.0
	if rate < 1e6 {
		g = 1.0 / 256
	}
	b := make([]byte, 2*len(x))
	for i, v := range x {
		b[2*i] = byte(int8(math.Round(real(v) * g)))
		b[2*i+1] = byte(int8(math.Round(imag(v) * g)))
	}
	return b
}

func s8IQ(b []byte) []complex128 {
	x := make([]complex128, len(b)/2)
	for i := range x {
		x[i] = complex(float64(int8(b[2*i])), float64(int8(b[2*i+1])))
	}
	return x
}

func TestDibits(t *testing.T) {
	t.Parallel()
	var b [33]byte
	for i := range b {
		b[i] = byte(i*73 + 5)
	}
	var bits [264]unpacked.Bit
	unpacked.UnpackAt(bits[:], b[:], 0)
	want := rftx.SymbolsFromBits(bits[:])
	got := Dibits(make([]int8, 132), b[:])
	for i := range want {
		if int(got[i]) != want[i] {
			t.Fatalf("symbol %d is %d, want %d", i, got[i], want[i])
		}
	}
	for n := range 132 {
		got := Dibits(make([]int8, n, 132), b[:])
		if len(got) != n/4*4 {
			t.Fatalf("a dst of %d takes %d symbols", n, len(got))
		}
		for i := range got {
			if int(got[i]) != want[i] {
				t.Fatalf("a dst of %d: symbol %d is %d, want %d", n, i, got[i], want[i])
			}
		}
	}
}

func TestAudioRoundTrip(t *testing.T) {
	t.Parallel()
	ts := newTestStream(t, 2, 20)
	for _, cfg := range audioTiers() {
		a := audioOf(t, cfg, ts.syms)
		if cfg.Hz == 24000 {
			var got []rf.Burst
			r, err := rf.NewAudioReceiver(&rf.AudioRxConfig{Rate: cfg.Hz, OnBursts: func(bs []rf.Burst) { got = append(got, bs...) }})
			if err != nil {
				t.Fatal(err)
			}
			check(r.Write(a))
			check(0, r.Flush())
			checkAll(t, cfg, ts.score(got, 0), 1)
		}
		x := dacFM(a, cfg.Hz, radio{})
		x48 := resample(x, dacRate, rxRate)
		checkAll(t, cfg, ts.score(receive(noisy(x48, 0, len(x48)+int(2*rxPad*rxRate), 30, 1), rf.Tier1), rxPad), 0)
	}
}

func checkAll(t *testing.T, cfg any, sc score, miss int) {
	t.Helper()
	if sc.voice == 0 || sc.voiceOK < sc.voice || sc.dataOK < sc.data-miss {
		t.Errorf("%+v: %d of %d voice frames and %d of %d data blocks", cfg, sc.voiceOK, sc.voice, sc.dataOK, sc.data)
	}
}

func TestIQRoundTrip(t *testing.T) {
	t.Parallel()
	ts := newTestStream(t, 2, 20).withLead(1200)
	for _, cfg := range iqTiers() {
		cfg.OffsetHz = 1200
		b := toS8(iqOf(t, cfg, ts.syms, ts.lead-6), cfg.Rate)
		rng := rand.New(rand.NewPCG(1, 2))
		for i := range b {
			b[i] = byte(int8(min(max(int(int8(b[i]))+rng.IntN(3)-1, math.MinInt8), math.MaxInt8)))
		}
		var got []rf.Burst
		r, err := rf.NewReceiver(&rf.RxConfig{Rate: cfg.Rate, Format: rf.IQFormatCS8, Tier: rf.Tier1, OnBursts: func(bs []rf.Burst) { got = append(got, bs...) }})
		if err != nil {
			t.Fatal(err)
		}
		for k := 0; k < len(b); k += 1 << 16 {
			check(r.Write(b[k:min(k+1<<16, len(b))]))
		}
		check(0, r.Flush())
		checkAll(t, cfg, ts.score(got, 0), 1)
	}
}

// ETSI EN 300 113 clause 7.4.3
func TestSpectrum(t *testing.T) {
	t.Parallel()
	ts := newTestStream(t, 3, 40)
	for _, cfg := range audioTiers() {
		x := dacFM(audioOf(t, cfg, ts.syms), cfg.Hz, radio{})
		checkSpectrum(t, cfg, newSpectrum(x[len(x)/8:len(x)*7/8], dacRate))
	}
	for _, cfg := range iqTiers() {
		x := iqOf(t, cfg, ts.syms, 0)
		checkSpectrum(t, cfg, newSpectrum(x[len(x)/8:len(x)*7/8], float64(cfg.Rate)))
	}
}

func checkSpectrum(t *testing.T, cfg any, sp spectrum) {
	t.Helper()
	acp, alt, mask := sp.acp(12500), sp.acp(25000), sp.maskMargin()
	if math.IsNaN(alt) && sp.rate >= 2*(25000+4250) {
		t.Errorf("%+v: no alternate channel power", cfg)
	}
	if acp > -57.4 || alt > -60 || mask < 0 {
		t.Errorf("%+v: adjacent %.1f dBc, alternate %.1f dBc, mask margin %.1f dB", cfg, acp, alt, mask)
	}
}

func TestFidelity(t *testing.T) {
	t.Parallel()
	ts := newTestStream(t, 2, 40)
	limit := map[Tier]float64{Tier0: 3, Tier1: 0.6, Tier2: 0.3}
	for _, cfg := range audioTiers() {
		x := dacFM(audioOf(t, cfg, ts.syms), cfg.Hz, radio{ideal: true})
		f := fsk(resample(x, dacRate, rxRate), ts.syms)
		if f.FSKError > limit[cfg.Tier] || math.Abs(f.DevError) > 5 {
			t.Errorf("%+v: %+v", cfg, f)
		}
	}
	for _, cfg := range iqTiers() {
		cfg.OffsetHz = -2500
		x := iqOf(t, cfg, ts.syms, 0)
		f := fsk(resample(x, cfg.Rate, rxRate), ts.syms)
		if f.FSKError > limit[cfg.Tier]+0.3 || math.Abs(f.DevError) > 5 || math.Abs(f.OffsetHz-cfg.OffsetHz) > 5 {
			t.Errorf("%+v: %+v", cfg, f)
		}
	}
}

// ETSI EN 300 113 clause 7.9; ETSI TS 102 361-1 clause 10.2.3.1.1
func TestRamp(t *testing.T) {
	t.Parallel()
	for _, cfg := range iqTiers() {
		if cfg.Rate > 100000 {
			cfg.Rate = 96000
		}
		q, err := NewIQ(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var out []int16
		for range 30 {
			out = q.AppendS16(out, 0)
		}
		on := len(out) / 2
		q.Key(true)
		for range 200 {
			out = q.AppendS16(out, 0)
		}
		q.Key(false)
		for range 60 {
			out = q.AppendS16(out, 0)
		}
		x := fromIQ(out)
		p := real(x[on+cfg.Rate/100] * cmplx.Conj(x[on+cfg.Rate/100]))
		if tr := transient(resample(x, cfg.Rate, dacRate), dacRate, p); tr > -50 {
			t.Errorf("%+v: adjacent channel transient power %.1f dBc", cfg, tr)
		}
		at := func(db float64) int {
			for i := on; i < len(x); i++ {
				if 10*math.Log10(real(x[i]*cmplx.Conj(x[i]))/p) >= db {
					return i
				}
			}
			return len(x)
		}
		if attack := float64(at(-6)-at(-30)) / float64(cfg.Rate); attack < 1e-4 {
			t.Errorf("%+v: attack time %.3f ms", cfg, attack*1e3)
		}
		if full := float64(at(-0.05)-on) / float64(cfg.Rate); full > 1.25e-3 {
			t.Errorf("%+v: full power %.3f ms after keying", cfg, full*1e3)
		}
	}
}
