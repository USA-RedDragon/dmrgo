//go:build !tinygo

package rf

import (
	"fmt"
	"math"
	"strconv"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
)

// genieAs stands for the test named name, so that a scene draws the
// payloads and the noise that test draws at the same run seed.
type genieAs struct {
	testing.TB
	name string
}

func (g genieAs) Name() string { return g.name }

// genieMS builds the MS calls of seqs on cfg one after the other, each
// after lead(k) samples of silence and before tail samples of it, padded to
// whole slots when align is set, with the time of every burst from the
// start of freq.
func genieMS(t testing.TB, cfg rftx.SeqConfig, seqs []string, lead func(k int) int, tail int, align bool) (freq, env []float64, truth []mxTruth) {
	guard := (slotSeconds - burstSeconds) / 2
	slotN := int(slotSeconds * basebandRate)
	for k, seq := range seqs {
		b, err := rftx.Sequence(seq, cfg)
		if err != nil {
			t.Fatal(err)
		}
		n := lead(k)
		freq, env = append(freq, make([]float64, n)...), append(env, make([]float64, n)...)
		t0 := float64(len(freq)) / basebandRate
		f, e := rftx.Modulate(b.Bursts, rftx.Schedule{Slot: cfg.Slot})
		for i, raw := range b.Bursts {
			truth = append(truth, mxTruth{at: t0 + float64(2*i+cfg.Slot)*slotSeconds + guard, data: raw, g: k})
		}
		freq, env = append(append(freq, f...), make([]float64, tail)...), append(append(env, e...), make([]float64, tail)...)
		for align && len(freq)%slotN != 0 {
			freq, env = append(freq, 0), append(env, 0)
		}
	}
	return freq, env, truth
}

func genieShift(truth []mxTruth, d float64) []mxTruth {
	out := make([]mxTruth, len(truth))
	for i, tr := range truth {
		tr.at += d
		out[i] = tr
	}
	return out
}

type genieBuild = func(t testing.TB) ([]*genieCase, error)

func genieRowsAs(test string, sub bool, rows ...string) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		out := make([]*genieCase, 0, len(rows))
		for _, r := range rows {
			row := mxRowName(r)
			if row.name() != r {
				return nil, fmt.Errorf("row %s reads as %s", r, row.name())
			}
			name := test
			if sub {
				name += "/" + r
			}
			c, err := genieRowCase(genieAs{t, name}, row, nil)
			if err != nil {
				return nil, err
			}
			out = append(out, c)
		}
		return out, nil
	}
}

func genieJoin(bs ...genieBuild) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		var out []*genieCase
		for _, b := range bs {
			c, err := b(t)
			if err != nil {
				return nil, err
			}
			out = append(out, c...)
		}
		return out, nil
	}
}

func genieBenchAs(test string, n int, cn, hz float64, seeds []uint64, modes ...string) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		slots, _ := benchStream(tt, n)
		out := make([]*genieCase, 0, len(seeds)*len(modes))
		for _, seed := range seeds {
			x := noisyBaseband(tt, slots, hz, cn, seed)
			for _, m := range modes {
				out = append(out, genieBenchCase(fmt.Sprintf("%s %d sf %+.0f Hz %g dB seed %d %s", test, n, hz, cn, seed, m), slots, x, hz, 0, m))
			}
		}
		return out, nil
	}
}

func genieSeeds(from, n uint64) []uint64 {
	s := make([]uint64, n)
	for i := range s {
		s[i] = from + uint64(i)
	}
	return s
}

