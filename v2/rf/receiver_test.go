package rf

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func noisyBaseband(slots []rftx.BSSlot, offsetHz, cn float64, seed uint64) []complex64 {
	fm, env := rftx.ModulateBS(slots)
	pad := int(0.3 * basebandRate)
	x := make([]complex64, len(fm)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - pad; k >= 0 && k < len(fm) {
			ph += 2 * math.Pi * (fm[k] + offsetHz) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	return x
}

func basebandStream(slots []rftx.BSSlot, offsetHz float64) []complex64 {
	return noisyBaseband(slots, offsetHz, 40, 1)
}

func receiveBaseband(x []complex64, t Tier) []Burst {
	r := mustNew(Config{Rate: basebandRate, Tier: t})
	var out []Burst
	for k := 0; k < len(x); k += 4096 {
		out = append(out, must(r.WriteBaseband(x[k:min(k+4096, len(x))]))...)
	}
	return append(out, r.Flush()...)
}

func exactFrames(slots []rftx.BSSlot, got []Burst) (exact, total int) {
	const pad = 0.3
	ramp := (slotSeconds - burstSeconds) / 2
	byIndex := map[int]Burst{}
	for _, r := range got {
		i := int(math.Round((r.Seconds - pad - ramp - float64(cachSymbols)/symbolRate) / slotSeconds))
		byIndex[i] = r
	}
	for i, s := range slots {
		want, res := s.Burst.AMBEFrames()
		if i%2 != 1 || res.Uncorrectable {
			continue
		}
		r, ok := byIndex[i]
		var f [3]AMBEFrameValue
		if ok {
			data, _ := r.Decoded()
			frames, _ := data.AMBEFrames()
			for k := range frames {
				f[k] = rftx.AMBEValue(frames[k])
			}
		}
		for k := range want {
			total++
			if ok && f[k] == rftx.AMBEValue(want[k]) {
				exact++
			}
		}
	}
	return exact, total
}

type AMBEFrameValue = uint64

func TestReceiverTiersAtLowSNR(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 6, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 3, 3)
	prev := -1
	for _, tier := range []Tier{Tier0, Tier2, Tier3} {
		exact, total := exactFrames(built.Slots, receiveBaseband(x, tier))
		t.Logf("tier %d: %d of %d frames exact", tier, exact, total)
		if exact <= prev {
			t.Errorf("tier %d: %d frames exact, the tier below %d", tier, exact, prev)
		}
		prev = exact
	}
}

func TestReceiverHalfRate(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 4, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 12, 6)
	full, total := exactFrames(built.Slots, receiveBaseband(x, Tier0))
	h := make([]complex64, len(x)/2)
	for i := range h {
		h[i] = (x[2*i] + x[2*i+1]) / 2
	}
	r := mustNew(Config{Tier: Tier0, BasebandHz: BasebandRate / 2})
	var got []Burst
	for k := 0; k < len(h); k += 2048 {
		got = append(got, must(r.WriteBaseband(h[k:min(k+2048, len(h))]))...)
	}
	got = append(got, r.Flush()...)
	half, _ := exactFrames(built.Slots, got)
	t.Logf("%d of %d frames exact at %d Hz, %d at %d Hz", full, total, BasebandRate, half, BasebandRate/2)
	if full < total*9/10 || half < full-total/20 {
		t.Errorf("%d of %d frames exact at %d Hz, %d at %d Hz", full, total, BasebandRate, half, BasebandRate/2)
	}
	if _, err := New(Config{Tier: Tier2, BasebandHz: BasebandRate / 2}); err == nil {
		t.Error("Tier2 takes input at half rate")
	}
}

func TestReceiverOnBurst(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 10, 4)
	for _, tier := range []Tier{Tier0, Tier2} {
		want := receiveBaseband(x, tier)
		var got []Burst
		r := mustNew(Config{Rate: basebandRate, Tier: tier, OnBurst: func(b *Burst) { got = append(got, *b) }})
		for k := 0; k < len(x); k += 4096 {
			if out := must(r.WriteBaseband(x[k:min(k+4096, len(x))])); len(out) != 0 {
				t.Fatalf("tier %d: WriteBaseband returned %d bursts", tier, len(out))
			}
		}
		if out := r.Flush(); len(out) != 0 {
			t.Fatalf("tier %d: Flush returned %d bursts", tier, len(out))
		}
		if len(got) != len(want) || len(want) == 0 {
			t.Fatalf("tier %d: %d bursts to OnBurst, %d returned", tier, len(got), len(want))
		}
		for i := range want {
			if got[i].Data != want[i].Data || got[i].Fixed != want[i].Fixed || got[i].Seconds != want[i].Seconds {
				t.Errorf("tier %d: burst %d differs", tier, i)
			}
		}
	}
}

func BenchmarkReceiver(b *testing.B) {
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		b.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 6, 3)
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3, Tier4} {
		b.Run(strconv.Itoa(int(tier)), func(b *testing.B) {
			r := mustNew(Config{Rate: basebandRate, Tier: tier})
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()
			for b.Loop() {
				for k := 0; k < len(x); k += 4096 {
					must(r.WriteBaseband(x[k:min(k+4096, len(x))]))
				}
				r.Flush()
			}
			b.ReportMetric(float64(time.Since(start).Nanoseconds())/float64(b.N)/(float64(len(x))/basebandRate), "ns/signal-s")
		})
	}
}

