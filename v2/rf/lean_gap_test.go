package rf

import (
	"encoding/hex"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/enums"
	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/layer2"
)

func TestLeanGapNoise(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.ColorCode, cfg.Wakeups, cfg.Slot, cfg.Preambles = 1, 0, 1, 2
	built, err := rftx.Sequence("preamble", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	want := map[string]int{}
	jit := rand.New(rand.NewPCG(7, 8))
	const calls = 120
	freq := make([]float64, 0, calls*(int(0.3*basebandRate)+10+len(fm)))
	amp := make([]float64, 0, cap(freq))
	for range calls {
		g := int(0.3*basebandRate) + jit.IntN(10)
		freq = append(freq, make([]float64, g)...)
		amp = append(amp, make([]float64, g)...)
		freq = append(freq, fm...)
		amp = append(amp, env...)
		for _, b := range built.Bursts {
			if x, err := layer2.NewBurstFromBytes(b); err == nil && x.Kind == layer2.KindData {
				p, _ := b.BPTCPayload()
				want[hex.EncodeToString(p[:])]++
			}
		}
	}
	for _, tier := range []Tier{Tier0, Tier1} {
		x := noisyFM(freq, amp, 0, 17, 24)
		got := map[string]int{}
		for _, b := range receiveBaseband(x, tier) {
			data, _ := b.Decoded()
			if y, err := layer2.NewBurstFromBytes(data); err == nil && y.Kind == layer2.KindData {
				p, _ := data.BPTCPayload()
				got[hex.EncodeToString(p[:])]++
			}
		}
		for k, n := range want {
			if got[k] < n {
				t.Errorf("Tier%d: %d of %d bursts of payload %s", tier-Tier0, got[k], n, k)
			}
		}
	}
}

func TestLeanGapNoiseSpread(t *testing.T) {
	t.Parallel()
	p := enums.SyncMSSourcedData
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	r := &leanCore{devScale: 1, lastAnch: 10}
	end := int64(1000)
	r.ns = end - leanSyncEnd + slotSymbols
	r.head = uint8(ringPos(r.ns))
	rng := rand.New(rand.NewPCG(1, 2))
	var e [syncSymbols]float64
	for k := range e {
		e[k] = rng.NormFloat64()
	}
	sigma := 1.05 * 2 * deviationHz
	spread := func() float32 {
		g, _, res := r.fitSync(end, p)
		return sqrtf(res) / (2 * g * deviationHz)
	}
	for range 8 {
		for k := range syncSymbols {
			v := deviationHz*float64(syms[k]) + sigma*e[k]
			r.ring[ringPos(end-syncSymbols+1+int64(k))] = int8(max(min(math.Round(v/discStepHz), 127), -127))
		}
		sigma *= 1.05 / float64(spread())
	}
	if s := spread(); s < 1.03 || s > 1.07 {
		t.Fatalf("spread %.3f", s)
	}
	clicks := func() int {
		n := 0
		for k := end - leanSyncEnd; k < end-leanSyncEnd+slotSymbols; k++ {
			if abs32(r.sym(k)) > float32(leanClickOut)/2*deviationHz {
				n++
			}
		}
		return n
	}
	for k := end - leanSyncEnd; clicks() < 32; k += 3 {
		if k <= end-syncSymbols || k > end {
			r.ring[ringPos(k)] = int8(100 * (1 - 2*(k&1)))
		}
	}
	g, _, res := r.fitSync(end, p)
	r.discVar = res / (6 * 6 * g * g)
	if !r.noCarrier(end, p) {
		t.Error("noise with 32 clicks and a spread of 1.05 anchors")
	}
	r.discVar = res / (2 * 2 * g * g)
	if r.noCarrier(end, p) {
		t.Error("a SYNC that spreads twice as much as the anchors does not anchor")
	}
}

func TestLeanReplayNoise(t *testing.T) {
	t.Parallel()
	p := enums.SyncMSSourcedVoice
	var syms [syncSymbols]int8
	syncSyms(p, &syms)
	end := int64(5000)
	base := end - leanSyncEnd
	r := &leanCore{devScale: 1, lastAnch: leanNoSlot, replaying: true}
	r.first, r.next = base-3*slotSymbols, 4
	r.ns = base + 10*slotSymbols
	r.head = uint8(ringPos(r.ns))
	rng := rand.New(rand.NewPCG(3, 4))
	for k := base - 1; k <= base+slotSymbols; k++ {
		r.ring[ringPos(k)] = int8(rng.IntN(41) - 20)
	}
	var e [syncSymbols]float64
	for k := range e {
		e[k] = rng.NormFloat64()
	}
	sigma := 1.6 * 2 * deviationHz
	spread := func() float32 {
		g, _, res := r.fitSync(end, p)
		return sqrtf(res) / (2 * g * deviationHz)
	}
	for range 8 {
		for k := range syncSymbols {
			v := deviationHz*float64(syms[k]) + sigma*e[k]
			r.ring[ringPos(end-syncSymbols+1+int64(k))] = int8(max(min(math.Round(v/discStepHz), 127), -127))
		}
		sigma *= 1.6 / float64(spread())
	}
	if s := spread(); s < 1.55 || s > 1.65 {
		t.Fatalf("spread %.3f", s)
	}
	clicks := func() int {
		n := 0
		for k := base; k < base+slotSymbols; k++ {
			if abs32(r.sym(k)) > float32(leanClickOut)/2*deviationHz {
				n++
			}
		}
		return n
	}
	for k := base; clicks() < 50; k += 2 {
		if k <= end-syncSymbols || k > end {
			r.ring[ringPos(k)] = int8(100 * (1 - 2*(k&1)))
		}
	}
	g, _, res := r.fitSync(end, p)
	r.discVar = res / (7 * 7 * g * g)
	if !r.noCarrier(end, p) {
		t.Error("noise with 50 clicks and 7 times the spread of the lock anchors")
	}
	r.discVar = res / (1.5 * 1.5 * g * g)
	if r.noCarrier(end, p) {
		t.Error("a weak SYNC with 1.5 times the spread of the lock does not anchor")
	}
}
