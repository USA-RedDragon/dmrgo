package rf

import (
	"fmt"
	"math"
	"math/bits"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

type msTalker struct {
	slot    int
	lead    int
	delay   float64
	levelDB float64
	hz      float64
}

func talkerFM(t testing.TB, k msTalker, src uint32) (freq, env []float64, bursts []layer2.RawBurst) {
	t.Helper()
	cfg := rftx.SeqConfig{ColorCode: 7, Source: src, Dest: 9, Group: true, Wakeups: 1, Superframes: 3, Message: "DMRGO MS BOTH"}
	b, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	f, e := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: k.slot})
	lead := float64(2*k.lead)*slotSeconds*basebandRate + k.delay*samplesPerSy
	n := len(f) + int(math.Ceil(lead)) + 1
	freq, env = make([]float64, n), make([]float64, n)
	for i := range n {
		u := float64(i) - lead
		j := int(math.Floor(u))
		if j < 0 || j+1 >= len(f) {
			continue
		}
		w := u - float64(j)
		freq[i] = (1-w)*f[j] + w*f[j+1]
		env[i] = (1-w)*e[j] + w*e[j+1]
	}
	return freq, env, b.Bursts
}

func msMix(t testing.TB, cn float64, seed uint64, talkers ...msTalker) ([]complex64, []layer2.RawBurst, []int) {
	t.Helper()
	pad := int(benchPad * basebandRate)
	fs, es := make([][]float64, 0, len(talkers)), make([][]float64, 0, len(talkers))
	var truth []layer2.RawBurst
	var who []int
	n := 0
	for i, k := range talkers {
		f, e, bs := talkerFM(t, k, 3191868+uint32(i))
		fs, es = append(fs, f), append(es, e)
		n = max(n, len(f))
		for m, b := range bs {
			j := 2*(k.lead+m) + k.slot
			for len(truth) <= j {
				truth, who = append(truth, layer2.RawBurst{}), append(who, -1)
			}
			truth[j], who[j] = b, i
		}
	}
	x := make([]complex64, n+2*pad)
	rng := rand.New(rand.NewPCG(seed, seed+1))
	sigma := math.Sqrt(basebandRate/(math.Pow(10, cn/10)*12500)) / math.Sqrt2
	ph := make([]float64, len(talkers))
	for i := range x {
		re, im := rng.NormFloat64()*sigma, rng.NormFloat64()*sigma
		if k := i - pad; k >= 0 {
			for m, tk := range talkers {
				if k >= len(fs[m]) {
					continue
				}
				ph[m] += 2 * math.Pi * (fs[m][k] + 1400 + tk.hz) / basebandRate
				a := es[m][k] * math.Pow(10, tk.levelDB/20)
				re += a * math.Cos(ph[m])
				im += a * math.Sin(ph[m])
			}
		}
		x[i] = complex(float32(re), float32(im))
	}
	return x, truth, who
}

type msScore struct {
	exact, truth []int
	false        int
	parity       bool
}

func scoreTalkers(truth []layer2.RawBurst, who []int, talkers int, got []Burst) msScore {
	s := msScore{exact: make([]int, talkers), truth: make([]int, talkers), parity: true}
	for _, w := range who {
		if w >= 0 {
			s.truth[w]++
		}
	}
	first := (slotSeconds-burstSeconds)/2 + benchPad
	seen := map[int]bool{}
	par := map[int]int64{}
	for i := range got {
		b := &got[i]
		j := int(math.Round((b.Seconds - first) / slotSeconds))
		if j < 0 || j >= len(truth) || who[j] < 0 || math.Abs(b.Seconds-first-float64(j)*slotSeconds) > 0.002 {
			s.false++
			continue
		}
		p := (b.Slot - int64(j)) & 1
		if q, ok := par[b.Stream]; ok && q != p {
			s.parity = false
		}
		par[b.Stream] = p
		d, _ := b.Decoded()
		if !seen[j] && d == truth[j] {
			s.exact[who[j]]++
		}
		seen[j] = true
	}
	return s
}

