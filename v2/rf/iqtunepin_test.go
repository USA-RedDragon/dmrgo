//go:build !tinygo

package rf

import (
	"math"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
)

func adjacentPin(t *testing.T, tier Tier, o *options, offsetHz float64, wanted, other uint64, noise uint64) (n, want int, off float32) {
	t.Helper()
	cfg := testscen.Of(wanted).Apply(testConfigFixed())
	cfg.ColorCode, cfg.Superframes, cfg.Wakeups, cfg.Slot = 1, 2, 0, 1
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 1})
	const calls = 3
	gap := make([]float64, int(0.3*basebandRate))
	var fmAll, envAll []float64
	frames := make([]uint64, 0, calls*18*cfg.Superframes)
	for c := range calls {
		if c > 0 {
			fmAll, envAll = append(fmAll, gap...), append(envAll, gap...)
		}
		fmAll, envAll = append(fmAll, fm...), append(envAll, env...)
		for i := range 18 * cfg.Superframes {
			frames = append(frames, rftx.AMBEValue(rftx.VoiceFrame(cfg.VoiceSeed, i)))
		}
	}
	ic := testscen.Of(other).Apply(testConfigFixed())
	ic.ColorCode, ic.Superframes, ic.Wakeups, ic.Slot, ic.BSSourced = 5, 4, 0, 0, true
	ib, err := rftx.Sequence("voice", ic)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := rftx.ModulateBS(ib.Slots)
	x := msBaseband(fmAll, envAll, offsetHz, 20, noise)
	var ph float64
	for i := range x {
		ph += 2 * math.Pi * (fi[i%len(fi)] + 12500) / basebandRate
		x[i] += complex(float32(100*math.Cos(ph)), float32(100*math.Sin(ph)))
	}
	var out []Burst
	r := mustNewOpt(RxConfig{Rate: basebandRate, Tier: tier, OnBursts: each(func(b *Burst) { out = append(out, *b) }), Format: IQFormatCU8}, *o)
	for k := 0; k < len(x); k += 960 {
		must(r.writeBB(x[k:min(k+960, len(x))]))
		if r.host().iqd.d.locked {
			off = max(off, abs32(r.host().iqd.conv.hz-float32(offsetHz)))
		}
	}
	r.flushAll()
	return c0c1InOrder(frames, voiceFrames(out)), len(frames), off
}

// TestIQUnheldAdjacentWalk pins an interferer payload whose filter residue
// walked an unlocked converter with no tuning hold from 0 to 12.5 kHz, where
// it locked on the adjacent channel and decoded none of the wanted frames.
func TestIQUnheldAdjacentWalk(t *testing.T) {
	t.Parallel()
	o := discOptions()
	o.IQDisc, o.IQHz = true, 12000
	if n, want, off := adjacentPin(t, Tier0, &o, 300, 1000, 2001, 7); n < want || off >= iqTuneMaxHz {
		t.Errorf("C0 and C1 of %d of %d voice frames exact, converter %.0f Hz off the carrier", n, want, off)
	}
}

// TestIQSyncTuneAdjacent pins a payload pair where the lag-1 mean of a
// receiver tuned on the SYNC fits read the carrier 300 Hz off 20 dB of C/N
// next to a 40 dB stronger channel at +12.5 kHz, and held the tune back.
func TestIQSyncTuneAdjacent(t *testing.T) {
	t.Parallel()
	o := discOptions()
	o.IQDisc, o.IQHz, o.IQTuneMaxHz, o.IQTuneSyncOnly = true, 12000, 2500, true
	for _, s := range []uint64{0, 2, 5, 11} {
		if n, want, off := adjacentPin(t, Tier0, &o, 300, 1000+s, 2000+s, 7); n < want || off > iqAFCLockHz {
			t.Errorf("payload %d: C0 and C1 of %d of %d voice frames exact, converter %.0f Hz off the carrier while locked", s, n, want, off)
		}
	}
}
