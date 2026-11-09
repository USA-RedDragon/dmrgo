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
		if r.host().iqd.d.locked() {
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

// TestIQSeekLevelsHoldTune pins an MS call where an EMB seek anchored the
// empty slot on noise and the converter followed the levels of that slot.
func TestIQSeekLevelsHoldTune(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		tier Tier
		off  float64
		run  uint64
		name string
	}{
		{Tier1, 0, 9979531959472472741, "TestIQAdjacentPull/1/0"},
		{Tier2, 300, 11058614226070481730, "TestIQAdjacentPull/2/300"},
		{Tier2, 300, 8091554330429897612, "TestIQAdjacentPull/2/300"},
	} {
		o := tierOptions(c.tier)
		s := runMix(c.run, nameHash(c.name))
		if n, want, off := adjacentPin(t, c.tier, &o, c.off, s, s, 3); n < want || off > iqAFCLockHz {
			t.Errorf("%s: C0 and C1 of %d of %d voice frames exact, converter %.0f Hz off the carrier while locked", c.name, n, want, off)
		}
	}
}

func lockOffsetPin(t *testing.T, run uint64, name string, off, cn float64, spur float32, tier Tier) float64 {
	t.Helper()
	sc := testscen.Of(runMix(run, nameHash(name)))
	const msg = "DMRGO RXBENCH"
	ids := testscen.Of(runMix(sc.VoiceSeed, nameHash(msg))).Apply(rftx.SeqConfig{Source: 3009101, BS: 990913, Dest: 3009201, Message: msg})
	slots, _ := benchStreamOf(t, ids, 1)
	x := cleanBaseband(slots, off).noisy(t, cn, runMix(runMix(run, nameHash(name)), 7))
	at, _, ok := lockNCO(t, cu8Spur(x, 32, spur), basebandRate, tier)
	if !ok {
		t.Fatalf("%s at %.0f Hz: no lock", name, off)
	}
	return at
}

// TestIQHuntAfterLock pins Tier 0 locks at +-1400 Hz with a DC spur where
// the spectrum hunt tuned to its estimate right after the audio it flushed
// made the lock.
func TestIQHuntAfterLock(t *testing.T) {
	t.Parallel()
	for _, off := range []float64{1400, -1400} {
		if at := lockOffsetPin(t, 3524953757563366875, "TestIQLockOffsetDCSpur", off, 10, -0.5, Tier0); math.Abs(at-off) > iqAFCPullHz {
			t.Errorf("%+.0f Hz: converter at %.0f Hz at the lock", off, at)
		}
	}
}

// TestIQRefillSettle pins Tier 3 locks where the converter moved to the
// coherent lock frequency while the audio taken before that tune was still
// to be read, and the next SYNC fit tuned it again by the same offset.
func TestIQRefillSettle(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		run  uint64
		name string
		spur float32
	}{
		{2146918402700525728, "TestCoreTier3LockOffset", -0.5},
		{373987276557011745, "TestCoreTier3LockOffset", -0.5},
		{10339290622464889001, "TestIQLockOffsetDCSpur", -0.5},
	} {
		if at := lockOffsetPin(t, c.run, c.name, 1400, 10, c.spur, Tier3); math.Abs(at-1400) > iqAFCPullHz {
			t.Errorf("%s %d: converter at %.0f Hz at the lock, carrier at 1400 Hz", c.name, c.run, at)
		}
	}
}

// TestRxRawBitsHuntTune pins a held-out run where the spectrum hunt tuned a
// Tier 0 receiver by less than iqHuntMaxHz inside burst 0, whose symbols
// before the tune passed the channel filter 2.6 kHz off its center.
func TestRxRawBitsHuntTune(t *testing.T) {
	t.Parallel()
	p := pinnedRun(t, 14370708237565064112, "TestRxRawBitsAfterNoise")
	cfg := randomizedN(p, 3009101, testConfigFixed())
	for _, seq := range []string{"ack", "data12", "voice"} {
		built, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
		pad := int(0.2 * basebandRate)
		fm, env = append(make([]float64, pad), fm...), append(make([]float64, pad), env...)
		iq := channel(p, rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 2600, 25, 5)
		r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier0, Format: IQFormatCU8})
		if err != nil {
			t.Fatal(err)
		}
		got := r.AppendFlush(r.AppendBursts(nil, iq))
		if len(got) != len(built.Bursts) {
			t.Errorf("%s: %d bursts, want %d", seq, len(got), len(built.Bursts))
			continue
		}
		for i := range got {
			if got[i].Data != built.Bursts[i] {
				t.Errorf("%s burst %d: raw bits differ", seq, i)
			}
		}
	}
}

