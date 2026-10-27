package rf

import (
	"math"
	"math/rand/v2"
	"runtime"
	"strconv"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testmode"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func msStream(t testing.TB, n int, jitter float64) ([]float64, []float64, []benchSlot) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: 3191868, Dest: 9, Group: true, Wakeups: 1, Message: "DMRGO TIER ORDER", Superframes: 2, Preambles: 2, Slot: 1}
	const slotN = slotSeconds * basebandRate
	gap := int(0.3 * basebandRate)
	var freq, env []float64
	var truth []benchSlot
	sf, pos := -1, enums.VoiceBurstUnknown
	k := 0
	for range n {
		for _, seq := range []string{"voice", "data12", "data34", "preamble"} {
			c := cfg
			if seq != "voice" {
				c.Group, c.Dest = false, 3140598
			}
			b, err := rftx.Sequence(seq, c)
			if err != nil {
				t.Fatal(err)
			}
			off := int(math.Round(math.Mod(jitter*float64(k), 1) * samplesPerSy))
			k++
			freq = append(freq, make([]float64, off)...)
			env = append(env, make([]float64, off)...)
			first := int(math.Round(float64(len(freq)) / slotN))
			f, e := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: 1})
			freq = append(freq, f...)
			env = append(env, e...)
			for i, raw := range b.Bursts {
				j := first + 2*i + 1
				for len(truth) <= j {
					truth = append(truth, benchSlot{kind: "idle"})
				}
				ts := &truth[j]
				ts.burst = raw
				x, err := layer2.NewBurstFromBytes(raw)
				switch {
				case err == nil && x.Kind == layer2.KindVoice && x.SyncPattern == enums.SyncMSSourcedVoice:
					sf++
					pos = enums.VoiceBurstA
					ts.kind, ts.pos, ts.sf = "voice", pos, sf
				case pos != enums.VoiceBurstUnknown && pos < enums.VoiceBurstF && (err != nil || x.Kind != layer2.KindData):
					pos++
					ts.kind, ts.pos, ts.sf = "voice", pos, sf
				case err == nil && x.Kind == layer2.KindData && x.SlotType.DataType != enums.DataTypeIdle:
					pos = enums.VoiceBurstUnknown
					ts.kind, ts.dt = "data", x.SlotType.DataType
				default:
					pos = enums.VoiceBurstUnknown
				}
			}
			freq = append(freq, make([]float64, gap)...)
			env = append(env, make([]float64, gap)...)
		}
	}
	return freq, env, truth
}

func scoreMS(truth []benchSlot, got []Burst) map[string]*benchScore {
	for i := range got {
		got[i].Seconds += float64(cachSymbols) / symbolRate
	}
	return scoreBench(truth, got)
}

func noisyFM(freq, env []float64, offsetHz, cn float64, seed uint64) []complex64 {
	pad := int(benchPad * basebandRate)
	x := make([]complex64, len(freq)+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	var ph float64
	for i := range x {
		var re, im float64
		if k := i - pad; k >= 0 && k < len(freq) {
			ph += 2 * math.Pi * (freq[k] + offsetHz) / basebandRate
			re, im = env[k]*math.Cos(ph), env[k]*math.Sin(ph)
		}
		x[i] = complex(float32(re+rng.NormFloat64()*sigma), float32(im+rng.NormFloat64()*sigma))
	}
	return x
}

func tierOrderPayloads() []string {
	return []string{"ambe_c0c1", "ambe", "rate12", "rate34", "csbk", "lc", "elc"}
}

type tierOrderCase struct {
	scenario string
	truth    []benchSlot
	x        []complex64
}

func TestTierOrder(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("receives 54 streams with 6 receivers")
	}
	type scenario struct {
		name  string
		cns   []float64
		seeds int
	}
	scenarios := []scenario{
		{"bs", []float64{3, 5, 7}, 8},
		{"ms", []float64{4, 6, 8, 10}, 6},
		{"ms-jitter", []float64{6, 10}, 3},
	}
	slots, bsTruth := benchStream(t, 6)
	fm, env, msTruth := msStream(t, 3, 0)
	jfm, jenv, jTruth := msStream(t, 3, 0.37)
	var cases []tierOrderCase
	for _, sc := range scenarios {
		for _, cn := range sc.cns {
			for seed := range uint64(testmode.N(sc.seeds, sc.seeds, 1)) {
				c := tierOrderCase{scenario: sc.name}
				switch sc.name {
				case "bs":
					c.truth, c.x = bsTruth, noisyBaseband(slots, 1400, cn, 100+seed)
				case "ms":
					c.truth, c.x = msTruth, noisyFM(fm, env, 1400, cn, 100+seed)
				default:
					c.truth, c.x = jTruth, noisyFM(jfm, jenv, 1400, cn, 100+seed)
				}
				cases = append(cases, c)
			}
		}
	}
	entries := []benchEntry{
		{benchAudio, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier2},
		{benchIQ, Tier0}, {benchIQ, Tier1}, {benchIQ, Tier2},
	}
	type key struct {
		scenario string
		e        benchEntry
		payload  string
	}
	sum := map[key]*benchScore{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range cases {
		for _, e := range entries {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				got := benchReceive(t, c.x, e)
				sc := scoreBench(c.truth, got)
				if c.scenario != "bs" {
					sc = scoreMS(c.truth, got)
				}
				mu.Lock()
				defer mu.Unlock()
				for _, p := range tierOrderPayloads() {
					k := key{c.scenario, e, p}
					if sum[k] == nil {
						sum[k] = &benchScore{}
					}
					sum[k].Truth += sc[p].Truth
					sum[k].Exact += sc[p].Exact
				}
			}()
		}
	}
	wg.Wait()
	for _, sc := range scenarios {
		for _, mode := range []string{benchAudio, benchIQ} {
			for _, p := range tierOrderPayloads() {
				for _, tier := range []Tier{Tier1, Tier2} {
					lo := sum[key{sc.name, benchEntry{mode, tier - 1}, p}]
					hi := sum[key{sc.name, benchEntry{mode, tier}, p}]
					if lo.Truth == 0 {
						t.Fatalf("%s: no %s in the stream", sc.name, p)
					}
					if hi.Exact >= lo.Exact-max(2, lo.Truth/100) {
						continue
					}
					t.Errorf("%s %s %s: Tier%d recovers %d of %d, Tier%d %d", sc.name, mode, p, tier-Tier0, hi.Exact, hi.Truth, tier-1-Tier0, lo.Exact)
				}
			}
		}
	}
	if testing.Verbose() {
		for _, sc := range scenarios {
			for _, e := range entries {
				line := sc.name + " " + e.name()
				for _, p := range tierOrderPayloads() {
					s := sum[key{sc.name, e, p}]
					line += " " + p + "=" + strconv.Itoa(s.Exact) + "/" + strconv.Itoa(s.Truth)
				}
				t.Log(line)
			}
		}
	}
}