func msBothEntries() []benchEntry {
	return []benchEntry{
		{benchAudio, Tier0}, {benchAudio, Tier1}, {benchAudio, Tier2},
		{benchIQLow, Tier0}, {benchIQ, Tier0}, {benchIQ, Tier1}, {benchIQ, Tier2},
	}
}

func TestMSBothSlots(t *testing.T) {
	t.Parallel()
	type tc struct {
		name    string
		talkers []msTalker
	}
	cases := []tc{
		{"even-0dB", []msTalker{{slot: 0}, {slot: 1, lead: 3, delay: 0.37}}},
		{"odd-first", []msTalker{{slot: 0, lead: 4, delay: 0.21}, {slot: 1, delay: 1.62}}},
		{"half-10dB", []msTalker{{slot: 0, levelDB: 10}, {slot: 1, lead: 2, delay: 0.5, hz: 150}}},
		{"strong-20dB", []msTalker{{slot: 0, levelDB: 20}, {slot: 1, lead: 2, delay: 2.73, hz: -200}}},
		{"weak-first-20dB", []msTalker{{slot: 0, lead: 3, delay: 0.88, levelDB: 20, hz: 250}, {slot: 1, delay: 0.13}}},
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, runtime.GOMAXPROCS(0))
	for _, c := range cases {
		x, truth, who := msMix(t, 22, 7, c.talkers...)
		for _, e := range msBothEntries() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				s := scoreTalkers(truth, who, len(c.talkers), benchReceive(t, x, e))
				mu.Lock()
				defer mu.Unlock()
				msg := fmt.Sprintf("%s %s: exact %v of %v, %d off the truth slots, parity ok %v", c.name, e.name(), s.exact, s.truth, s.false, s.parity)
				ok := s.false == 0 && s.parity
				for k := range s.exact {
					ok = ok && 10*s.exact[k] >= 9*s.truth[k]
				}
				if !ok {
					t.Error(msg)
				} else if testing.Verbose() {
					t.Log(msg)
				}
			}()
		}
	}
	wg.Wait()
}

func TestMSOneSlot(t *testing.T) {
	t.Parallel()
	for _, slot := range []int{0, 1} {
		x, truth, who := msMix(t, 22, 11, msTalker{slot: slot, lead: 1, delay: 0.41})
		for _, e := range msBothEntries() {
			got := benchReceive(t, x, e)
			s := scoreTalkers(truth, who, 1, got)
			if s.false != 0 || !s.parity || 10*s.exact[0] < 9*s.truth[0] {
				t.Errorf("slot %d %s: exact %d of %d, %d off the truth slots, parity ok %v", slot, e.name(), s.exact[0], s.truth[0], s.false, s.parity)
			}
		}
	}
}

func TestMSBothLookback(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.Wakeups = 40
	b, err := rftx.Sequence("wakeup", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: -1})
	iq := rftx.ToIQ(fm, env, 2e6, 0, 0.9)
	x := rftx.FromCU8(rftx.Channel(iq, 2e6, 2.048e6, -2500, 20, 7))
	cu8 := make([]byte, 2*len(x))
	for i, v := range x {
		cu8[2*i] = byte(math.Max(0, math.Min(255, math.Round(real(v)*127.5+127.5))))
		cu8[2*i+1] = byte(math.Max(0, math.Min(255, math.Round(imag(v)*127.5+127.5))))
	}
	for _, tier := range []Tier{Tier1, Tier2} {
		r := mustNew(RxConfig{Rate: 2048000, Tier: tier, Format: IQFormatCU8})
		var got []Burst
		for k := 0; k < len(cu8); k += 1 << 16 {
			got = r.AppendBursts(got, cu8[k:min(k+1<<16, len(cu8))])
		}
		got = r.AppendFlush(got)
		errs := 0
		for i := range got {
			for k := range got[i].Data {
				errs += bits.OnesCount8(got[i].Data[k] ^ b.Bursts[0][k])
			}
		}
		if len(got) != len(b.Bursts) || errs != 0 {
			t.Errorf("tier %d: %d of %d bursts, %d bit errors", tier-Tier0, len(got), len(b.Bursts), errs)
		}
	}
}
