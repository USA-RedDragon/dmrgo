//go:build !tinygo

package rf

import (
	"encoding/json"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

const rxsheetPath = "testdata/rxsheet.json"

type sheetEye struct {
	Mode   string    `json:"mode"`
	Tier   int       `json:"tier"`
	CN     float64   `json:"cn"`
	Traces [][]int16 `json:"traces"`
}

type sheetPoint struct {
	X        float64                `json:"x"`
	Payloads map[string]*benchScore `json:"payloads"`
}

type sheetSweep struct {
	Mode   string       `json:"mode"`
	Tier   int          `json:"tier"`
	Points []sheetPoint `json:"points"`
}

type sheetAcq struct {
	CN    float64   `json:"cn"`
	Runs  int       `json:"runs"`
	Times []float64 `json:"times_ms"`
}

type sheetAcqTier struct {
	Mode   string     `json:"mode"`
	Tier   int        `json:"tier"`
	Points []sheetAcq `json:"points"`
}

type sheetTiming struct {
	Mode string  `json:"mode"`
	Tier int     `json:"tier"`
	Bias float64 `json:"bias_symbols"`
}

type sheetData struct {
	EyeSteps   int            `json:"eye_steps"`
	BiasStep   float64        `json:"bias_step_symbols"`
	OffsetCN   float64        `json:"offset_cn"`
	RateCN     float64        `json:"rate_cn"`
	SweepSeeds int            `json:"sweep_seeds"`
	AcqWindow  float64        `json:"acq_window_ms"`
	Timing     []sheetTiming  `json:"timing"`
	Eyes       []sheetEye     `json:"eyes"`
	Offset     []sheetSweep   `json:"offset"`
	RateError  []sheetSweep   `json:"rate_error"`
	Acquire    []sheetAcqTier `json:"acquire"`
	RatesIQ    []sheetAccept  `json:"rates_iq"`
	RatesAudio []sheetAccept  `json:"rates_audio"`
}

type sheetAccept struct {
	Hz    int    `json:"hz"`
	Tiers []bool `json:"tiers"`
}

const (
	sheetEyeSteps  = 10
	sheetEyeTraces = 128
	sheetOffsetCN  = 15
	sheetRateCN    = 15
	sheetAcqMS     = 1500
	sheetAcqRuns   = 12
	sheetSeeds     = 8
)

func sheetEyeCNs() []float64 { return []float64{30, 12, 6} }

func sheetOffsets() []float64 {
	var out []float64
	for hz := -7000.0; hz <= 7000; hz += 500 {
		out = append(out, hz)
	}
	return out
}

func sheetPPMs() []float64 {
	return []float64{-5000, -4000, -3000, -2000, -1500, -1000, -700, -500, -300, -200, -100, 0, 100, 200, 300, 500, 700, 1000, 1500, 2000, 3000, 4000, 5000}
}

func sheetAcqCNs() []float64 {
	var out []float64
	for cn := 4.0; cn <= 20; cn += 2 {
		out = append(out, cn)
	}
	return out
}

func sinc(x float64) float64 {
	if x == 0 {
		return 1
	}
	return math.Sin(math.Pi*x) / (math.Pi * x)
}

func kaiserAt(t, beta float64) float64 {
	i0 := func(v float64) float64 {
		s, u := 1.0, 1.0
		for k := 1; k < 40; k++ {
			u *= v / (2 * float64(k))
			s += u * u
		}
		return s
	}
	if math.Abs(t) >= 1 {
		return 0
	}
	return i0(beta*math.Sqrt(1-t*t)) / i0(beta)
}

func interpComplex(x []complex64, pos float64) complex128 {
	const half, beta, fc = 16, 8.0, 0.9
	k0 := int(math.Floor(pos))
	var re, im float64
	for k := k0 - half + 1; k <= k0+half; k++ {
		if k < 0 || k >= len(x) {
			continue
		}
		d := pos - float64(k)
		w := fc * sinc(fc*d) * kaiserAt(d/half, beta)
		re += float64(real(x[k])) * w
		im += float64(imag(x[k])) * w
	}
	return complex(re, im)
}

func resampleClock(x []complex64, ppm float64) []complex64 {
	if ppm == 0 {
		return x
	}
	r := 1 / (1 + ppm*1e-6)
	n := int(float64(len(x)) / r)
	y := make([]complex64, n)
	for i := range y {
		y[i] = complex64(interpComplex(x, float64(i)*r))
	}
	return y
}

func upsample(x []complex64, k int) []complex64 {
	y := make([]complex64, len(x)*k)
	for i := range y {
		y[i] = complex64(interpComplex(x, float64(i)/float64(k)))
	}
	return y
}

func centered(x []float64, h []float64) []float64 {
	half := len(h) / 2
	y := make([]float64, len(x))
	for i := range y {
		var s float64
		for k, w := range h {
			if j := i + k - half; j >= 0 && j < len(x) {
				s += x[j] * w
			}
		}
		y[i] = s
	}
	return y
}

func sheetDiscHz(x []complex64, rate float64) []float64 {
	h := audioIFTaps()
	re, im := make([]float64, len(x)), make([]float64, len(x))
	for i, v := range x {
		re[i], im[i] = float64(real(v)), float64(imag(v))
	}
	re, im = centered(re, h), centered(im, h)
	out := make([]float64, len(x))
	for i := 1; i < len(x); i++ {
		a := complex(re[i], im[i]) * complex(re[i-1], -im[i-1])
		out[i] = math.Atan2(imag(a), real(a)) * rate / (2 * math.Pi)
	}
	return out
}

func sheetEyeSignal(e benchEntry, x []complex64) (f []float64, rate, lag float64) {
	const up = basebandRate / iq0LowHz
	switch e.mode {
	case benchAudio:
		a := discAudio(x, benchAudioHz)
		c := make([]complex64, len(a))
		for i, v := range a {
			c[i] = complex((float32(v)-audioOffset)/audioGain, 0)
		}
		u := upsample(c, up)
		f = make([]float64, len(u))
		for i, v := range u {
			f[i] = float64(real(v))
		}
	case benchIQLow:
		f, lag = sheetDiscHz(upsample(lowIQ(x, iq0LowHz), up), basebandRate), 0.5
	default:
		f, lag = sheetDiscHz(x, basebandRate), 0.5
	}
	return centered(f, rrcTapsAt(basebandRate/symbolRate)), basebandRate, lag
}

func sheetReceive(t testing.TB, x []complex64, e benchEntry) []Burst {
	return benchReceive(t, x, e)
}

type sheetFold struct {
	f         []float64
	rate, lag float64
	got       []Burst
}

func sheetFoldOf(t testing.TB, sig benchSignal, e benchEntry, cn float64) sheetFold {
	x := sig.noisy(cn, 21)
	f, rate, lag := sheetEyeSignal(e, x)
	return sheetFold{f: f, rate: rate, lag: lag, got: sheetReceive(t, x, e)}
}

func (s sheetFold) at(sec float64) float64 {
	p := sec*s.rate + s.lag
	i := int(math.Floor(p))
	if i < 0 || i+1 >= len(s.f) {
		return math.NaN()
	}
	return s.f[i] + (s.f[i+1]-s.f[i])*(p-float64(i))
}

const (
	sheetBurstSymbols = layer2.BurstBits / 2
	sheetBiasSteps    = 50
)

func sheetBias(t testing.TB, sig benchSignal, e benchEntry) float64 {
	s := sheetFoldOf(t, sig, e, 40)
	offs := make([]float64, 0, len(s.got))
	for _, b := range s.got {
		best, bd := 0.0, math.Inf(1)
		for d := -sheetBiasSteps; d < sheetBiasSteps; d++ {
			var sum float64
			for k := 2; k < sheetBurstSymbols-2; k++ {
				a := math.Abs(s.at(b.Seconds+(float64(k)+float64(d)/(2*sheetBiasSteps))/symbolRate) / deviationHz)
				sum += min(math.Abs(a-1), math.Abs(a-3))
			}
			if sum < bd {
				best, bd = float64(d)/(2*sheetBiasSteps), sum
			}
		}
		offs = append(offs, best)
	}
	if len(offs) == 0 {
		return 0
	}
	sort.Float64s(offs)
	return offs[len(offs)/2]
}

func sheetEyeOf(t testing.TB, sig benchSignal, e benchEntry, cn, bias float64) sheetEye {
	s := sheetFoldOf(t, sig, e, cn)
	var centers []float64
	for _, b := range s.got {
		for k := 2; k < sheetBurstSymbols-2; k++ {
			centers = append(centers, b.Seconds+(float64(k)+bias)/symbolRate)
		}
	}
	out := sheetEye{Mode: e.mode, Tier: e.number(), CN: cn}
	if len(centers) == 0 {
		return out
	}
	rng := rand.New(rand.NewPCG(7, 8))
	for range 4 * sheetEyeTraces {
		if len(out.Traces) == sheetEyeTraces {
			break
		}
		i := rng.IntN(len(centers))
		tr := make([]int16, 0, 2*sheetEyeSteps+1)
		ok := true
		for k := -sheetEyeSteps; k <= sheetEyeSteps; k++ {
			v := s.at(centers[i] + float64(k)/(sheetEyeSteps*symbolRate))
			if math.IsNaN(v) {
				ok = false
				break
			}
			tr = append(tr, int16(max(-32000, min(32000, math.Round(v/deviationHz*100)))))
		}
		if ok {
			out.Traces = append(out.Traces, tr)
		}
	}
	return out
}

func sheetScore(truth []benchSlot, got []Burst) map[string]*benchScore {
	all := scoreBench(truth, got)
	out := map[string]*benchScore{}
	for _, k := range []string{"ambe", "csbk"} {
		if v := all[k]; v != nil {
			out[k] = v
		}
	}
	return out
}

func sheetSweepOf(t testing.TB, truth []benchSlot, e benchEntry, xs []float64, mk func(float64) []complex64, clock func(float64) float64) sheetSweep {
	out := sheetSweep{Mode: e.mode, Tier: e.number()}
	for _, v := range xs {
		got := sheetReceive(t, mk(v), e)
		for i := range got {
			got[i].Seconds /= clock(v)
		}
		out.Points = append(out.Points, sheetPoint{X: v, Payloads: sheetScore(truth, got)})
	}
	return out
}

func sheetSum(sw []sheetSweep) sheetSweep {
	out := sw[0]
	out.Points = make([]sheetPoint, len(sw[0].Points))
	for j, p := range sw[0].Points {
		sum := map[string]*benchScore{}
		for _, s := range sw {
			for k, v := range s.Points[j].Payloads {
				if sum[k] == nil {
					sum[k] = &benchScore{}
				}
				sum[k].Exact += v.Exact
				sum[k].Truth += v.Truth
			}
		}
		out.Points[j] = sheetPoint{X: p.X, Payloads: sum}
	}
	return out
}

func noClock(float64) float64 { return 1 }

func ppmClock(ppm float64) float64 { return 1 + ppm*1e-6 }

func sheetAcqOf(t testing.TB, sig benchSignal, truth []benchSlot, e benchEntry) sheetAcqTier {
	out := sheetAcqTier{Mode: e.mode, Tier: e.number()}
	const pad = 0.3
	ramp := (slotSeconds - burstSeconds) / 2
	n := len(sig.re)
	win := int(sheetAcqMS * 1e-3 * basebandRate)
	for _, cn := range sheetAcqCNs() {
		p := sheetAcq{CN: cn, Runs: sheetAcqRuns}
		for run := range sheetAcqRuns {
			rng := rand.New(rand.NewPCG(uint64(run)+500, uint64(cn*10)))
			lo := int((pad + 0.2) * basebandRate)
			s0 := lo + rng.IntN(n-win-lo-int(pad*basebandRate))
			x := sig.noisy(cn, uint64(run)+900)[s0 : s0+win]
			for _, b := range sheetReceive(t, x, e) {
				sec := b.Seconds + float64(s0)/basebandRate
				i := int(math.Round((sec - pad - ramp - float64(cachSymbols)/symbolRate) / slotSeconds))
				if i < 0 || i >= len(truth) {
					continue
				}
				if d, _ := b.Decoded(); d == truth[i].burst {
					p.Times = append(p.Times, math.Round((b.Seconds+burstSeconds)*1e4)/10)
					break
				}
			}
		}
		sort.Float64s(p.Times)
		out.Points = append(out.Points, p)
	}
	return out
}

func sheetRates(audio bool) []sheetAccept {
	var hzs []int
	if audio {
		hzs = []int{7999, 8000, 9600, 11025, 12000, 22050, 24000, 44100, 48000, 48001, 96000, 1000000, 1000001}
	} else {
		hzs = []int{9599, 9600, 9601, 11999, 12000, 12001, 23999, 24000, 47999, 48000, 96000, 250000, 2400000, 20000000, 1000000000, 1000000001}
	}
	out := make([]sheetAccept, 0, len(hzs))
	for _, hz := range hzs {
		a := sheetAccept{Hz: hz}
		for tier := Tier0; tier <= Tier3; tier++ {
			var err error
			if audio {
				_, err = NewAudioReceiver(&AudioRxConfig{Rate: hz, Tier: tier})
			} else {
				_, err = NewReceiver(&RxConfig{Rate: hz, Tier: tier})
			}
			a.Tiers = append(a.Tiers, err == nil)
		}
		out = append(out, a)
	}
	return out
}

func TestRxSheet(t *testing.T) {
	t.Parallel()
	if os.Getenv("DMRRF_RXSHEET") != "1" {
		t.Skip("set DMRRF_RXSHEET=1 to measure testdata/rxsheet.json")
	}
	slots, truth := benchStream(t, 6)
	sig := cleanBaseband(slots, 0)
	entries := benchEntries()
	d := sheetData{EyeSteps: sheetEyeSteps, BiasStep: 1.0 / (2 * sheetBiasSteps), OffsetCN: sheetOffsetCN, RateCN: sheetRateCN, SweepSeeds: sheetSeeds, AcqWindow: sheetAcqMS,
		RatesIQ: sheetRates(false), RatesAudio: sheetRates(true)}
	eyes := make([][]sheetEye, len(entries))
	bias := make([]float64, len(entries))
	offs := make([][]sheetSweep, len(entries))
	rates := make([][]sheetSweep, len(entries))
	acqs := make([]sheetAcqTier, len(entries))
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	var wg sync.WaitGroup
	job := func(f func()) {
		wg.Add(1)
		go func() {
			sem <- struct{}{}
			defer func() { <-sem; wg.Done() }()
			f()
		}()
	}
	for i, e := range entries {
		job(func() {
			bias[i] = sheetBias(t, sig, e)
			for _, cn := range sheetEyeCNs() {
				eyes[i] = append(eyes[i], sheetEyeOf(t, sig, e, cn, bias[i]))
			}
		})
		offs[i], rates[i] = make([]sheetSweep, sheetSeeds), make([]sheetSweep, sheetSeeds)
		for k := range uint64(sheetSeeds) {
			job(func() {
				offs[i][k] = sheetSweepOf(t, truth, e, sheetOffsets(), func(hz float64) []complex64 {
					return cleanBaseband(slots, hz).noisy(sheetOffsetCN, 31+k)
				}, noClock)
			})
			job(func() {
				x := sig.noisy(sheetRateCN, 41+k)
				rates[i][k] = sheetSweepOf(t, truth, e, sheetPPMs(), func(ppm float64) []complex64 { return resampleClock(x, ppm) }, ppmClock)
			})
		}
		job(func() { acqs[i] = sheetAcqOf(t, sig, truth, e) })
	}
	wg.Wait()
	for i, e := range entries {
		d.Timing = append(d.Timing, sheetTiming{Mode: e.mode, Tier: e.number(), Bias: bias[i]})
		d.Eyes = append(d.Eyes, eyes[i]...)
	}
	for i := range entries {
		d.Offset, d.RateError = append(d.Offset, sheetSum(offs[i])), append(d.RateError, sheetSum(rates[i]))
	}
	d.Acquire = acqs
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rxsheetPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