func TestReceiverWithoutWarmUp(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 20, 5)
	o := tierOptions(Tier3)
	o.WarmUp = false
	r := mustNew(Config{Rate: basebandRate, opt: &o})
	var got []Burst
	for k := 0; k < len(x); k += 4096 {
		got = append(got, must(r.WriteBaseband(x[k:min(k+4096, len(x))]))...)
	}
	got = append(got, r.Flush()...)
	exact, total := exactFrames(built.Slots, got)
	if exact < total*9/10 {
		t.Errorf("%d of %d frames exact", exact, total)
	}
}

func TestReceiverKeyUp(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 2)
	pad := int(0.3 * basebandRate)
	a := noisyBaseband(slots, 1400, 15, 1)
	gap := noisyBaseband(nil, 1400, 15, 2)[:int(0.15*basebandRate)+137]
	b := noisyBaseband(slots, 1100, 15, 3)
	x := append(append(append([]complex64(nil), a...), gap...), b...)
	start := float64(len(a)+len(gap)+pad) / basebandRate
	for _, tier := range []Tier{Tier0, Tier1, Tier2, Tier3} {
		t.Run(strconv.Itoa(int(tier-Tier0)), func(t *testing.T) {
			t.Parallel()
			first := math.Inf(1)
			for _, bu := range receiveBaseband(x, tier) {
				if bu.Seconds >= start-0.03 {
					first = min(first, bu.Seconds)
				}
			}
			if first > start+0.15 {
				t.Errorf("first burst of the second stream at %.3f s, %.3f s after its start", first, first-start)
			}
		})
	}
}

func resampleFM(freq, env []float64, ppm float64) ([]float64, []float64) {
	r := 1 + ppm*1e-6
	n := int(float64(len(freq)-1) / r)
	f, e := make([]float64, n), make([]float64, n)
	for i := range n {
		p := float64(i) * r
		k := int(p)
		u := p - float64(k)
		f[i] = freq[k]*(1-u) + freq[k+1]*u
		e[i] = env[k]*(1-u) + env[k+1]*u
	}
	return f, e
}

func directCalls(t testing.TB, n int) ([]float64, []float64, []benchSlot) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, Group: true, Superframes: 2, Slot: 1}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	f, e := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: 1})
	tail := make([]float64, int(0.3*basebandRate))
	var freq, env []float64
	var truth []benchSlot
	for c := range n {
		first := len(freq) / slotSamplesN
		lead := make([]float64, c*37%100*samplesPerSy/100)
		freq = append(append(append(freq, lead...), f...), tail...)
		env = append(append(append(env, lead...), e...), tail...)
		for len(freq)%slotSamplesN != 0 {
			freq, env = append(freq, 0), append(env, 0)
		}
		pos := enums.VoiceBurstUnknown
		for i, raw := range b.Bursts {
			x, err := layer2.NewBurstFromBytes(raw)
			switch {
			case err == nil && x.Kind == layer2.KindVoice && x.SyncPattern == enums.SyncMSSourcedVoice:
				pos = enums.VoiceBurstA
			case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || x.Kind != layer2.KindData):
				pos++
			default:
				continue
			}
			j := first + 2*i + 1
			for len(truth) <= j {
				truth = append(truth, benchSlot{kind: "idle"})
			}
			truth[j] = benchSlot{burst: raw, kind: "voice", pos: pos}
		}
	}
	return freq, env, truth
}

func TestReceiverDirectModeTiming(t *testing.T) {
	t.Parallel()
	fm, env, truth := directCalls(t, 4)
	for _, c := range []struct{ ppm, share float64 }{{0, 0.995}, {200, 0.94}, {-200, 0.94}} {
		t.Run(strconv.FormatFloat(c.ppm, 'f', -1, 64), func(t *testing.T) {
			t.Parallel()
			f, e := resampleFM(fm, env, c.ppm)
			var exact, total int
			for seed := range uint64(6) {
				sc := scoreMS(truth, receiveBaseband(noisyFM(f, e, 1400, 5, 40+seed), Tier3))
				exact += sc["ambe_c0c1"].Exact
				total += sc["ambe_c0c1"].Truth
			}
			t.Logf("%d of %d frames with C0 and C1 exact", exact, total)
			if float64(exact) < c.share*float64(total) {
				t.Errorf("%d of %d frames with C0 and C1 exact", exact, total)
			}
		})
	}
}

func TestReceiverTwoPass(t *testing.T) {
	t.Parallel()
	fm, env, truth := directCalls(t, 4)
	f, e := resampleFM(fm, env, 200)
	o := tierOptions(Tier3)
	o.TwoPass = true
	var one, two, total int
	for seed := range uint64(6) {
		x := noisyFM(f, e, 1400, 5, 40+seed)
		one += scoreMS(truth, receiveBaseband(x, Tier3))["ambe_c0c1"].Exact
		r := mustNew(Config{Rate: basebandRate, opt: &o})
		var got []Burst
		for k := 0; k < len(x); k += 4096 {
			got = append(got, must(r.WriteBaseband(x[k:min(k+4096, len(x))]))...)
		}
		sc := scoreMS(truth, append(got, r.Flush()...))["ambe_c0c1"]
		two += sc.Exact
		total += sc.Truth
	}
	t.Logf("%d of %d frames with C0 and C1 exact, %d in one pass", two, total, one)
	if two < total || two <= one {
		t.Errorf("%d of %d frames with C0 and C1 exact, %d in one pass", two, total, one)
	}
}
