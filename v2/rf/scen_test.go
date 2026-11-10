package rf

import (
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/USA-RedDragon/dmrgo/v2/internal/rftx"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testscen"
	"github.com/USA-RedDragon/dmrgo/v2/internal/testseed"
)

// fixedPayloadTests compare with stored digests, counts or files, or write
// the measurements that later runs compare with.
var fixedPayloadTests = map[string]bool{
	"TestTierDigests":         true,
	"TestTier3Digest":         true,
	"TestExactTrellisDigest":  true,
	"TestReceiverDirectSeeds": true,
	"TestRxBench":             true,
	"TestRxBenchShard":        true,
	"TestRxBenchRuleCheck":    true,
	"TestRxShardCheck":        true,
	"TestRxBenchRecordings":   true,
	"TestRxBenchMerge":        true,
	"TestRxFloor":             true,
	"TestRxAware":             true,
	"TestRxFloorShard":        true,
	"TestRxFloorMerge":        true,
	"TestFloorProbe":          true,
	"TestFloorTurbo":          true,
	"TestRxACS":               true,
	"TestExtCompare":          true,
	"TestMCUBenchSignals":     true,
	"TestMMDVMCompare":        true,
	"TestRxSheet":             true,
	"TestShortLCBench":        true,
	"TestShortLCCost":         true,
	"TestSweepWorker":         true,
	"TestTablesGenerated":     true,
	"TestRxMatrix":            true,
}

var fixedRun = sync.OnceValue(func() bool {
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "DMRRF_") || k == "DMRGO_MCUBENCH_OUT" || k == "DMRGO_MMDVM" {
			return true
		}
	}
	return false
})