// genieScenes gives the scenes of the harness, one or more for each item of
// the rf acceptance list whose question is a per-burst one: is each burst
// that a test finds lost or wrong a loss of the receiver or a physical
// limit of the input, and at which timing does the reference lose it. A
// scene that mirrors a test draws the payloads and the noise of that test
// at the same run seed, so a repro seed of the test repeats its failure.
func genieScenes() []genieScene {
	return []genieScene{
		{name: "a01-matrix", q: "TestRxMatrix rows at 40 dB and inf (deviation, seconds, first burst missing, extra burst)",
			tiers: allTiers, build: genieRowsAs("TestRxMatrix", true,
				"agc.none.+700.ms2.data1.0ppm.high.a9600.40dB",
				"gap.none.+0.ms.csbk.0ppm.nom.cs16-48k.40dB.dmr40",
				"gap.none.-4000.ms2.data1.+500ppm.nom.cs16-48k.inf",
				"gap.cw500ms.-4000.ms.voice.-500ppm.nom.a9600.40dB")},
		{name: "a02-direct-reentry", q: "TestReceiverDirectModeTiming/0: the frames of the later calls of a held MS stream that Tier 3 misses at 5 dB",
			tiers: []Tier{Tier3}, build: genieDirect("TestReceiverDirectModeTiming/0", 5, 6)},
		{name: "a03-level-step", q: "TestRxLevelStep: the CACH and bits of the burst of a +10 dB step at 2, 3 and 12 ms",
			build: genieLevelStep()},
		{name: "a04-early-clock", q: "TestCoreTier3EarlySlotsClock: the frames of the first 12 slots at +5000 Hz, +-500 ppm, 3 dB, and the clock stretch of the replay",
			tiers: []Tier{Tier3}, build: genieEarlyClock("TestCoreTier3EarlySlotsClock", 3, genieSeeds(7000, 24))},
		{name: "a05-two-talker-clip", q: "TestRxTwoTalkerClipStep: the burst of talker 2 after a 10 dB rise at Tiers 0 and 1",
			tiers: []Tier{Tier0, Tier1}, build: genieRowsAs("TestRxTwoTalkerClipStep", false, "gap.none.+2500.ms2.data1.-500ppm.nom.a12000.40dB.step10dB")},
		{name: "a06-stream-end-noise", q: "TestRxStreamEndNoise: the bursts out of the noise after a stream is cut",
			tiers: allTiers, build: genieStreamEnd("TestRxStreamEndNoise", genieSeeds(1, 8))},
		{name: "a07-two-talker-level", q: "TestRxAudioTwoTalkerLevelStep: the flywheel bursts after a 10 dB step between two talkers",
			tiers: allTiers, build: genieRowsAs("TestRxAudioTwoTalkerLevelStep", false, "gap.none.+2500.ms2.pi.-500ppm.nom.a12000.40dB.step10dB")},
		{name: "a08-tail-readings", q: "TestReceiverReadingsAfterLevelStep: the extra bursts and the readings after a carrier tail",
			tiers: allTiers, build: genieTail("TestReceiverReadingsAfterLevelStep")},
		{name: "a09-lowiq-isi", q: "TestLowIQISI: the frames that low-rate I/Q Tier 0 loses on some payloads at 25 dB",
			tiers: []Tier{Tier0}, build: genieBenchAs("TestLowIQISI", 30, 25, 1400, genieSeeds(1, 3), "iqlow")},
		{name: "a12-late-entry", q: "late entry at 40 dB, audio Tiers 2 and 3 (approximates TestRxRobust ms and msj)",
			tiers: []Tier{Tier2, Tier3}, build: genieRowsAs("TestGenie/a12", true, "mid.none.+0.ms.voice.0ppm.nom.a48000.40dB", "mid.none.-700.ms2.voice.0ppm.nom.a48000.40dB")},
		{name: "a13-seek-second", q: "TestRxSeekSecondTalker at 40 dB, Tiers 1 and 2: the bursts of the second talker",
			tiers: []Tier{Tier1, Tier2}, build: genieRowsAs("TestRxSeekSecondTalker", false, "mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB", "mid.none.-700.ms2.voice.-500ppm.nom.cs16-48k.40dB.dmr40")},
		{name: "a14-weak-direct", q: "the carrier reading and the bursts of a direct mode call at 0.5 dB (approximates TestSpectrumWeakDirectMode)",
			tiers: []Tier{Tier1, Tier2, Tier3}, build: genieDirect("TestGenie/a14", 0.5, 2)},
		{name: "a15-first-lock", q: "TestCoreFirstLock: the calls that never lock at 0 dB, Tiers 2 and 3",
			tiers: []Tier{Tier2, Tier3}, build: genieDraws("TestCoreFirstLock", 6, 12, 4, 7000, 0)},
		{name: "a16-spec-hunt", q: "TestIQSpecHunt: the frames lost at +-6000 Hz, 10 dB, Tiers 0 and 1",
			tiers: []Tier{Tier0, Tier1, Tier2}, build: genieJoin(genieBenchAs("TestIQSpecHunt", 6, 10, 6000, []uint64{4}, "iq"), genieBenchAs("TestIQSpecHunt", 6, 10, -6000, []uint64{4}, "iq"))},
		{name: "a17-one-stream", q: "TestCoreTier3OneStream: a call at -2.5 dB split into 2 streams",
			tiers: []Tier{Tier3}, build: genieBenchAs("TestCoreTier3OneStream", 10, -2.5, 1400, genieSeeds(7100, 8), "iq")},
		{name: "a18-disc-weak", q: "TestDiscTier2Weak: the dips of the Tier 2 audio share at 3 dB",
			tiers: []Tier{Tier0, Tier2}, build: genieBenchAs("TestDiscTier2Weak", 10, 3, 1400, genieSeeds(1, discWeakSeeds), "a24000", "a12000", "a9600")},
		{name: "a19-false-data-sync", q: "F113: a voice burst taken as data at 40 dB, payload 1001116",
			tiers: allTiers, build: geniePinned(1001116)},
		{name: "a20-fade-gate", q: "TestRxStreamEndKeepsFades (F108): the faded voice frames lost in a 5 Hz Rayleigh fade at 10 dB",
			tiers: []Tier{Tier0, Tier3}, build: genieFade("TestRxStreamEndKeepsFades", genieSeeds(9000, 4))},
		{name: "a21-weak-proof", q: "TestCoreTier3WeakProof: the SYNCs lost at -2.5 dB",
			tiers: []Tier{Tier3}, build: genieDraws("TestCoreTier3WeakProof", 30, 4, 2, 70, -2.5)},
		{name: "a23-pair-lock", q: "TestLeanPairLock: the seeds that give no burst at 2 dB, Tier 0 at 2 MHz",
			tiers: []Tier{Tier0}, build: geniePairLock("TestLeanPairLock", genieSeeds(1, 16))},
	}
}