// TestIQLowCWLead pins held-out runs where the low-rate I/Q mean learned a
// carrier lead on the tuned frequency and the first burst lost bits before
// the carrier test let it go.
func TestIQLowCWLead(t *testing.T) {
	t.Parallel()
	for _, run := range []uint64{979335601447876854, 14328703105160663048} {
		for _, name := range []string{"s0.cw500ms.+0.ms.voice.0ppm.nom.cs16-9600.40dB.dc", "s0.cw2s.+0.ms.voice.0ppm.high.cs16-9600.40dB"} {
			row := mxRowOf(t, name)
			res, err := mxRun(pinnedRun(t, run, "TestIQLowDCOffset"), row, []mxRx{{row.val(mxFormat), Tier0}})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range res {
				if s := r.symptoms(); s != "" {
					t.Errorf("run %d %s: %s", run, name, s)
				}
			}
		}
	}
}

// TestDiscACAcquire pins held-out runs where the AC restorer took seconds to
// train: its symbol scale started from the first symbol alone, and a weak
// first SYNC held the search to its slot grid for acAcquire symbols.
func TestDiscACAcquire(t *testing.T) {
	t.Parallel()
	for _, run := range []uint64{2177666937959402496, 102544416862867516} {
		p := pinnedRun(t, run, "TestDiscACCoupled")
		slots, truth := benchStream(p, 16)
		x := noisyBaseband(p, slots, 1400, 30, 1)
		for _, hz := range []int{24000, 12000, 9600} {
			a := acCoupled(discAudio(x, hz), hz)
			for _, tier := range []Tier{Tier1, Tier2} {
				got, _ := receiveFixed(t, a, hz, tier)
				sc := scoreBench(truth, got)
				for _, k := range []string{"csbk", "rate12", "ambe"} {
					if s := sc[k]; s.Truth == 0 || s.Exact < s.Truth/2 {
						t.Errorf("run %d, %d Hz %v, %s: %d of %d recovered from AC-coupled audio", run, hz, tier, k, s.Exact, s.Truth)
					}
				}
			}
		}
	}
}

// TestRxLockRetuneCoherent pins a held-out run where Tier 2 locked with the
// carrier 3 kHz off the converter and decoded burst 0 from the symbols taken
// there, as the lock retune and replay skipped the coherent tiers.
func TestRxLockRetuneCoherent(t *testing.T) {
	t.Parallel()
	p := pinnedRun(t, 958015591593368716, "TestRxVoiceSlowClock")
	cfg := randomizedN(p, 3009101, rftx.SeqConfig{ColorCode: 1, Source: 3009101, Dest: 9990, BS: 990911, Wakeups: 2, Message: "DMRGO TEST MESSAGE", Superframes: 4})
	built, err := rftx.Sequence("voice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env = append(make([]float64, pad), fm...), append(make([]float64, pad), env...)
	iq := channel(p, rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6/(1-500e-6), 2.048e6, 3000, 25, 5)
	r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier2, Format: IQFormatCU8})
	if err != nil {
		t.Fatal(err)
	}
	got := r.AppendFlush(r.AppendBursts(nil, iq))
	if len(got) != len(built.Bursts) {
		t.Fatalf("%d bursts, want %d", len(got), len(built.Bursts))
	}
	for i := range got {
		if got[i].Data != built.Bursts[i] {
			t.Errorf("burst %d: raw bits differ", i)
		}
	}
}

// TestRxMidLockRetune pins a held-out run where Tier 1 locked on a SYNC
// found mid-burst after a 4.2 kHz hunt tune inside burst 0, and kept the
// symbols of that burst taken before the tune, as that lock path skipped
// the retune and refill of the other lock paths.
func TestRxMidLockRetune(t *testing.T) {
	t.Parallel()
	p := pinnedRun(t, 15169259909967258117, "TestRxRawBitsAfterNoise")
	cfg := randomizedN(p, 3009101, testConfigFixed())
	built, err := rftx.Sequence("ack", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env = append(make([]float64, pad), fm...), append(make([]float64, pad), env...)
	iq := channel(p, rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 4000, 25, 5)
	r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier1, Format: IQFormatCU8})
	if err != nil {
		t.Fatal(err)
	}
	got := r.AppendFlush(r.AppendBursts(nil, iq))
	if len(got) != len(built.Bursts) {
		t.Fatalf("%d bursts, want %d", len(got), len(built.Bursts))
	}
	for i := range got {
		if got[i].Data != built.Bursts[i] {
			t.Errorf("burst %d: raw bits differ", i)
		}
	}
}