// bugPinTests keep the fixed payloads and noise seeds because their seeds
// reproduce a traced bug.
var bugPinTests = map[string]string{
	"TestTier2FadeUncoded":               "a fading draw where Tier 2 took uncoded AMBE bits from the coherent detection by the protected errors of each frame",
	"TestRate34CheckedCore":              "a fading draw where Tier 1 gave rate 3/4 blocks with a failed CRC-9 over good ones from its core",
	"TestLeanClockRateNullMS":            "seeds that learned a false MS clock rate",
	"TestRateLineNullMS":                 "cases that stepped a false rate on the rate line",
	"TestCoreTier3WarmNullClock":         "first locks at -521, -473 and +473 ppm on 0 ppm streams (11fd46a52)",
	"TestTier2LockError":                 "seeds that catch the lock frequency and timing faults (5670bc2a3)",
	"TestDiscTier2FalseLockBeforeStream": "a seed whose noise makes a false lock before the stream",
	"TestRxReplayNoisePad":               "starts and seeds where a replay placed a burst before the call",
	"TestRxEmbeddedLCTalkerChange":       "seeds that failed every cell under the old ELC combiner (f63c851a5)",
	"TestMSKeyUpGuardHold":               "a payload whose second burst the loop moved over the guard",
	"TestSecondsLockBurstWideFit":        "a payload whose Tier1 lock fit on the narrow filter output",
	"TestSecondsLockBurstFarOffset":      "a payload whose Tier0 lock burst clipped in the kept samples 2.4 kHz off",
	"TestSecondsLockBurstDeferredFit":    "a payload whose lock took the strobe when the sync fit deferred",
	"TestEMBFitLockZeroPast":             "a payload whose lock SYNC fit put its MM zero past the newest sample",
	"TestSecondsLockBurstFineFit":        "payloads whose Tier2 lock burst read 0.6 to 0.7 us RMS 2.5 kHz off",
	"TestSecondsTalkerSwitchHuntFit":     "payloads whose second talker locked on the sync correlation peak",
	"TestRxLevelStepTiming":              "payloads where a level and DC step pulled the timing loop",
	"TestRxLevelStepOneDrop":             "payloads where one level rise dropped the audio gain twice",
	"TestRxTwoTalkerClipDCSide":          "draws where a level rise clipped the second talker at one rail only",
	"TestRxTwoTalkerSyncLevelStepHead":   "payloads whose step burst head took the post-step phase unit",
	"TestRxAudioTwoTalkerLevelStepBlind": "payloads whose imbalance kept the blind refit of a stepped burst off",
	"TestDiscInvertedVoicePin":           "voice patterns that delayed the polarity flip past the fourth superframe",
	"TestLeanRCAnchorProof":              "a standalone RC SYNC with a bad EMB after a voice call",
	"TestRxJumpPin":                      "payloads whose first voice A after a 2 ms jump fell in a slot without a hunt",
	"TestShortLCSoftNotWrong":            "seeds where soft Short LCs passed the CRC-8 with wrong bits",
	"TestIQUnheldAdjacentWalk":           "an interferer payload whose residue walked an unheld converter onto the adjacent channel",
	"TestIQSyncTuneAdjacent":             "payloads whose lag-1 mean held the SYNC-fit tune back next to an adjacent channel",
	"TestIQSeekLevelsHoldTune":           "MS calls where the converter followed an EMB seek on noise in the empty slot",
	"TestIQHuntAfterLock":                "Tier 0 locks where the spectrum hunt tuned after the lock",
	"TestIQRefillSettle":                 "Tier 3 locks that tuned twice by the coherent lock offset",
	"TestRxRawBitsHuntTune":              "a held-out run whose first burst started before a 2.4 kHz hunt tune",
	"TestIQLowCWLead":                    "held-out runs whose first burst after a carrier lead lost bits to the I/Q mean",
	"TestDiscACAcquire":                  "held-out runs where the AC restorer took seconds to train",
	"TestRxLockRetuneCoherent":           "a held-out run where Tier 2 locked 3 kHz off and kept burst 0 from there",
	"TestRxMidLockRetune":                "a held-out run where a mid-burst Tier 1 lock kept symbols from before a hunt tune",
	"TestIQLowDCStart":                   "a held-out run whose first low-rate burst lost bits to the start of the I/Q mean",
	"TestIQSyncTuneTwoSlots":             "runs where the sync-only tune followed a weaker talker on the other slot",
	"TestIQSyncTuneFitNoise":             "weak calls where the sync-only tune followed the noise of the SYNC fit",
	"TestRxLockFarHeld":                  "a held-out run where Tier 0 locked 2.6 kHz off and put out burst 0 raw",
	"TestIQRefillInChannel":              "a held-out run where a refill tune left the channel",
	"TestRxEMBSeekSliceTie":              "a payload whose first EMB of the second talker decoded exactly at two slice timings",
	"TestRxReplayKeepsHunt":              "a payload whose replay restarted the kept samples before its EMB seeks",
	"TestRxEMBNotRCSync":                 "a payload whose voice EMB field reads as an MS reverse channel SYNC",
	"TestRxReplayLockOffset":             "a payload whose forced lock left the lock strobe 0.1 symbol off its SYNC",
	"TestRxEMBNotDataSync":               "a payload whose voice EMB field lies 12 bits from the MS data SYNC",
	"TestRxEMBShiftNotDataSync":          "a payload whose voice field one symbol off the grid lies 12 bits from the BS data SYNC",
	"TestRxReplaySeekEMBTiming":          "payloads whose EMB seek the 24-symbol peak timed 7 to 9 us off",
	"TestLeanFadeKeepsPhase":             "a payload whose burst after a fade was stamped early (loop on the faded slots)",
	"TestLeanWeakLockKeeps":              "payloads whose header a weak lock on its payload cut from the kept samples",
	"TestLeanPayloadHoldsSlot":           "payloads whose random EMB held a stopped talker's slot against a new SYNC",
	"TestRxReplayPayloadSync":            "a payload whose burst E passed the coherent checks as a data SYNC",
	"TestRxLateFillLevels":               "a payload whose late burst was filled at the levels of a later lock",
	"TestCoreLockSlotSync":               "a payload and noise where a voice burst locked on the past-sync sum",
	"TestRxReplayStretchSyncs":           "a payload whose SYNC correlation peaks put the replayed bursts 15 us early",
	"TestRxSeekAfterCohTune":             "a draw where the core took the coherent lock tune before the outputs made at it",
	"TestRxTier0SeekEMB":                 "payloads whose second talker starts at voice B before a color code vote, one losing a bit of burst C to the carried phase",
	"TestRxSeekBackfillRate":             "payloads whose backfilled burst B lost head bits to the nominal coherent grid",
	"TestRxSeekEyeVertex":                "a payload whose burst C EMB reads wrong at all five quarter symbol phases",
	"TestRxReplaySeekPhase":              "draws where the replay sliced the second talker at the phase of the first",
	"TestRxTier0LockRefit":               "draws where the locked weak talker lost head bits of burst B to its held phase",
	"TestRxCarrierLeadNoBurst":           "draws whose carrier lead slots read as a valid EMB",
	"TestRxCarrierLeadFirstBurstPhase":   "payloads whose first burst after a carrier lead took a carrier fit at the lock phase scale",
}