func genieDirect(test string, cn float64, seeds int) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		cfg := directConfig()
		if !fixedPayload(tt) {
			cfg = sharedRandomized(cfg)
		}
		f, e, truth := genieMS(tt, cfg, []string{"voice", "voice", "voice", "voice"}, func(k int) int { return k * 37 % 100 * samplesPerSy / 100 }, int(0.3*basebandRate), true)
		out := make([]*genieCase, 0, seeds)
		for s := range uint64(seeds) {
			x := noisyFM(tt, f, e, 1400, cn, 40+s)
			out = append(out, genieBBCase(fmt.Sprintf("%s %g dB seed %d", test, cn, 40+s), genieShift(truth, benchPad), x, 1400, 1, "iq"))
		}
		return out, nil
	}
}

func genieLevelStep() genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		var out []*genieCase
		for _, c := range []struct {
			off   string
			ms    float64
			tiers []Tier
		}{{"-2500", 12, []Tier{Tier3}}, {"+0", 12, []Tier{Tier3}}, {"+0", 2, []Tier{Tier0}}, {"+0", 3, []Tier{Tier0}}, {"-2500", 3, []Tier{Tier0}}} {
			row := mxRowName("s0.none." + c.off + ".bs.data12.0ppm.nom.a24000.inf")
			for _, tier := range c.tiers {
				name := fmt.Sprintf("TestRxLevelStep/%+.0fdB/%s/%gms/t%d", 10.0, c.off, c.ms, tier-Tier0)
				gc, err := genieRowCase(genieAs{t, name}, row, func(s *mxSignal) {
					s.step = s.truth[4].at + c.ms/1000
					s.gain = [2]float64{math.Pow(10, -10.0/20), 1}
				})
				if err != nil {
					return nil, err
				}
				gc.name, gc.only = name, []Tier{tier}
				out = append(out, gc)
			}
		}
		return out, nil
	}
}

func genieEarlyClock(test string, cn float64, seeds []uint64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		slots, _ := benchStream(tt, 6)
		start := (slotSeconds-burstSeconds)/2 + float64(cachSymbols)/symbolRate + benchPad
		n := int((start + 16*slotSeconds) * basebandRate)
		out := make([]*genieCase, 0, 2*len(seeds))
		for _, ppm := range []float64{-500, 500} {
			sig := clockBaseband(slots, ppm)
			sig.re, sig.im = sig.re[:n], sig.im[:n]
			for _, seed := range seeds {
				x := sig.noisy(tt, cn, seed)
				out = append(out, genieBenchCase(fmt.Sprintf("+5000 Hz %+g ppm %g dB seed %d", ppm, cn, seed), slots, x, 5000, ppm, "iq"))
			}
		}
		return out, nil
	}
}

func genieDraws(test string, sf, draws, seeds int, seed0 uint64, cn float64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		out := make([]*genieCase, 0, draws*seeds)
		for d := range uint64(draws) {
			slots, _ := benchDraw(tt, sf, d)
			sig := cleanBaseband(slots, 1400)
			for i := range uint64(seeds) {
				seed := seed0 + d*uint64(seeds) + i
				out = append(out, genieBenchCase(fmt.Sprintf("draw %d seed %d %g dB", d, seed, cn), slots, sig.noisy(tt, cn, seed), 1400, 0, "iq"))
			}
		}
		return out, nil
	}
}