// TestIQLowDCStart pins held-out runs where the low-rate I/Q receiver
// started on a call with a DC offset as strong as the carrier, and the I/Q
// mean of the first blocks, which still held a share of the signal, cost
// burst 0 its first bits. In run 171 the circle fits of the first two blocks
// failed on the ramp; in run 705 the first fit held on the ramp at 100 of the
// 236 units of radius, 145 units off the offset, and stayed in the mean.
func TestIQLowDCStart(t *testing.T) {
	t.Parallel()
	name := "s0.none.+0.ms.voice.0ppm.low.cs16-9600.40dB.dc"
	row := mxRowOf(t, name)
	for _, p := range []struct {
		run  uint64
		test string
	}{{3, "TestIQLowDCOffset"}, {171, "TestRxMatrix"}, {705, "TestRxMatrix"}, {782, "TestRxMatrix"}} {
		run := p.run
		res, err := mxRun(pinnedRun(t, run, p.test), row, []mxRx{{row.val(mxFormat), Tier0}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if s := r.symptoms(); s != "" {
				t.Errorf("run %d %s: %s", run, name, s)
			}
		}
	}
}

// TestRxLockFarHeld pins a held-out run where Tier 0 locked with the carrier
// 2.6 kHz off the converter, before any hunt tune, and put out burst 0 with
// the raw bits taken through the channel filter off its center.
func TestRxLockFarHeld(t *testing.T) {
	t.Parallel()
	p := pinnedRun(t, 7257307190297903594, "TestRxRawBitsAfterNoise")
	cfg := randomizedN(p, 3009101, testConfigFixed())
	built, err := rftx.Sequence("ack", cfg)
	if err != nil {
		t.Fatal(err)
	}
	fm, env := rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
	pad := int(0.2 * basebandRate)
	fm, env = append(make([]float64, pad), fm...), append(make([]float64, pad), env...)
	iq := channel(p, rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, -2600, 25, 5)
	r, err := NewReceiver(&RxConfig{Rate: 2.048e6, Tier: Tier0, Format: IQFormatCU8})
	if err != nil {
		t.Fatal(err)
	}
	got := r.AppendFlush(r.AppendBursts(nil, iq))
	if len(got) != len(built.Bursts) {
		t.Fatalf("%d bursts, want %d", len(got), len(built.Bursts))
	}
	for i := range got {
		if got[i].Data != built.Bursts[i] {
			t.Errorf("burst %d: raw bits differ", i)
		}
	}
}

// TestIQRefillInChannel pins a held-out run where an unheld Tier 3 receiver
// locked on the residue of a 40 dB stronger channel and the refill tune
// moved the converter 6.5 kHz off, past the channel edge.
func TestIQRefillInChannel(t *testing.T) {
	t.Parallel()
	o := tierOptions(Tier3)
	o.IQTuneMaxHz, o.IQTuneSyncOnly, o.IQSpecHunt = 0, false, false
	s := runMix(11190316752511572895, nameHash("TestTier3Adjacent"))
	if _, _, off := adjacentPin(t, Tier3, &o, 0, s, s, 7); off >= channelEdgeHz {
		t.Errorf("converter %.0f Hz off the carrier, outside the channel", off)
	}
}

// TestIQSyncTuneTwoSlots pins held-out runs where an MS talker 13 dB weaker
// on the other slot, 250 Hz off, moved the SYNC fit of a sync-only receiver
// next to an AM neighbor: the tune followed each slot's fit, and burst 7 lost
// bits before a 10 dB level step.
func TestIQSyncTuneTwoSlots(t *testing.T) {
	t.Parallel()
	name := "mid.none.-1500.ms2.pi.-500ppm.nom.cs16-48k.40dB.step10dB.am3-60"
	row := mxRowOf(t, name)
	for _, run := range []uint64{3, 4, 8} {
		res, err := mxRun(pinnedRun(t, run, "TestRxMatrix"), row, []mxRx{{row.val(mxFormat), Tier0}})
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range res {
			if s := r.symptoms(); s != "" {
				t.Errorf("run %d %s: %s", run, name, s)
			}
		}
	}
}

// TestIQSyncTuneFitNoise pins weak calls on the carrier, 2 dB of C/N, where
// the sync-only tune moved a locked Tier 0 converter by the noise of the
// SYNC fit level average while the carrier stayed put.
func TestIQSyncTuneFitNoise(t *testing.T) {
	t.Parallel()
	for run := uint64(1); run <= 5; run++ {
		p := pinnedRun(t, run, "TestIQSyncTuneFitNoise")
		slots, _ := benchStream(p, 4)
		x := cleanBaseband(slots, 0).noisy(p, 2, 7)
		r := mustNew(RxConfig{Rate: basebandRate, Tier: Tier0, Format: IQFormatCU8})
		h := r.host()
		moves, seen, last := 0, false, h.iqd.conv.hz
		for k := 0; k < len(x); k += 960 {
			if _, err := r.writeBB(x[k:min(k+960, len(x))]); err != nil {
				t.Fatal(err)
			}
			if hz := h.iqd.conv.hz; hz != last {
				if h.iqd.d.locked() && seen {
					moves++
				}
				last = hz
			}
			seen = seen || h.iqd.d.locked()
		}
		if moves > 0 {
			t.Errorf("run %d: converter moved %d times while locked on a carrier at 0 Hz", run, moves)
		}
	}
}
