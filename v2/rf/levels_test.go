package rf

import (
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

type levelsReceiver struct {
	hz   int
	tier Tier
}

func (lr levelsReceiver) name() string {
	if lr.hz == 0 {
		return fmt.Sprintf("iq%d", lr.tier-Tier0)
	}
	return fmt.Sprintf("audio%d-%d", lr.tier-Tier0, lr.hz)
}

func (lr levelsReceiver) receive(t testing.TB, x []complex64) []Burst {
	t.Helper()
	if lr.hz == 0 {
		return receiveBaseband(x, lr.tier)
	}
	var out []Burst
	r, err := NewDisc(DiscConfig{Hz: lr.hz, Tier: lr.tier, OnBurst: func(b *Burst) { out = append(out, *b) }})
	if err != nil {
		t.Fatal(err)
	}
	a := discAudio(x, lr.hz)
	for k := 0; k < len(a); k += 1024 {
		r.Write(a[k:min(k+1024, len(a))])
	}
	r.Flush()
	return out
}

func levelsReceivers() []levelsReceiver {
	return []levelsReceiver{
		{9600, Tier0}, {12000, Tier0}, {24000, Tier0}, {9600, Tier1},
		{0, Tier0}, {0, Tier1},
	}
}

func signalBaseband(parts []signalPart, end, cn float64, seed uint64) []complex64 {
	x := make([]complex64, int(end*basebandRate))
	rng := rand.New(rand.NewPCG(seed, seed+1))
	var sigma float64
	if !math.IsInf(cn, 1) {
		sigma = math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	}
	f := make([]float64, len(x))
	a := make([]float64, len(x))
	for _, p := range parts {
		s := int(p.start * basebandRate)
		for k, e := range p.env {
			if i := s + k; i < len(x) {
				a[i] = e
				if p.fm != nil {
					f[i] = p.fm[k]
				}
			}
		}
	}
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (f[i] + 300) / basebandRate
		x[i] = complex(float32(a[i]*math.Cos(ph)+rng.NormFloat64()*sigma), float32(a[i]*math.Sin(ph)+rng.NormFloat64()*sigma))
	}
	return x
}

type signalPart struct {
	start   float64
	fm, env []float64
}

func exactFrom(truth []benchSlot, got []Burst, start float64, from int) (exact, total int) {
	var g []Burst
	shift := start - benchPad + float64(from)*slotSeconds
	for _, b := range got {
		if b.Seconds-shift > benchPad-slotSeconds/2 {
			b.Seconds -= shift
			g = append(g, b)
		}
	}
	sc := scoreBench(truth[from:], g)
	for _, k := range []string{"ambe", "rate12", "rate34", "rate1", "csbk", "header", "lc"} {
		exact += sc[k].Exact
		total += sc[k].Truth
	}
	return exact, total
}

func TestDiscLevelsAfterUnlock(t *testing.T) {
	t.Parallel()
	s1, _ := benchStream(t, 2)
	s2, truth := benchStream(t, 4)
	f1, e1 := rftx.ModulateBS(s1)
	f2, e2 := rftx.ModulateBS(s2)
	for _, lr := range levelsReceivers() {
		t.Run(lr.name(), func(t *testing.T) {
			t.Parallel()
			for k := range 40 {
				pad := 0.3 + float64(k)*0.0007
				start := pad + float64(len(f1))/basebandRate + 0.045
				x := signalBaseband([]signalPart{{pad, f1, e1}, {start, f2, e2}}, start+float64(len(f2))/basebandRate+0.3, 30, 1)
				if ex, tot := exactFrom(truth, lr.receive(t, x), start, lostSlots+3); ex < tot-2 {
					t.Errorf("first stream from %.4f s: %d of %d payloads of the second exact", pad, ex, tot)
				}
			}
		})
	}
}

func TestCarrierLeadIn(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	fm, env := rftx.ModulateBS(slots)
	for i := 0; env[i] < 1; i++ {
		env[i] = 1
	}
	rxs := append(levelsReceivers(), levelsReceiver{0, Tier2})
	for _, cn := range []float64{math.Inf(1), 40} {
		for _, lr := range rxs {
			t.Run(fmt.Sprintf("%s/%.0f", lr.name(), cn), func(t *testing.T) {
				t.Parallel()
				for _, base := range []float64{0.1, 0.5, 2} {
					for k := range 30 {
						lead := base + float64(k)*0.001
						start := benchPad + lead
						carrier := make([]float64, int(lead*basebandRate))
						for i := range carrier {
							carrier[i] = 1
						}
						x := signalBaseband([]signalPart{{benchPad, nil, carrier}, {start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, 1)
						if ex, tot := exactFrom(truth, lr.receive(t, x), start, 1); ex < tot-2 {
							t.Errorf("%.3f s of carrier: %d of %d payloads exact", lead, ex, tot)
						}
					}
				}
			})
		}
	}
}

func TestCoherentStart(t *testing.T) {
	t.Parallel()
	slots, truth := benchStream(t, 2)
	fm, env := rftx.ModulateBS(slots)
	for _, cn := range []float64{20, 40} {
		t.Run(fmt.Sprint(cn), func(t *testing.T) {
			t.Parallel()
			for k := range 60 {
				start := benchPad + float64(k)*0.0005
				x := signalBaseband([]signalPart{{start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, uint64(k)+1)
				if ex, tot := exactFrom(truth, levelsReceiver{0, Tier2}.receive(t, x), start, 1); ex < tot-2 {
					t.Errorf("start at %.4f s: %d of %d payloads exact", start, ex, tot)
				}
			}
		})
	}
}

func TestCarrierLeadInMS(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	for i := 0; env[i] < 1; i++ {
		env[i] = 1
	}
	want := make([]uint64, 0, 18*cfg.Superframes)
	for k := range 18 * cfg.Superframes {
		want = append(want, rftx.AMBEValue(rftx.TestAMBEFrame(k)))
	}
	for _, cn := range []float64{math.Inf(1), 40} {
		for _, lr := range []levelsReceiver{{24000, Tier0}, {0, Tier2}} {
			t.Run(fmt.Sprintf("%s/%.0f", lr.name(), cn), func(t *testing.T) {
				t.Parallel()
				for _, base := range []float64{0.01, 0.1, 0.5, 2} {
					for k := range 30 {
						lead := base + float64(k)*0.001
						start := benchPad + lead
						carrier := make([]float64, int(lead*basebandRate))
						for i := range carrier {
							carrier[i] = 1
						}
						x := signalBaseband([]signalPart{{benchPad, nil, carrier}, {start, fm, env}}, start+float64(len(fm))/basebandRate+0.3, cn, uint64(k)+1)
						if n := c0c1InOrder(want, voiceFrames(lr.receive(t, x))); n < len(want) {
							t.Errorf("%.3f s of carrier: C0 and C1 of %d of %d voice frames exact", lead, n, len(want))
						}
					}
				}
			})
		}
	}
}