func genieStreamEnd(test string, seeds []uint64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		slots, _ := benchStream(tt, 6)
		out := make([]*genieCase, 0, 2*len(seeds))
		for _, seed := range seeds {
			x := noisyBaseband(tt, slots[:40], 1400, 10, seed)
			for _, m := range []string{"iq", "a" + strconv.Itoa(benchAudioHz)} {
				out = append(out, genieBenchCase(fmt.Sprintf("40 slots then noise, 10 dB seed %d %s", seed, m), slots[:40], x, 1400, 0, m))
			}
		}
		return out, nil
	}
}

func genieTail(test string) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		cfg := randomizedN(tt, 3009101, rftx.SeqConfig{ColorCode: 1, Source: 3009101, Dest: 9990, BS: 990911, Wakeups: 2, Message: "DMRGO TEST MESSAGE"})
		built, err := rftx.Sequence("data12", cfg)
		if err != nil {
			return nil, err
		}
		slots, _ := benchStream(tt, 1)
		ramp := (slotSeconds - burstSeconds) / 2
		out := make([]*genieCase, 0, 6)
		for _, ms := range []bool{true, false} {
			for _, tail := range []string{"no carrier", "unmodulated carrier", "carrier 2 kHz away"} {
				fm, env := rftx.ModulateBS(slots)
				truth := make([]mxTruth, 0, len(slots))
				if ms {
					fm, env = rftx.Modulate(built.Bursts, rftx.Schedule{Slot: 0})
					for i, b := range built.Bursts {
						truth = append(truth, mxTruth{at: float64(2*i)*slotSeconds + ramp, data: b, g: 2 * i})
					}
				} else {
					for i, s := range slots {
						truth = append(truth, mxTruth{at: ramp + float64(i)*slotSeconds + float64(cachSymbols)/symbolRate, data: s.Burst, cach: s.CACH, hasCACH: true, bs: true, g: i})
					}
				}
				n := len(fm)
				fm, env = append(fm, make([]float64, 4*n)...), append(env, make([]float64, 4*n)...)
				if tail != "no carrier" {
					for i := n; i < len(env); i++ {
						env[i] = 1
					}
				}
				if tail == "carrier 2 kHz away" {
					for i := n; i < len(fm); i++ {
						fm[i] = 2000
					}
				}
				iq := channel(tt, rftx.ToIQ(fm, env, 2e6, 0, 0.9), 2e6, 2.048e6, 3000, 25, 1)
				out = append(out, genieIQBytesCase(fmt.Sprintf("ms %v then %s", ms, tail), iq, IQFormatCU8, 2048000, truth, 3000, 1))
			}
		}
		return out, nil
	}
}

func geniePinned(run uint64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		p := pinnedTB{t, run}
		slots, _ := benchStream(p, 6)
		x := noisyBaseband(p, slots, 1400, 40, 1)
		return []*genieCase{genieBenchCase(fmt.Sprintf("payload %d 40 dB iq", run), slots, x, 1400, 0, "iq"), genieBenchCase(fmt.Sprintf("payload %d 40 dB audio", run), slots, x, 1400, 0, "a9600")}, nil
	}
}

func genieFade(test string, seeds []uint64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		slots, _ := benchStream(genieAs{t, test}, 6)
		fm, env := rftx.ModulateBS(slots)
		out := make([]*genieCase, 0, 2*len(seeds))
		for _, seed := range seeds {
			x := fadedBaseband(fm, env, 5, 10, seed)
			for _, m := range []string{"iq", "a" + strconv.Itoa(benchAudioHz)} {
				out = append(out, genieBenchCase(fmt.Sprintf("Rayleigh 5 Hz 10 dB seed %d %s", seed, m), slots, x, 1200, 0, m))
			}
		}
		return out, nil
	}
}

func geniePairLock(test string, seeds []uint64) genieBuild {
	return func(t testing.TB) ([]*genieCase, error) {
		tt := genieAs{t, test}
		slots, _ := benchStream(tt, 4)
		fm, env := rftx.ModulateBS(slots)
		cut := int(0.3 * basebandRate)
		tx := rftx.ToIQ(fm[cut:], env[cut:], 2e6, -320, 0.9)
		ramp := (slotSeconds - burstSeconds) / 2
		truth := make([]mxTruth, 0, len(slots))
		for i, s := range slots {
			if at := ramp + float64(i)*slotSeconds + float64(cachSymbols)/symbolRate - 0.3; at > 0 {
				truth = append(truth, mxTruth{at: at, data: s.Burst, cach: s.CACH, hasCACH: true, bs: true, g: i})
			}
		}
		out := make([]*genieCase, 0, len(seeds))
		for _, seed := range seeds {
			iq := channel(tt, tx, 2e6, 2e6, 0, 2-22.1, seed)
			out = append(out, genieIQBytesCase(fmt.Sprintf("2 dB seed %d", seed), iq, IQFormatCU8, 2000000, truth, -320, 1))
		}
		return out, nil
	}
}