func fixedPayload(t testing.TB) bool {
	if _, pinned := t.(pinnedTB); pinned {
		return false
	}
	if t == nil || fixedRun() {
		return true
	}
	top, _, _ := strings.Cut(t.Name(), "/")
	for _, p := range []string{"Example", "Fuzz", "Benchmark", "TestAllocs"} {
		if strings.HasPrefix(top, p) {
			return true
		}
	}
	_, pin := bugPinTests[top]
	return pin || fixedPayloadTests[top]
}

var reproLogged sync.Map

func logRepro(t testing.TB) {
	if _, shared := t.(sharedTB); shared {
		return
	}
	if _, seen := reproLogged.LoadOrStore(t, true); !seen {
		testseed.Repro(t)
	}
}

func nameHash(s string) uint64 { return testseed.NameHash(s) }

func runMix(a, b uint64) uint64 { return testseed.Mix(a, b) }

// pinnedTB draws its payloads and noise from a fixed run seed, for a bug pin
// whose bug needs a payload that the fixed one does not carry.
type pinnedTB struct {
	testing.TB
	run uint64
}

func scenarioOf(t testing.TB) (s testscen.Scenario, ok bool) {
	if p, pinned := t.(pinnedTB); pinned {
		return testscen.Of(p.run), true
	}
	if fixedPayload(t) {
		return s, false
	}
	logRepro(t)
	return testscen.Of(runMix(testseed.RunSeed(), nameHash(t.Name()))), true
}

func randomized(t testing.TB, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return s.Apply(cfg)
	}
	return cfg
}

var runScenario = sync.OnceValue(func() testscen.Scenario {
	return testscen.Of(runMix(testseed.RunSeed(), 0x7266))
})

func sharedRandomized(cfg rftx.SeqConfig) rftx.SeqConfig { return runScenario().Apply(cfg) }

func noiseSeed(t testing.TB, seed uint64) uint64 {
	if p, pinned := t.(pinnedTB); pinned {
		return runMix(p.run, seed)
	}
	if fixedPayload(t) {
		return seed
	}
	logRepro(t)
	return runMix(runMix(testseed.RunSeed(), nameHash(t.Name())), seed)
}

func testConfig(t testing.TB) rftx.SeqConfig { return randomized(t, testConfigFixed()) }

func channel(t testing.TB, iq []byte, txRate, rxRate, offsetHz, snrDB float64, seed uint64) []byte {
	return rftx.Channel(iq, txRate, rxRate, offsetHz, snrDB, noiseSeed(t, seed))
}

func randomizedN(t testing.TB, salt uint64, cfg rftx.SeqConfig) rftx.SeqConfig {
	if s, ok := scenarioOf(t); ok {
		return testscen.Of(runMix(s.VoiceSeed, salt)).Apply(cfg)
	}
	return cfg
}

func benchTalkgroup(ids rftx.SeqConfig) uint32 {
	if ids.Dest == 3009201 {
		return 9
	}
	return 1 + uint32(runMix(uint64(ids.Dest), 9)%testscen.MaxAddress)
}

type sharedTB struct{ testing.TB }

func (sharedTB) Name() string { return "shared" }

// sharedRun stands for the tests that share a cached random input: its
// noise seeds come from the run seed only.
var sharedRun testing.TB = sharedTB{}
