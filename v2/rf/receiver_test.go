package rf

import (
	"math"
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// noisyBaseband modulates a BS outbound stream at basebandRate with a
// carrier offset, white Gaussian noise at C/N cn dB in 12.5 kHz and 0.3 s
// of noise before and after.
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

// basebandStream is noisyBaseband at 40 dB.
func basebandStream(slots []rftx.BSSlot, offsetHz float64) []complex64 {
	return noisyBaseband(slots, offsetHz, 40, 1)
}

// receiveBaseband runs x through a receiver of tier t.
func receiveBaseband(x []complex64, t Tier) []Burst {
	r := mustNew(Config{Rate: basebandRate, Tier: t})
	var out []Burst
	for k := 0; k < len(x); k += 4096 {
		out = append(out, r.WriteBaseband(x[k:min(k+4096, len(x))])...)
	}
	return append(out, r.Flush()...)
}

// exactFrames counts the AMBE+2 frames of the voice bursts of slots that
// got decodes exactly, matching the bursts by time.
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

// AMBEFrameValue is the 49 bits of an AMBE+2 frame as a number.
type AMBEFrameValue = uint64

// TestReceiverTiersAtLowSNR receives a BS outbound voice call at 3 dB C/N
// and checks that TierDisc, TierCoherent and TierExact each recover more
// AMBE+2 frames exactly than the tier below.
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
	for _, tier := range []Tier{TierDisc, TierCoherent, TierExact} {
		exact, total := exactFrames(built.Slots, receiveBaseband(x, tier))
		t.Logf("tier %d: %d of %d frames exact", tier, exact, total)
		if exact <= prev {
			t.Errorf("tier %d: %d frames exact, the tier below %d", tier, exact, prev)
		}
		prev = exact
	}
}

// TestReceiverHalfRate receives a BS outbound voice call at 12 dB C/N as
// baseband at BasebandRate and, averaged over pairs of samples, at half
// that rate, and checks that TierDisc recovers about as many AMBE+2 frames
// from each.
func TestReceiverHalfRate(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 4, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 12, 6)
	full, total := exactFrames(built.Slots, receiveBaseband(x, TierDisc))
	h := make([]complex64, len(x)/2)
	for i := range h {
		h[i] = (x[2*i] + x[2*i+1]) / 2
	}
	r := mustNew(Config{Tier: TierDisc, BasebandHz: BasebandRate / 2})
	var got []Burst
	for k := 0; k < len(h); k += 2048 {
		got = append(got, r.WriteBaseband(h[k:min(k+2048, len(h))])...)
	}
	got = append(got, r.Flush()...)
	half, _ := exactFrames(built.Slots, got)
	t.Logf("%d of %d frames exact at %d Hz, %d at %d Hz", full, total, BasebandRate, half, BasebandRate/2)
	if full < total*9/10 || half < full-total/20 {
		t.Errorf("%d of %d frames exact at %d Hz, %d at %d Hz", full, total, BasebandRate, half, BasebandRate/2)
	}
	if _, err := New(Config{Tier: TierCoherent, BasebandHz: BasebandRate / 2}); err == nil {
		t.Error("TierCoherent takes input at half rate")
	}
}

// TestReceiverOnBurst checks that a receiver with OnBurst hands it the
// bursts that the returned slices hold without it, and returns none.
func TestReceiverOnBurst(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 10, 4)
	for _, tier := range []Tier{TierDisc, TierCoherent} {
		want := receiveBaseband(x, tier)
		var got []Burst
		r := mustNew(Config{Rate: basebandRate, Tier: tier, OnBurst: func(b Burst) { got = append(got, b) }})
		for k := 0; k < len(x); k += 4096 {
			if out := r.WriteBaseband(x[k:min(k+4096, len(x))]); len(out) != 0 {
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

// BenchmarkReceiver times each tier on a BS outbound voice call at 6 dB
// C/N, in nanoseconds per second of signal, with its allocations.
func BenchmarkReceiver(b *testing.B) {
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		b.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 6, 3)
	for _, tier := range []Tier{TierDisc, TierCoherent, TierExact, TierMax} {
		b.Run(strconv.Itoa(int(tier)), func(b *testing.B) {
			r := mustNew(Config{Rate: basebandRate, Tier: tier})
			b.ReportAllocs()
			b.ResetTimer()
			start := time.Now()
			for b.Loop() {
				for k := 0; k < len(x); k += 4096 {
					r.WriteBaseband(x[k:min(k+4096, len(x))])
				}
				r.Flush()
			}
			b.ReportMetric(float64(time.Since(start).Nanoseconds())/float64(b.N)/(float64(len(x))/basebandRate), "ns/signal-s")
		})
	}
}

// TestReceiverWithoutWarmUp receives a BS outbound voice call at 20 dB C/N
// with WarmUp off and checks that the coherent receiver still decodes it.
func TestReceiverWithoutWarmUp(t *testing.T) {
	t.Parallel()
	cfg := testConfig()
	cfg.BSSourced, cfg.Superframes, cfg.Slot = true, 2, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	x := noisyBaseband(built.Slots, 1200, 20, 5)
	o := tierOptions(TierCoherent)
	o.WarmUp = false
	r := mustNew(Config{Rate: basebandRate, opt: &o})
	var got []Burst
	for k := 0; k < len(x); k += 4096 {
		got = append(got, r.WriteBaseband(x[k:min(k+4096, len(x))])...)
	}
	got = append(got, r.Flush()...)
	exact, total := exactFrames(built.Slots, got)
	if exact < total*9/10 {
		t.Errorf("%d of %d frames exact", exact, total)
	}
}
