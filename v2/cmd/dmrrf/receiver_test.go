package main

import (
	"math"
	"math/rand/v2"
	"testing"
)

// noisyBaseband modulates a BS outbound stream at basebandRate with a
// carrier offset, white Gaussian noise at C/N cn dB in 12.5 kHz and 0.3 s
// of noise before and after.
func noisyBaseband(slots []BSSlot, offsetHz, cn float64, seed uint64) []complex64 {
	fm, env := ModulateBS(slots)
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
func basebandStream(slots []BSSlot, offsetHz float64) []complex64 {
	return noisyBaseband(slots, offsetHz, 40, 1)
}

// receiveBaseband runs x through a receiver of tier t.
func receiveBaseband(x []complex64, t Tier) []Received {
	r := NewReceiver(ReceiverConfig{Rate: basebandRate, Tier: t})
	var out []Received
	for k := 0; k < len(x); k += 4096 {
		out = append(out, r.WriteBaseband(x[k:min(k+4096, len(x))])...)
	}
	return append(out, r.Flush()...)
}

// exactFrames counts the AMBE+2 frames of the voice bursts of slots that
// got decodes exactly, matching the bursts by time.
func exactFrames(slots []BSSlot, got []Received) (exact, total int) {
	const pad = 0.3
	ramp := (slotSeconds - burstSeconds) / 2
	byIndex := map[int]Received{}
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
			data, _ := r.decoded()
			frames, _ := data.AMBEFrames()
			for k := range frames {
				f[k] = ambeValue(frames[k])
			}
		}
		for k := range want {
			total++
			if ok && f[k] == ambeValue(want[k]) {
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
	built, err := Sequence("voice", cfg)
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
