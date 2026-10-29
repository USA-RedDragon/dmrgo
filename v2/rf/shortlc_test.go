//go:build !tinygo

package rf

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2"
	"github.com/USA-RedDragon/dmrgo/v2/fec"
	"github.com/USA-RedDragon/dmrgo/v2/internal/fec/bptc"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
	"github.com/USA-RedDragon/dmrgo/v2/layer2/pdu"
)

func slcKey(sec float64) int64 {
	ramp := (slotSeconds - burstSeconds) / 2
	return int64(math.Round((sec - 0.3 - ramp - float64(cachSymbols)/symbolRate) / slotSeconds))
}

func slcWant(slots []rftx.BSSlot) map[int64]pdu.ShortLC {
	m := map[int64]pdu.ShortLC{}
	for g := 0; 4*g+3 < len(slots); g++ {
		var w [4]uint32
		for j := range w {
			w[j] = cachPayloadWord(layer2.DecodeCACHField(slots[4*g+j].CACH).Payload)
		}
		v, _ := bptc.DecodeCACH(w)
		v <<= 4
		m[int64(4*g+3)] = pdu.DecodeShortLC([5]byte{byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
	}
	return m
}

type slcInput struct {
	audio []int16
	iq    []complex64
}

func slcInputOf(x []complex64, e benchEntry) slcInput {
	switch e.mode {
	case benchAudio:
		return slcInput{audio: discAudio(x, benchAudioHz)}
	case benchIQLow:
		return slcInput{iq: lowIQ(x, iq0LowHz)}
	}
	return slcInput{iq: x}
}

func slcReceive(t testing.TB, x []complex64, e benchEntry) ([]Burst, []ShortLC) {
	t.Helper()
	return slcRun(slcInputOf(x, e), e, true)
}

func slcRun(in slcInput, e benchEntry, withLC bool) ([]Burst, []ShortLC) {
	var bursts []Burst
	var lcs []ShortLC
	var on func([]ShortLC)
	if withLC {
		on = func(l []ShortLC) { lcs = append(lcs, l...) }
	}
	switch e.mode {
	case benchAudio:
		a := in.audio
		r := mustAudio(AudioRxConfig{Rate: benchAudioHz, Tier: e.tier, OnBursts: each(func(b *Burst) { bursts = append(bursts, *b) }), OnShortLC: on})
		for k := 0; k < len(a); k += 1024 {
			must(r.writeAll(a[k:min(k+1024, len(a))]))
		}
		r.flushAll()
	case benchIQLow:
		y := in.iq
		r := mustNew(RxConfig{Tier: Tier0, Rate: iq0LowHz, Format: IQFormatCU8, OnShortLC: on})
		for k := 0; k < len(y); k += 1024 {
			bursts = append(bursts, must(r.writeBB(y[k:min(k+1024, len(y))]))...)
		}
		bursts = append(bursts, r.flushAll()...)
	default:
		x := in.iq
		r := mustNew(RxConfig{Rate: basebandRate, Tier: e.tier, Format: IQFormatCU8, OnShortLC: on})
		for k := 0; k < len(x); k += 4096 {
			bursts = append(bursts, must(r.writeBB(x[k:min(k+4096, len(x))]))...)
		}
		bursts = append(bursts, r.flushAll()...)
	}
	return bursts, lcs
}

func slcHardOf(bursts []Burst) []ShortLC {
	var a layer2.ShortLCAssembler
	var out []ShortLC
	stream := 0
	for i := range bursts {
		b := &bursts[i]
		if !b.HasCACH {
			continue
		}
		if b.Stream != stream {
			a.Reset()
			stream = b.Stream
		}
		_, c := b.Decoded()
		if lc, done, err := a.Add(layer2.DecodeCACHField(c)); err == nil && done && !lc.FEC.Uncorrectable {
			out = append(out, ShortLC{LC: lc, Stream: b.Stream, Slot: b.Slot, Seconds: b.Seconds})
		}
	}
	return out
}

func slcSame(a, b pdu.ShortLC) bool {
	return a.SLCO == b.SLCO && a.RawPayload == b.RawPayload
}

func slcCount(lcs []ShortLC, want map[int64]pdu.ShortLC) (good, bad int) {
	for _, l := range lcs {
		if w, ok := want[slcKey(l.Seconds)]; ok && slcSame(w, l.LC) {
			good++
		} else {
			bad++
		}
	}
	return good, bad
}

func TestShortLCClean(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	want := slcWant(slots)
	x := cleanBaseband(slots, 1400).noisy(40, 1)
	for _, e := range benchEntries() {
		t.Run(e.name(), func(t *testing.T) {
			t.Parallel()
			bursts, lcs := slcReceive(t, x, e)
			hard := slcHardOf(bursts)
			good, bad := slcCount(lcs, want)
			if bad != 0 || good < len(want)-1 {
				t.Errorf("Short LCs: %d right, %d wrong, of %d", good, bad, len(want))
			}
			var hl []ShortLC
			for _, l := range lcs {
				if !l.Soft {
					hl = append(hl, l)
				}
			}
			if len(hl) != len(hard) {
				t.Fatalf("got %d hard Short LCs, hard decode gives %d", len(hl), len(hard))
			}
			for i := range hl {
				if !slcSame(hl[i].LC, hard[i].LC) || hl[i].Seconds != hard[i].Seconds || hl[i].Slot != hard[i].Slot || hl[i].Stream != hard[i].Stream {
					t.Errorf("LC %d: got %+v, hard %+v", i, hl[i], hard[i])
				}
			}
			if s := len(lcs) - len(hl); s > 0 {
				t.Logf("%d soft Short LCs where the hard decode fails", s)
			}
		})
	}
}

func TestShortLCSoft(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("short")
	}
	slots, _ := benchStream(t, 30)
	want := slcWant(slots)
	sig := cleanBaseband(slots, 1400)
	for _, e := range []benchEntry{{benchAudio, Tier1}, {benchIQ, Tier1}} {
		t.Run(e.name(), func(t *testing.T) {
			t.Parallel()
			var good, bad, hGood, hBad, soft int
			for _, cn := range []float64{3, 4, 5} {
				for s := range uint64(2) {
					bursts, lcs := slcReceive(t, sig.noisy(cn, 900+s), e)
					g, b := slcCount(lcs, want)
					hg, hb := slcCount(slcHardOf(bursts), want)
					good, bad, hGood, hBad = good+g, bad+b, hGood+hg, hBad+hb
					for _, l := range lcs {
						if l.Soft {
							soft++
						}
					}
				}
			}
			t.Logf("%s: OnShortLC %d right %d wrong (%d soft), hard %d right %d wrong, of %d", e.name(), good, bad, soft, hGood, hBad, 6*len(want))
			if good <= hGood || bad > hBad {
				t.Errorf("OnShortLC %d right %d wrong, hard %d right %d wrong", good, bad, hGood, hBad)
			}
		})
	}
}

func TestShortLCConfig(t *testing.T) {
	t.Parallel()
	on := func([]ShortLC) {}
	var r Receiver
	var ce *dmrgo.ConfigError
	if err := r.Init(RxConfig{Rate: iq0LowHz, OnShortLC: on}); !errors.As(err, &ce) || ce.Field != "RxConfig.OnShortLC" {
		t.Errorf("Receiver.Init: %v", err)
	}
	var a AudioReceiver
	if err := a.Init(AudioRxConfig{OnShortLC: on}); !errors.As(err, &ce) || ce.Field != "AudioRxConfig.OnShortLC" {
		t.Errorf("AudioReceiver.Init: %v", err)
	}
	for tier := Tier0; tier <= Tier3; tier++ {
		r, err := NewReceiver(&RxConfig{Tier: tier, OnShortLC: on})
		if err != nil || r.sink.slc == nil || (r.sink.slc.soft != nil) != (tier != Tier0) {
			t.Errorf("NewReceiver %v: %v", tier, err)
		}
		ar, err := NewAudioReceiver(&AudioRxConfig{Tier: tier, OnShortLC: on})
		if err != nil || ar.bursts.slc == nil || (ar.bursts.slc.soft != nil) != (tier != Tier0) {
			t.Errorf("NewAudioReceiver %v: %v", tier, err)
		}
	}
	if r, _ := NewReceiver(nil); r.sink.slc != nil {
		t.Error("NewReceiver(nil) set up a Short LC output")
	}
}

func TestShortLCBurstsSame(t *testing.T) {
	t.Parallel()
	slots, _ := benchStream(t, 6)
	sig := cleanBaseband(slots, 1400)
	for _, e := range benchEntries() {
		t.Run(e.name(), func(t *testing.T) {
			t.Parallel()
			for _, cn := range []float64{4, 8} {
				x := sig.noisy(cn, 3)
				with, _ := slcReceive(t, x, e)
				without := benchReceive(t, x, e)
				if len(with) != len(without) {
					t.Fatalf("C/N %g: %d bursts with OnShortLC, %d without", cn, len(with), len(without))
				}
				for i := range with {
					if with[i] != without[i] {
						t.Fatalf("C/N %g: burst %d differs with OnShortLC", cn, i)
					}
				}
			}
		})
	}
}

func TestShortLCCost(t *testing.T) {
	t.Parallel()
	v := os.Getenv("DMRRF_SLCCOST")
	if v == "" {
		t.Skip("DMRRF_SLCCOST not set")
	}
	slots, _ := benchStream(t, 30)
	sig := cleanBaseband(slots, 1400)
	for _, e := range benchEntries() {
		if v != "all" && !slices.Contains(strings.Split(v, ","), e.name()) {
			continue
		}
		for _, cn := range []float64{4, 6} {
			x := sig.noisy(cn, 7)
			in := slcInputOf(x, e)
			off, on := math.Inf(1), math.Inf(1)
			for range 7 {
				off = min(off, wallSeconds(func() { slcRun(in, e, false) }))
				on = min(on, wallSeconds(func() { slcRun(in, e, true) }))
			}
			s := float64(len(x)) / basebandRate
			fmt.Printf("SLCCOST\t%s\tC/N %g\twithout %.3f ms/s\twith %.3f ms/s\t%+.1f%%\n", e.name(), cn, off/s*1e3, on/s*1e3, (on/off-1)*100)
		}
	}
}

func wallSeconds(f func()) float64 {
	start := time.Now()
	f()
	return time.Since(start).Seconds()
}

const slcBenchPath = "testdata/rxbench_shortlc.json"

type slcBenchTier struct {
	Mode         string    `json:"mode"`
	Tier         int       `json:"tier"`
	CN           []float64 `json:"cn"`
	Truth        []int     `json:"truth"`
	Hard         []int     `json:"hard"`
	LC           []int     `json:"lc"`
	HardWrong    int       `json:"hard_wrong"`
	LCWrong      int       `json:"lc_wrong"`
	Decoded      int       `json:"decoded"`
	NoiseSeconds float64   `json:"noise_seconds"`
	NoiseHard    int       `json:"noise_hard"`
	NoiseLC      int       `json:"noise_lc"`
}

type slcBenchCapture struct {
	Sigma    float64 `json:"sigma"`
	Tier     int     `json:"tier"`
	Bursts   int     `json:"bursts"`
	Hard     int     `json:"hard"`
	LC       int     `json:"lc"`
	Soft     int     `json:"soft"`
	Distinct int     `json:"distinct"`
}

type slcBench struct {
	Seeds    int               `json:"seeds"`
	Capture  string            `json:"capture"`
	Tiers    []slcBenchTier    `json:"tiers"`
	Captures []slcBenchCapture `json:"captures"`
}

func slcSweep(t *testing.T, e benchEntry, sig benchSignal, noise benchSignal, want map[int64]pdu.ShortLC, seeds int) slcBenchTier {
	bt := slcBenchTier{Mode: e.mode, Tier: e.number()}
	for cn := -3.0; cn <= 16; cn += 0.5 {
		var hg, ng int
		for s := range uint64(seeds) {
			bursts, lcs := slcReceive(t, sig.noisy(cn, 2000+s), e)
			g, b := slcCount(lcs, want)
			h, hb := slcCount(slcHardOf(bursts), want)
			hg, ng, bt.HardWrong, bt.LCWrong, bt.Decoded = hg+h, ng+g, bt.HardWrong+hb, bt.LCWrong+b, bt.Decoded+len(lcs)
		}
		bt.CN, bt.Truth, bt.Hard, bt.LC = append(bt.CN, cn), append(bt.Truth, seeds*len(want)), append(bt.Hard, hg), append(bt.LC, ng)
		if hg == seeds*len(want) {
			break
		}
	}
	for seed := uint64(1); bt.NoiseSeconds < 120; seed++ {
		x := noise.noisy(0, seed)
		bt.NoiseSeconds += float64(len(x)) / basebandRate
		bursts, lcs := slcReceive(t, x, e)
		bt.NoiseHard += len(slcHardOf(bursts))
		bt.NoiseLC += len(lcs)
	}
	return bt
}

func slcCaptures(raw []byte) []slcBenchCapture {
	var out []slcBenchCapture
	for _, sigma := range []float64{0, 16, 20, 24} {
		x := raw
		if sigma > 0 {
			x = make([]byte, len(raw))
			rng := rand.New(rand.NewPCG(77, uint64(sigma*1000)))
			for i, v := range raw {
				x[i] = byte(max(0, min(255, math.Round(float64(v)+rng.NormFloat64()*sigma))))
			}
		}
		for tier := Tier0; tier <= Tier3; tier++ {
			var lcs []ShortLC
			r := mustNew(RxConfig{Rate: 2048000, Tier: tier, Format: IQFormatCU8, OnShortLC: func(l []ShortLC) { lcs = append(lcs, l...) }})
			var bursts []Burst
			for k := 0; k < len(x); k += 1 << 16 {
				bursts = append(bursts, must(r.writeIQ(x[k:min(k+1<<16, len(x))]))...)
			}
			bursts = append(bursts, r.flushAll()...)
			c := slcBenchCapture{Sigma: sigma, Tier: int(tier - Tier0), Bursts: len(bursts), Hard: len(slcHardOf(bursts)), LC: len(lcs)}
			seen := map[pdu.ShortLC]bool{}
			for _, l := range lcs {
				if l.Soft {
					c.Soft++
				}
				l.LC.FEC = fec.Result{}
				seen[l.LC] = true
			}
			c.Distinct = len(seen)
			out = append(out, c)
		}
	}
	return out
}

// TestShortLCBench writes testdata/rxbench_shortlc.json with
// DMRRF_SLCBENCH set to the path of the XPR 8400 idle capture as CU8 at
// 2.048 MHz.
func TestShortLCBench(t *testing.T) {
	t.Parallel()
	capPath := os.Getenv("DMRRF_SLCBENCH")
	if capPath == "" {
		t.Skip("DMRRF_SLCBENCH not set")
	}
	raw, err := os.ReadFile(capPath)
	if err != nil {
		t.Fatal(err)
	}
	slots, _ := benchStream(t, 30)
	want := slcWant(slots)
	sig, noise := cleanBaseband(slots, 1400), cleanBaseband(nil, 0)
	out := slcBench{Seeds: 4, Capture: "motorola-xpr8400-idle-424075000-2048k.cu8"}
	entries := benchEntries()
	out.Tiers = make([]slcBenchTier, len(entries))
	var wg sync.WaitGroup
	wg.Add(len(entries) + 1)
	for i, e := range entries {
		go func() {
			defer wg.Done()
			out.Tiers[i] = slcSweep(t, e, sig, noise, want, out.Seeds)
		}()
	}
	go func() {
		defer wg.Done()
		out.Captures = slcCaptures(raw)
	}()
	wg.Wait()
	b, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(slcBenchPath, append(b, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}
